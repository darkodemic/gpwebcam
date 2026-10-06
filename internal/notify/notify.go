// Package notify shows desktop notifications through notify-send, which
// talks to the freedesktop Notifications service on the session D-Bus.
// Notifications are a convenience: every failure is ignored.
package notify

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// Urgency levels of the freedesktop Notifications specification.
const (
	Low    = "low"
	Normal = "normal"
)

// repeatGap is how long an identical notification stays quiet, so that a
// camera that keeps failing does not fill the desktop.
const repeatGap = 30 * time.Second

// Notifier sends notifications. The zero value and a nil *Notifier send
// nothing.
type Notifier struct {
	run func(ctx context.Context, args ...string) error
	now func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
}

// New returns a Notifier that uses the notify-send program, or nil when it
// is not installed.
func New() *Notifier {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		return nil
	}
	return &Notifier{
		run: func(ctx context.Context, args ...string) error {
			return exec.CommandContext(ctx, path, args...).Run()
		},
		now:  time.Now,
		last: map[string]time.Time{},
	}
}

// Send shows summary and body, unless the same pair was shown within
// repeatGap. It returns at once; notify-send runs in the background.
func (n *Notifier) Send(urgency, summary, body string) {
	n.send(urgency, summary, body, false)
}

// SendNow is Send that waits for notify-send, for a process about to exit.
func (n *Notifier) SendNow(urgency, summary, body string) {
	n.send(urgency, summary, body, true)
}

func (n *Notifier) send(urgency, summary, body string, wait bool) {
	if n == nil || n.run == nil {
		return
	}
	key := summary + "\x00" + body
	n.mu.Lock()
	now := n.now()
	if t, ok := n.last[key]; ok && now.Sub(t) < repeatGap {
		n.mu.Unlock()
		return
	}
	n.last[key] = now
	n.mu.Unlock()

	args := []string{
		"--app-name=gpwebcam",
		"--icon=camera-web",
		"--urgency=" + urgency,
		"--expire-time=6000",
		"--", summary, body,
	}
	show := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = n.run(ctx, args...)
	}
	if wait {
		show()
		return
	}
	go show()
}
