# goflower

`goflower` is a small Go client for publishing Celery/Flower worker events to Redis.

It is intended for Go services that want to appear as Flower workers by emitting:

- task received events
- task started events
- task succeeded events
- task failed events
- worker heartbeats

## Status

This package is focused on the high-level client API. It is not intended to be a general-purpose Celery event-format toolkit.

## Installation

```sh
go get github.com/lagerstrom/goflower
```

## Usage

```go
package main

import (
	"log"

	"github.com/gomodule/redigo/redis"
	"github.com/lagerstrom/goflower"
	"go.uber.org/zap"
)

func main() {
	pool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "127.0.0.1:6379")
		},
	}

	logger := zap.NewExample()

	client, err := goflower.NewClient(pool, "payments", logger)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	taskID := "task-123"
	rootID := taskID

	if err := client.PublishReceived("payments.charge", `["order-42"]`, rootID, taskID); err != nil {
		log.Fatal(err)
	}

	if err := client.PublishStarted(taskID); err != nil {
		log.Fatal(err)
	}

	if err := runTask(); err != nil {
		traceback := goflower.CurrentStackTrace(0)
		if publishErr := client.PublishFailed(taskID, err.Error(), traceback); publishErr != nil {
			log.Fatal(publishErr)
		}
		return
	}

	if err := client.PublishSucceeded(taskID, `"ok"`); err != nil {
		log.Fatal(err)
	}
}

func runTask() error {
	return nil
}
```

## API

The intended public API is:

- `NewClient`
- `Client`
- `CurrentStackTrace`

`NewClient` starts a background heartbeat loop immediately. Call `Close()` when the client is no longer needed so the heartbeat goroutine can stop cleanly.

`CurrentStackTrace` is a convenience helper for callers that want to attach the current Go call stack to `PublishFailed`.

## Event Semantics

The client publishes Celery event messages to Redis channels under `/0.celeryev/...`.

The publish flow is expected to be:

1. `PublishReceived`
2. `PublishStarted`
3. `PublishSucceeded` or `PublishFailed`

Runtime tracking is derived from the time between `PublishStarted` and `PublishSucceeded` or `PublishFailed`.

The `args`, `result`, `exception`, and `traceback` fields are passed through as strings and should already be encoded in the shape you want Flower to display.

`PublishFailed` accepts an explicit traceback string. If you want the current Go stack at reporting time, use `CurrentStackTrace`.

## Development

Run tests:

```sh
make test
```

Regenerate mocks:

```sh
make generate
```

The `generate` target installs a pinned `mockgen` binary into `./.bin` if it is not already present.

The `test` target regenerates mocks first and then runs the module test suite.

Remove generated artifacts:

```sh
make clean
```

## Notes

- Redis connectivity is required for publishing events.
- Hostnames are derived from the current machine hostname, optionally prefixed with the `hostPrefix` argument passed to `NewClient`.
- The package uses `redigo` for Redis and `zap` for logging.
- `Close()` waits for the heartbeat loop to stop and returns an error if shutdown does not complete within the internal timeout.
- Configure your `redis.Pool` with sensible dial, read, and write timeouts. This package assumes Redis operations are bounded; if your pool allows indefinitely blocking I/O, shutdown can still be delayed until those operations return.

## License

MIT. See [LICENSE](LICENSE).
