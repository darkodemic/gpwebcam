package feed

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu     sync.Mutex
	frames []string
}

func (r *recorder) WriteFrame(f []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, string(f))
	return nil
}

func (r *recorder) count(s string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, f := range r.frames {
		if f == s {
			n++
		}
	}
	return n
}

func TestFeed(t *testing.T) {
	rec := &recorder{}
	f := New(rec, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Run(ctx)

	if err := f.Idle(Still("idle")); err != nil {
		t.Fatal(err)
	}
	if rec.count("idle") != 1 {
		t.Fatal("Idle did not write a frame at once")
	}
	time.Sleep(50 * time.Millisecond)
	if n := rec.count("idle"); n < 3 {
		t.Fatalf("idle frame repeated %d times in 50ms", n)
	}

	f.Live([]byte("live"))
	before := rec.count("idle")
	time.Sleep(50 * time.Millisecond)
	if n := rec.count("idle"); n != before {
		t.Errorf("idle frame written %d times while live", n-before)
	}
	if rec.count("live") != 1 {
		t.Error("live frame not written")
	}

	f.Idle(Still("idle"))
	time.Sleep(50 * time.Millisecond)
	if n := rec.count("idle"); n < before+3 {
		t.Errorf("idle repeats did not resume")
	}
}

// steps is an animated Source: "a" for the first 20 ms, then "b".
type steps struct{}

func (steps) Frame(elapsed time.Duration) []byte {
	if elapsed < 20*time.Millisecond {
		return []byte("a")
	}
	return []byte("b")
}

func TestFeedAnimates(t *testing.T) {
	rec := &recorder{}
	f := New(rec, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Run(ctx)
	f.Idle(steps{})
	time.Sleep(60 * time.Millisecond)
	if rec.count("a") == 0 || rec.count("b") == 0 {
		t.Errorf("animation steps written: a=%d b=%d", rec.count("a"), rec.count("b"))
	}
}

func TestFeedSwap(t *testing.T) {
	old, next := &recorder{}, &recorder{}
	f := New(old, time.Hour)
	if err := f.Idle(Still("small")); err != nil {
		t.Fatal(err)
	}
	err := f.Swap(func(s Sink) (Sink, Source, error) {
		if s != old {
			t.Error("swap did not get the current sink")
		}
		return next, Still("large"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.count("large") != 1 {
		t.Error("the new idle picture was not written at once")
	}
	if err := f.Live([]byte("frame")); err != nil {
		t.Fatal(err)
	}
	if next.count("frame") != 1 || old.count("frame") != 0 {
		t.Error("live frames do not go to the new sink")
	}

	// A failed swap keeps the sink.
	if err := f.Swap(func(Sink) (Sink, Source, error) { return nil, nil, errors.New("no device") }); err == nil {
		t.Error("failed swap reported no error")
	}
	if err := f.Live([]byte("again")); err != nil || next.count("again") != 1 {
		t.Errorf("after a failed swap: %v", err)
	}
}
