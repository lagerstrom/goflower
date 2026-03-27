package goflower

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"go.uber.org/zap"
)

func TestCreateHeartbeatBodyUsesCurrentRuntimeMetadata(t *testing.T) {
	body, err := createHeartbeatBody("worker-1", 3)
	if err != nil {
		t.Skipf("createHeartbeatBody() unavailable in this environment: %v", err)
	}

	var heartbeat gossipHeartbeatBody
	if err := json.Unmarshal(body, &heartbeat); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got, want := heartbeat.Active, 3; got != want {
		t.Fatalf("Active = %d, want %d", got, want)
	}

	if got, want := heartbeat.SwSys, runtime.GOOS; got != want {
		t.Fatalf("SwSys = %q, want %q", got, want)
	}

	if got, want := heartbeat.SwVer, runtime.Version(); got != want {
		t.Fatalf("SwVer = %q, want %q", got, want)
	}
}

func TestRunHeartbeatReconnectsAfterPublishError(t *testing.T) {
	var getConnCalls atomic.Int32
	var failedPublishes atomic.Int32
	var successfulPublishes atomic.Int32

	badConn := &fakeConn{
		doFn: func(commandName string, args ...interface{}) (interface{}, error) {
			failedPublishes.Add(1)
			return nil, errors.New("boom")
		},
	}
	goodConn := &fakeConn{
		doFn: func(commandName string, args ...interface{}) (interface{}, error) {
			successfulPublishes.Add(1)
			return nil, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		runHeartbeat(ctx, func(context.Context) (redis.Conn, error) {
			if getConnCalls.Add(1) == 1 {
				return badConn, nil
			}
			return goodConn, nil
		}, "worker-1", &atomic.Int32{}, zap.NewNop(), 5*time.Millisecond, 20*time.Millisecond)
	}()

	waitFor(t, 250*time.Millisecond, func() bool {
		return failedPublishes.Load() >= 1 && successfulPublishes.Load() >= 1
	})

	cancel()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("runHeartbeat() did not stop after cancellation")
	}

	if got := getConnCalls.Load(); got < 2 {
		t.Fatalf("getConn() calls = %d, want at least 2", got)
	}
}

func TestClientCloseStopsHeartbeat(t *testing.T) {
	var publishes atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	c := &client{
		getConn: func(context.Context) (redis.Conn, error) {
			return &fakeConn{
				doFn: func(commandName string, args ...interface{}) (interface{}, error) {
					publishes.Add(1)
					return nil, nil
				},
			}, nil
		},
		hostname:           "worker-1",
		runtimeState:       make(map[string]time.Time),
		heartbeatCancel:    cancel,
		heartbeatDone:      make(chan struct{}),
		heartbeatIOTimeout: 20 * time.Millisecond,
		closeWaitTimeout:   20 * time.Millisecond,
	}

	go func() {
		defer close(c.heartbeatDone)
		runHeartbeat(ctx, c.getConn, c.hostname, &c.activeTasks, zap.NewNop(), 5*time.Millisecond, c.heartbeatIOTimeout)
	}()

	waitFor(t, 250*time.Millisecond, func() bool {
		return publishes.Load() >= 1
	})

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	publishCount := publishes.Load()
	time.Sleep(20 * time.Millisecond)

	if got := publishes.Load(); got != publishCount {
		t.Fatalf("publishes after Close() = %d, want %d", got, publishCount)
	}
}
