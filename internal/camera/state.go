package camera

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// WebcamStatus is the webcam state the camera reports, per the Open GoPro spec.
type WebcamStatus int

const (
	StatusOff              WebcamStatus = 0
	StatusIdle             WebcamStatus = 1
	StatusHighPowerPreview WebcamStatus = 2 // streaming
	StatusLowPowerPreview  WebcamStatus = 3 // streaming, preview quality
	StatusUnavailable      WebcamStatus = 4 // in the spec's table, not its enum; HERO13 returns it
)

func (s WebcamStatus) String() string {
	switch s {
	case StatusOff:
		return "off"
	case StatusIdle:
		return "idle"
	case StatusHighPowerPreview:
		return "high power preview"
	case StatusLowPowerPreview:
		return "low power preview"
	case StatusUnavailable:
		return "unavailable"
	}
	return fmt.Sprintf("unknown status %d", int(s))
}

// Streaming reports whether the camera is sending a stream.
func (s WebcamStatus) Streaming() bool {
	return s == StatusHighPowerPreview || s == StatusLowPowerPreview
}

// WebcamError is the error code in a webcam reply, per the Open GoPro spec.
type WebcamError int

var webcamErrors = map[WebcamError]string{
	0: "none",
	1: "set preset failed",
	2: "set window size failed",
	3: "exec stream failed",
	4: "shutter is active",
	5: "com timeout",
	6: "invalid parameter",
	7: "webcam unavailable",
	8: "exit failed",
}

func (e WebcamError) String() string {
	if s, ok := webcamErrors[e]; ok {
		return s
	}
	return fmt.Sprintf("unknown error %d", int(e))
}

// Reply is the JSON body of /gopro/webcam/status: {"status":N,"error":N}.
type Reply struct {
	Status WebcamStatus `json:"status"`
	Error  WebcamError  `json:"error"`
}

// ReplyError is a webcam reply whose error code is not "none".
type ReplyError struct {
	Op   string
	Code WebcamError
}

func (e *ReplyError) Error() string {
	return fmt.Sprintf("%s: camera reports %s (error %d)", e.Op, e.Code, int(e.Code))
}

type rawReply struct {
	Status *WebcamStatus `json:"status"`
	Error  *WebcamError  `json:"error"`
}

// parseStatus decodes a status reply. Both fields are required, so an empty
// object or unrelated JSON is not mistaken for a state.
func parseStatus(op string, body []byte) (Reply, error) {
	var raw rawReply
	if err := json.Unmarshal(body, &raw); err != nil {
		return Reply{}, fmt.Errorf("%s: unexpected reply %q: %w", op, truncate(body), err)
	}
	if raw.Status == nil || raw.Error == nil {
		return Reply{}, fmt.Errorf("%s: unexpected reply %q: want status and error", op, truncate(body))
	}
	r := Reply{Status: *raw.Status, Error: *raw.Error}
	if r.Error != 0 {
		return r, &ReplyError{Op: op, Code: r.Error}
	}
	return r, nil
}

// checkCommand checks the reply to a webcam command. The spec documents an
// empty object, but cameras also answer {"status":N,"error":N}; an error
// code other than 0 is a failure either way.
func checkCommand(op string, body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	var raw rawReply
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("%s: unexpected reply %q: %w", op, truncate(body), err)
	}
	if raw.Error != nil && *raw.Error != 0 {
		return &ReplyError{Op: op, Code: *raw.Error}
	}
	return nil
}

func truncate(b []byte) string {
	const n = 200
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
