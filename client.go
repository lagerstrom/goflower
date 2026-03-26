package flower

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"errors"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gomodule/redigo/redis"
	"go.uber.org/zap"
)

const (
	defaultHeartbeatInterval     = 2 * time.Second
	defaultHeartbeatIOTimeout    = 2 * time.Second
	defaultHeartbeatCloseTimeout = 3 * time.Second
)

//go:generate mockgen -destination mocks/client.go github.com/lagerstrom/goflower Client

// Client publishes Flower-compatible task lifecycle events to Redis.
//
// The intended call sequence for a task is:
// PublishReceived, PublishStarted, then either PublishSucceeded or
// PublishFailed.
//
// A Client created by NewClient also runs a background heartbeat loop until
// Close is called.
type Client interface {
	PublishReceived(taskName, args, rootId, taskId string) error
	PublishStarted(taskId string) error
	PublishSucceeded(taskId, result string) error
	PublishFailed(taskId, exception, traceback string) error
	Close() error
}

// client implements the Client interface.
type client struct {
	getConn            func(context.Context) (redis.Conn, error)
	runtimeState       map[string]time.Time
	mutex              sync.Mutex
	hostname           string
	activeTasks        atomic.Int32
	heartbeatCancel    context.CancelFunc
	heartbeatDone      chan struct{}
	heartbeatIOTimeout time.Duration
	closeWaitTimeout   time.Duration
	closeOnce          sync.Once
}

// NewClient creates a Client backed by the provided Redis pool.
//
// The returned Client starts a background heartbeat goroutine immediately and
// must be closed when no longer needed.
//
// hostPrefix, when non-empty, is prepended to the machine hostname to form the
// worker name reported to Flower.
//
// If logger is nil, a no-op logger is used.
func NewClient(redisPool *redis.Pool, hostPrefix string, logger *zap.Logger) (Client, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if redisPool == nil {
		return nil, fmt.Errorf("missing dependencies: [redis pool]")
	}

	hostname, err := buildHostname(hostPrefix)
	if err != nil {
		return nil, err
	}

	c := &client{
		getConn:            redisPool.GetContext,
		hostname:           hostname,
		runtimeState:       make(map[string]time.Time),
		heartbeatDone:      make(chan struct{}),
		heartbeatIOTimeout: defaultHeartbeatIOTimeout,
		closeWaitTimeout:   defaultHeartbeatCloseTimeout,
	}

	if err := c.validate(); err != nil {
		return nil, err
	}

	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	c.heartbeatCancel = heartbeatCancel

	// Start sending heartbeats to Flower
	go func() {
		defer close(c.heartbeatDone)
		runHeartbeat(heartbeatCtx, c.getConn, c.hostname, &c.activeTasks, logger, defaultHeartbeatInterval, c.heartbeatIOTimeout)
	}()

	return c, nil
}

func buildHostname(hostPrefix string) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("unable to get hostname: %w", err)
	}

	if hostPrefix == "" {
		return hostname, nil
	}

	return fmt.Sprintf("%s-%s", hostPrefix, hostname), nil
}

// validate checks if all required dependencies are set.
func (c *client) validate() error {
	var missingDeps []string

	if c.getConn == nil {
		missingDeps = append(missingDeps, "redis connection getter")
	}

	if c.hostname == "" {
		missingDeps = append(missingDeps, "hostname")
	}

	if len(missingDeps) > 0 {
		return fmt.Errorf("missing dependencies: %v", missingDeps)
	}

	return nil
}

func (c *client) Close() error {
	var closeErr error

	c.closeOnce.Do(func() {
		if c.heartbeatCancel != nil {
			c.heartbeatCancel()
		}
		if c.heartbeatDone != nil {
			select {
			case <-c.heartbeatDone:
			case <-time.After(c.closeWaitTimeout):
				closeErr = errors.New("timed out waiting for heartbeat shutdown")
			}
		}
	})

	return closeErr
}

func createDefaultMultiEnvelope(hostname string, body []byte) gossipEnvelope {
	envelope := createDefaultEnvelope(hostname)
	envelope.Properties.DeliveryInfo.RoutingKey = "task.multi"
	envelope.Body = base64.StdEncoding.EncodeToString(body)

	return envelope
}

// PublishFailed publishes a task failed event to the Redis server.
func (c *client) PublishFailed(taskId, exception, traceback string) error {
	conn, err := c.getConn(context.Background())
	if err != nil {
		return fmt.Errorf("unable to get redis connection: %w", err)
	}
	if conn == nil {
		return fmt.Errorf("unable to get redis connection: nil connection")
	}
	defer conn.Close()

	c.finishRuntime(taskId)

	body, err := createTaskMultiFailedBody(c.hostname, taskId, exception, traceback)
	if err != nil {
		return fmt.Errorf("unable to create task failed body: %w", err)
	}

	failedTask := createDefaultMultiEnvelope(c.hostname, body)

	jsonPayload, err := json.Marshal(failedTask)
	if err != nil {
		return fmt.Errorf("unable to marshal failed task: %w", err)
	}

	if _, err := conn.Do("PUBLISH", "/0.celeryev/task.multi", jsonPayload); err != nil {
		return fmt.Errorf("unable to publish failed task: %w", err)
	}

	return nil
}

// PublishReceived publishes a task received event to the Redis server.
func (c *client) PublishReceived(taskName, args, rootId, taskId string) error {
	conn, err := c.getConn(context.Background())
	if err != nil {
		return fmt.Errorf("unable to get redis connection: %w", err)
	}
	if conn == nil {
		return fmt.Errorf("unable to get redis connection: nil connection")
	}
	defer conn.Close()

	body, err := createTaskMultiReceivedBody(c.hostname, taskName, args, rootId, taskId)
	if err != nil {
		return fmt.Errorf("unable to create task received body: %w", err)
	}

	receivedTask := createDefaultMultiEnvelope(c.hostname, body)

	jsonPayload, err := json.Marshal(receivedTask)
	if err != nil {
		return fmt.Errorf("unable to marshal received task: %w", err)
	}

	if _, err := conn.Do("PUBLISH", "/0.celeryev/task.multi", jsonPayload); err != nil {
		return fmt.Errorf("unable to publish received task: %w", err)
	}

	return nil
}

// PublishStarted publishes a task started event to the Redis server.
func (c *client) PublishStarted(taskId string) error {
	conn, err := c.getConn(context.Background())
	if err != nil {
		return fmt.Errorf("unable to get redis connection: %w", err)
	}
	if conn == nil {
		return fmt.Errorf("unable to get redis connection: nil connection")
	}
	defer conn.Close()

	body, err := createTaskMultiStartedBody(c.hostname, taskId)
	if err != nil {
		return fmt.Errorf("unable to create task started body: %w", err)
	}

	startedTask := createDefaultMultiEnvelope(c.hostname, body)

	jsonPayload, err := json.Marshal(startedTask)
	if err != nil {
		return fmt.Errorf("unable to marshal heartbeat: %w", err)
	}

	if _, err := conn.Do("PUBLISH", "/0.celeryev/task.multi", jsonPayload); err != nil {
		return fmt.Errorf("unable to publish started task: %w", err)
	}

	c.startRuntime(taskId)

	return nil
}

// PublishSucceeded publishes a task succeeded event to the Redis server.
func (c *client) PublishSucceeded(taskId, result string) error {
	conn, err := c.getConn(context.Background())
	if err != nil {
		return fmt.Errorf("unable to get redis connection: %w", err)
	}
	if conn == nil {
		return fmt.Errorf("unable to get redis connection: nil connection")
	}
	defer conn.Close()

	runtime := c.finishRuntime(taskId)

	body, err := createTaskMultiSucceededBody(c.hostname, taskId, result, runtime)
	if err != nil {
		return fmt.Errorf("unable to create task succeeded body: %w", err)
	}

	succeededTask := createDefaultMultiEnvelope(c.hostname, body)

	jsonPayload, err := json.Marshal(succeededTask)
	if err != nil {
		return fmt.Errorf("unable to marshal heartbeat: %w", err)
	}

	if _, err := conn.Do("PUBLISH", "/0.celeryev/task.multi", jsonPayload); err != nil {
		return fmt.Errorf("unable to publish succeeded task: %w", err)
	}

	return nil
}

// startRuntime records the start time of a task.
func (c *client) startRuntime(taskId string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, exists := c.runtimeState[taskId]; exists {
		c.runtimeState[taskId] = time.Now()
		return
	}

	c.runtimeState[taskId] = time.Now()
	c.activeTasks.Add(1)
}

// finishRuntime calculates the runtime of a task and removes it from the state.
func (c *client) finishRuntime(taskId string) float64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	startTime, ok := c.runtimeState[taskId]
	if !ok {
		return 0
	}

	delete(c.runtimeState, taskId)
	c.activeTasks.Add(-1)

	return time.Since(startTime).Seconds()
}

// CurrentStackTrace returns the current Go call stack as a string.
//
// skip controls how many additional caller frames to omit from the top of the
// returned stack beyond the helper itself.
func CurrentStackTrace(skip int) string {
	// skip: number of initial frames to skip
	// 0 = include the direct caller of CurrentStackTrace, 1 = skip one more, etc.
	pc := make([]uintptr, 32)
	n := runtime.Callers(skip+2, pc) // +2 to skip runtime.Callers and runtime.CallersFrames setup
	frames := runtime.CallersFrames(pc[:n])

	var b bytes.Buffer
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&b, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return b.String()
}
