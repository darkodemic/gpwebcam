package camera

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// HTTPPort is where the Open GoPro HTTP API listens, over USB as over Wi-Fi.
const HTTPPort = 8080

// Open GoPro HTTP API 2.0 paths.
const (
	pathInfo      = "/gopro/camera/info"
	pathWiredUSB  = "/gopro/camera/control/wired_usb"
	pathKeepAlive = "/gopro/camera/keep_alive"
	pathStatus    = "/gopro/webcam/status"
	pathStart     = "/gopro/webcam/start"
	pathStop      = "/gopro/webcam/stop"
	pathExit      = "/gopro/webcam/exit"
)

// ErrNoAnswer means the camera's HTTP server did not answer within
// StartOptions.Connect. A replug usually fixes it.
var ErrNoAnswer = errors.New("the camera does not answer")

// ErrCannotCapture means webcam start failed with error 4, "shutter is
// active": the camera cannot start capturing. A HERO13 Black without its
// battery refused every start this way; with the battery in, one start
// 2.3 s after a stop did (both 2026-10-06).
var ErrCannotCapture = errors.New("the camera cannot start capturing")

// errShutter is the webcam error code behind ErrCannotCapture.
const errShutter WebcamError = 4

// startTries is how many starts StartWebcam sends while the camera answers
// ErrCannotCapture, StartOptions.Pause apart.
const startTries = 3

// KeepAliveInterval is the spec's recommended keep-alive period.
const KeepAliveInterval = 3 * time.Second

// DisableWiredUSBControl turns wired USB control off, which the spec requires
// before webcam commands over USB.
func (c *Client) DisableWiredUSBControl(ctx context.Context) error {
	body, err := c.get(ctx, pathWiredUSB, "p=0")
	if err != nil {
		return err
	}
	return checkCommand("disable wired USB control", body)
}

// Info is what /gopro/camera/info says about the camera.
type Info struct {
	Model    string `json:"model_name"`
	Firmware string `json:"firmware_version"`
}

// Info reads the camera's model and firmware version. It changes nothing on
// the camera.
func (c *Client) Info(ctx context.Context) (Info, error) {
	body, err := c.get(ctx, pathInfo, "")
	if err != nil {
		return Info{}, err
	}
	var i Info
	if err := json.Unmarshal(body, &i); err != nil {
		return Info{}, fmt.Errorf("camera info: unexpected reply %q: %w", truncate(body), err)
	}
	return i, nil
}

// Status returns the webcam state.
func (c *Client) Status(ctx context.Context) (Reply, error) {
	body, err := c.get(ctx, pathStatus, "")
	if err != nil {
		return Reply{}, err
	}
	return parseStatus("webcam status", body)
}

// KeepAlive keeps the camera from powering down.
func (c *Client) KeepAlive(ctx context.Context) error {
	_, err := c.get(ctx, pathKeepAlive, "")
	return err
}

// Start asks for an MPEG-TS stream over UDP to the caller's address. A
// refusal with error 4 matches ErrCannotCapture.
func (c *Client) Start(ctx context.Context, res Resolution, fov FOV, port uint16) error {
	// The spec wants the arguments in this order.
	q := fmt.Sprintf("res=%d&fov=%d&port=%d&protocol=TS", res.code(), fov.code(), port)
	body, err := c.get(ctx, pathStart, q)
	if err == nil {
		err = checkCommand("webcam start", body)
	}
	if code, ok := replyCode(err); ok && code == errShutter {
		return fmt.Errorf("%w: %w", ErrCannotCapture, err)
	}
	return err
}

// replyCode returns the webcam error code that err carries: from a
// ReplyError, or from an HTTP error whose body is a webcam reply. A HERO13
// answers a refused start with HTTP 500 and {"status":1,"error":4}.
func replyCode(err error) (WebcamError, bool) {
	var re *ReplyError
	if errors.As(err, &re) {
		return re.Code, true
	}
	var se *StatusError
	if errors.As(err, &se) {
		var raw rawReply
		if json.Unmarshal([]byte(se.Body), &raw) == nil && raw.Error != nil {
			return *raw.Error, true
		}
	}
	return 0, false
}

// startTrying sends start up to startTries times while the camera answers
// ErrCannotCapture, which can pass by itself after a stop.
func (c *Client) startTrying(ctx context.Context, o StartOptions) error {
	for try := 1; ; try++ {
		err := c.Start(ctx, o.Res, o.FOV, o.Port)
		if err == nil || !errors.Is(err, ErrCannotCapture) || try == startTries {
			return err
		}
		if o.OnRefused != nil {
			o.OnRefused(err)
		}
		t := time.NewTimer(o.Pause)
		select {
		case <-ctx.Done():
			t.Stop()
			return err
		case <-t.C:
		}
	}
}

// Stop ends the stream; the camera stays in webcam mode.
func (c *Client) Stop(ctx context.Context) error {
	body, err := c.get(ctx, pathStop, "")
	if err != nil {
		return err
	}
	return checkCommand("webcam stop", body)
}

// Exit ends the stream and leaves webcam mode.
func (c *Client) Exit(ctx context.Context) error {
	body, err := c.get(ctx, pathExit, "")
	if err != nil {
		return err
	}
	return checkCommand("webcam exit", body)
}

// StartOptions tunes StartWebcam.
type StartOptions struct {
	Res  Resolution
	FOV  FOV
	Port uint16
	// Poll is the pause between retries and status checks.
	Poll time.Duration
	// Connect bounds the wait for the camera's HTTP server to answer.
	Connect time.Duration
	// Streaming bounds the wait for the camera to report a running stream
	// after Start.
	Streaming time.Duration
	// OnStatus, if set, gets the webcam status read before Start.
	OnStatus func(WebcamStatus)
	// Pause separates starts after ErrCannotCapture.
	Pause time.Duration
	// OnRefused, if set, gets each ErrCannotCapture that is tried again.
	OnRefused func(error)
}

// StartWebcam follows the spec's webcam state machine: wired USB control
// off, stop a stream a previous run left behind, start, then wait until the
// camera reports that it is streaming.
func (c *Client) StartWebcam(ctx context.Context, o StartOptions) error {
	cctx, cancel := context.WithTimeout(ctx, o.Connect)
	err := retry(cctx, o.Poll, func() error { return c.DisableWiredUSBControl(cctx) })
	cancel()
	if err != nil {
		return fmt.Errorf("%w within %v: %w", ErrNoAnswer, o.Connect, err)
	}

	st, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if o.OnStatus != nil {
		o.OnStatus(st.Status)
	}
	if st.Status == StatusIdle {
		// Open GoPro FAQ: after a new USB connection the camera reports
		// idle instead of off, and GoPro's workaround is a start followed
		// at once by a stop. Without it, the first start after a replug
		// twice reported streaming but sent nothing (2026-10-05).
		if err := c.startTrying(ctx, o); err != nil {
			return fmt.Errorf("start-stop workaround for an idle camera: %w", err)
		}
		if err := c.Stop(ctx); err != nil {
			return fmt.Errorf("start-stop workaround for an idle camera: %w", err)
		}
	}
	if st.Status.Streaming() {
		// Left over from an earlier run that did not stop it.
		if err := c.Stop(ctx); err != nil {
			return fmt.Errorf("stop the previous stream: %w", err)
		}
	}

	if err := c.startTrying(ctx, o); err != nil {
		return err
	}
	sctx, cancel := context.WithTimeout(ctx, o.Streaming)
	defer cancel()
	var last Reply
	err = retry(sctx, o.Poll, func() error {
		r, err := c.Status(sctx)
		if err != nil {
			return err
		}
		last = r
		if !r.Status.Streaming() {
			return fmt.Errorf("webcam status is %s", r.Status)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("camera did not start streaming within %v (last status %s): %w", o.Streaming, last.Status, err)
	}
	return nil
}

// StopWebcam stops the stream and leaves webcam mode, as GoPro's own SDK
// does. Both requests are sent even if the first fails.
func (c *Client) StopWebcam(ctx context.Context) error {
	return errors.Join(c.Stop(ctx), c.Exit(ctx))
}

// retry calls f until it succeeds or ctx ends, pausing poll between calls.
// It returns f's last error that was not caused by ctx ending, because that
// one says why the camera did not answer.
func retry(ctx context.Context, poll time.Duration, f func() error) error {
	t := time.NewTicker(poll)
	defer t.Stop()
	var last error
	for {
		err := f()
		if err == nil {
			return nil
		}
		if last == nil || ctx.Err() == nil {
			last = err
		}
		select {
		case <-ctx.Done():
			return last
		case <-t.C:
		}
	}
}
