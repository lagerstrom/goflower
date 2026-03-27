package goflower

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/shirou/gopsutil/v3/load"
	"go.uber.org/zap"
)

// createHeartbeatBody creates the body of the heartbeat message and returns it as a byte slice in JSON format.
func createHeartbeatBody(hostname string, activeTasks int) ([]byte, error) {
	avg, err := load.Avg()
	if err != nil {
		return nil, fmt.Errorf("unable to get load average: %w", err)
	}

	now := time.Now()
	hb := gossipHeartbeatBody{
		Hostname:  hostname,
		Pid:       os.Getpid(),
		Clock:     0,
		Freq:      2.0,
		Active:    activeTasks,
		Processed: 0,
		Loadavg:   []float64{avg.Load1, avg.Load5, avg.Load15},
		SwIdent:   "gopher-goflower",
		SwVer:     runtime.Version(),
		SwSys:     runtime.GOOS,
		Timestamp: timestampFromTime(now),
		Type:      "worker-heartbeat",
	}

	return json.Marshal(hb)
}

// sendHeartbeat sends a heartbeat message to the Redis server
func sendHeartbeat(redisConnection redis.Conn, hostname string, activeTasks *atomic.Int32, timeout time.Duration) error {
	if redisConnection == nil {
		return fmt.Errorf("redis connection is nil")
	}

	active := 0
	if activeTasks != nil {
		active = int(activeTasks.Load())
	}

	heartbeatBody, err := createHeartbeatBody(hostname, active)
	if err != nil {
		return fmt.Errorf("unable to create heartbeat body: %w", err)
	}
	heartbeat := createDefaultEnvelope(hostname)
	heartbeat.Properties.DeliveryInfo.RoutingKey = "worker.heartbeat"
	heartbeat.Body = base64.StdEncoding.EncodeToString(heartbeatBody)

	heartbeatPayload, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("unable to marshal heartbeat: %w", err)
	}

	if _, err := doRedisCommand(redisConnection, timeout, "PUBLISH", "/0.celeryev/worker.heartbeat", heartbeatPayload); err != nil {
		return fmt.Errorf("unable to publish heartbeat: %w", err)
	}
	return nil
}

func doRedisCommand(conn redis.Conn, timeout time.Duration, commandName string, args ...interface{}) (interface{}, error) {
	if timeout > 0 {
		if connWithTimeout, ok := conn.(redis.ConnWithTimeout); ok {
			return connWithTimeout.DoWithTimeout(timeout, commandName, args...)
		}
	}

	return conn.Do(commandName, args...)
}

func runHeartbeat(ctx context.Context, getConn func(context.Context) (redis.Conn, error), hostname string, activeTasks *atomic.Int32, logger *zap.Logger, interval, timeout time.Duration) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if getConn == nil {
		logger.Error("unable to start heartbeat", zap.Error(fmt.Errorf("redis connection getter is nil")))
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			opCtx, cancel := context.WithTimeout(ctx, timeout)
			conn, err := getConn(opCtx)
			cancel()
			if err != nil {
				logger.Error("unable to get heartbeat redis connection", zap.Error(err))
				continue
			}
			if err := sendHeartbeat(conn, hostname, activeTasks, timeout); err != nil {
				logger.Error("unable to send heartbeat", zap.Error(err))
			}
			if conn != nil {
				if err := conn.Close(); err != nil {
					logger.Debug("unable to close heartbeat redis connection", zap.Error(err))
				}
			}
		}
	}
}
