package feed

import (
	"context"
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

	if err := f.Idle([]byte("idle")); err != nil {
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

	f.Idle([]byte("idle"))
	time.Sleep(50 * time.Millisecond)
	if n := rec.count("idle"); n < before+3 {
		t.Errorf("idle repeats did not resume")
	}
}
