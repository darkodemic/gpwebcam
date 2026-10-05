package notify

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSend(t *testing.T) {
	var mu sync.Mutex
	var calls [][]string
	sent := make(chan struct{}, 10)
	now := time.Unix(0, 0)
	n := &Notifier{
		run: func(_ context.Context, args ...string) error {
			mu.Lock()
			calls = append(calls, args)
			mu.Unlock()
			sent <- struct{}{}
			return nil
		},
		now:  func() time.Time { return now },
		last: map[string]time.Time{},
	}
	wait := func() {
		select {
		case <-sent:
		case <-time.After(time.Second):
			t.Fatal("notification not sent")
		}
	}

	n.Send(Low, "GoPro connected", "Streaming 1080p")
	wait()
	n.Send(Low, "GoPro connected", "Streaming 1080p") // repeat within the gap
	n.Send(Low, "GoPro disconnected", "")
	wait()
	now = now.Add(repeatGap)
	n.Send(Low, "GoPro connected", "Streaming 1080p")
	wait()

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("sent %d notifications, want 3: %q", len(calls), calls)
	}
	got := strings.Join(calls[0], " ")
	if !strings.Contains(got, "--urgency=low") || !strings.HasSuffix(got, "-- GoPro connected Streaming 1080p") {
		t.Errorf("arguments = %q", got)
	}
}

func TestNilNotifier(t *testing.T) {
	var n *Notifier
	n.Send(Normal, "x", "y") // must not panic
}
