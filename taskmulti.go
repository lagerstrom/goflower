package flower

import (
	"encoding/json"
	"os"
	"time"
)

// taskMultiBaseBody represents the base body of a task message.
type taskMultiBaseBody struct {
	Hostname  string  `json:"hostname"`
	Utcoffset int     `json:"utcoffset"`
	Pid       int     `json:"pid"`
	Clock     int     `json:"clock"`
	UUID      string  `json:"uuid"`
	Timestamp float64 `json:"timestamp"`
	Type      string  `json:"type"`
}

// taskMultiSucceededBody represents the body of a task-succeeded message.
type taskMultiSucceededBody struct {
	taskMultiBaseBody
	Result  string  `json:"result"`
	Runtime float64 `json:"runtime"`
}

// taskMultiStartedBody represents the body of a task-started message.
type taskMultiStartedBody taskMultiBaseBody

// taskMultiReceivedBody represents the body of a task-received message.
type taskMultiReceivedBody struct {
	taskMultiBaseBody
	Name     string      `json:"name"`
	Args     string      `json:"args"`
	Kwargs   string      `json:"kwargs"`
	RootID   string      `json:"root_id"`
	ParentID interface{} `json:"parent_id"`
	Retries  int         `json:"retries"`
	Eta      interface{} `json:"eta"`
	Expires  interface{} `json:"expires"`
}

// taskMultiFailedBody represents the body of a task-failed message.
type taskMultiFailedBody struct {
	taskMultiBaseBody
	Exception string `json:"exception"`
	Traceback string `json:"traceback"`
}

func newTaskMultiBaseBody(hostname, taskId, taskType string) taskMultiBaseBody {
	now := time.Now()

	return taskMultiBaseBody{
		Hostname:  hostname,
		Pid:       os.Getpid(),
		Clock:     0,
		UUID:      taskId,
		Timestamp: timestampFromTime(now),
		Type:      taskType,
	}
}

func createTaskMultiFailedBody(hostname, taskId, exception, traceback string) ([]byte, error) {
	body := taskMultiFailedBody{
		taskMultiBaseBody: newTaskMultiBaseBody(hostname, taskId, "task-failed"),
		Exception:         exception,
		Traceback:         traceback,
	}

	return json.Marshal([]taskMultiFailedBody{body})
}

// createTaskMultiReceivedBody creates a taskMultiReceivedBody and returns it as JSON.
func createTaskMultiReceivedBody(hostname, taskName, args, rootID, taskID string) ([]byte, error) {
	body := taskMultiReceivedBody{
		taskMultiBaseBody: newTaskMultiBaseBody(hostname, taskID, "task-received"),
		Name:              taskName,
		Args:              args,
		Kwargs:            "{}",
		RootID:            rootID,
		ParentID:          nil,
		Retries:           0,
		Eta:               nil,
		Expires:           nil,
	}

	return json.Marshal(body)
}

// createTaskMultiStartedBody creates a taskMultiStartedBody and returns it as JSON.
func createTaskMultiStartedBody(hostname, taskID string) ([]byte, error) {
	body := taskMultiStartedBody(newTaskMultiBaseBody(hostname, taskID, "task-started"))

	return json.Marshal([]taskMultiStartedBody{body})
}

// createTaskMultiSucceededBody creates a taskMultiSucceededBody and returns it as JSON.
func createTaskMultiSucceededBody(hostname, taskID, result string, runtime float64) ([]byte, error) {
	body := taskMultiSucceededBody{
		taskMultiBaseBody: newTaskMultiBaseBody(hostname, taskID, "task-succeeded"),
		Result:            result,
		Runtime:           runtime,
	}

	return json.Marshal([]taskMultiSucceededBody{body})
}
