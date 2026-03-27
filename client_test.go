package goflower

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
)

func TestFinishRuntimeMissingTaskLeavesActiveCountUnchanged(t *testing.T) {
	c := &client{
		runtimeState: make(map[string]time.Time),
	}

	if got := c.finishRuntime("missing-task"); got != 0 {
		t.Fatalf("finishRuntime() = %v, want 0 for unknown task", got)
	}

	if got := c.activeTasks.Load(); got != 0 {
		t.Fatalf("activeTasks = %d, want 0", got)
	}
}

func TestCloseTimesOutWhenHeartbeatDoesNotStop(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{})
	c := &client{
		getConn: func(context.Context) (redis.Conn, error) {
			return &fakeConn{
				doFn: func(commandName string, args ...interface{}) (interface{}, error) {
					close(started)
					<-blocked
					return nil, nil
				},
			}, nil
		},
		hostname:           "worker-1",
		runtimeState:       make(map[string]time.Time),
		heartbeatDone:      make(chan struct{}),
		closeWaitTimeout:   20 * time.Millisecond,
		heartbeatIOTimeout: 20 * time.Millisecond,
	}

	go func() {
		runHeartbeat(context.Background(), c.getConn, c.hostname, &c.activeTasks, nil, 5*time.Millisecond, c.heartbeatIOTimeout)
		close(c.heartbeatDone)
	}()

	waitFor(t, 250*time.Millisecond, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	})

	if err := c.Close(); err == nil {
		t.Fatal("Close() error = nil, want timeout error")
	}
}

func TestStartRuntimeDoesNotDoubleCountDuplicateTaskID(t *testing.T) {
	c := &client{
		runtimeState: make(map[string]time.Time),
	}

	c.startRuntime("task-1")
	c.startRuntime("task-1")

	if got := c.activeTasks.Load(); got != 1 {
		t.Fatalf("activeTasks = %d, want 1", got)
	}

	if got := len(c.runtimeState); got != 1 {
		t.Fatalf("runtimeState size = %d, want 1", got)
	}
}

func TestPublishLifecycleDoesNotDriveActiveTasksNegativeWhenStartedPublishFails(t *testing.T) {
	startedConn := &fakeConn{
		doFn: func(commandName string, args ...interface{}) (interface{}, error) {
			return nil, errors.New("publish failed")
		},
	}
	failedConn := &fakeConn{}

	var calls int
	c := &client{
		getConn: func(context.Context) (redis.Conn, error) {
			calls++
			if calls == 1 {
				return startedConn, nil
			}
			return failedConn, nil
		},
		hostname:         "worker-1",
		runtimeState:     make(map[string]time.Time),
		closeWaitTimeout: 20 * time.Millisecond,
	}

	if err := c.PublishStarted("task-1"); err == nil {
		t.Fatal("PublishStarted() error = nil, want non-nil")
	}

	if got := c.activeTasks.Load(); got != 0 {
		t.Fatalf("activeTasks after failed start = %d, want 0", got)
	}

	if err := c.PublishFailed("task-1", "boom", CurrentStackTrace(0)); err != nil {
		t.Fatalf("PublishFailed() error = %v, want nil", err)
	}

	if got := c.activeTasks.Load(); got != 0 {
		t.Fatalf("activeTasks after failed lifecycle = %d, want 0", got)
	}

	if got := len(c.runtimeState); got != 0 {
		t.Fatalf("runtimeState size = %d, want 0", got)
	}
}
