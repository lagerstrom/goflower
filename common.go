package flower

import (
	"time"

	"github.com/google/uuid"
)

// gossipDeliveryInfo represents the delivery information for a gossip message.
type gossipDeliveryInfo struct {
	Exchange   string `json:"exchange"`
	RoutingKey string `json:"routing_key"`
}

// gossipProperties represents the properties of a gossip message.
type gossipProperties struct {
	DeliveryMode int                `json:"delivery_mode"`
	DeliveryInfo gossipDeliveryInfo `json:"delivery_info"`
	Priority     int                `json:"priority"`
	BodyEncoding string             `json:"body_encoding"`
	DeliveryTag  string             `json:"delivery_tag"`
}

// gossipHeaders represents the headers of a gossip message.
type gossipHeaders struct {
	Hostname string `json:"hostname"`
}

// gossipEnvelope represents the envelope of a gossip message.
type gossipEnvelope struct {
	Body            string           `json:"body"`
	ContentEncoding string           `json:"content-encoding"`
	ContentType     string           `json:"content-type"`
	Headers         gossipHeaders    `json:"headers"`
	Properties      gossipProperties `json:"properties"`
}

// gossipHeartbeatBody represents the body of a gossip heartbeat message.
type gossipHeartbeatBody struct {
	Hostname  string    `json:"hostname"`
	Utcoffset int       `json:"utcoffset"`
	Pid       int       `json:"pid"`
	Clock     int       `json:"clock"`
	Freq      float64   `json:"freq"`
	Active    int       `json:"active"`
	Processed int       `json:"processed"`
	Loadavg   []float64 `json:"loadavg"`
	SwIdent   string    `json:"sw_ident"`
	SwVer     string    `json:"sw_ver"`
	SwSys     string    `json:"sw_sys"`
	Timestamp float64   `json:"timestamp"`
	Type      string    `json:"type"`
}

// createDefaultEnvelope creates a default gossipEnvelope with preset values.
func createDefaultEnvelope(hostname string) gossipEnvelope {
	return gossipEnvelope{
		ContentEncoding: "utf-8",
		ContentType:     "application/json",
		Headers: gossipHeaders{
			Hostname: hostname,
		},
		Properties: gossipProperties{
			DeliveryMode: 1,
			DeliveryInfo: gossipDeliveryInfo{
				Exchange:   "celeryev",
				RoutingKey: "CHANGE_ME",
			},
			Priority:     0,
			BodyEncoding: "base64",
			DeliveryTag:  uuid.NewString(),
		},
	}
}

func timestampFromTime(now time.Time) float64 {
	return float64(now.UnixNano()) / float64(time.Second)
}
