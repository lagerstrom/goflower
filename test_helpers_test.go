package flower

import (
	"testing"
	"time"
)

type fakeConn struct {
	doFn    func(commandName string, args ...interface{}) (interface{}, error)
	closeFn func() error
}

func (c *fakeConn) Close() error {
	if c.closeFn != nil {
		return c.closeFn()
	}
	return nil
}

func (c *fakeConn) Err() error {
	return nil
}

func (c *fakeConn) Do(commandName string, args ...interface{}) (interface{}, error) {
	if c.doFn != nil {
		return c.doFn(commandName, args...)
	}
	return nil, nil
}

func (c *fakeConn) DoWithTimeout(timeout time.Duration, commandName string, args ...interface{}) (interface{}, error) {
	return c.Do(commandName, args...)
}

func (c *fakeConn) Send(string, ...interface{}) error {
	return nil
}

func (c *fakeConn) Flush() error {
	return nil
}

func (c *fakeConn) Receive() (interface{}, error) {
	return nil, nil
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("condition was not met within %s", timeout)
}
