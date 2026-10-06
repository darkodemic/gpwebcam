// Package feed keeps the loopback device supplied with frames: live camera
// frames when there are any, a placeholder frame otherwise. One producer
// that never closes the device keeps it listed as a camera.
package feed

import (
	"context"
	"sync"
	"time"
)

// Sink takes complete frames, e.g. *v4l2.Output.
type Sink interface {
	WriteFrame([]byte) error
}

// Source supplies the idle picture. Frame gets the time since the source
// was set, so an animated picture knows which step to show.
type Source interface {
	Frame(elapsed time.Duration) []byte
}

// Still is a Source that is always the same frame.
type Still []byte

// Frame implements Source.
func (s Still) Frame(time.Duration) []byte { return s }

// Feed writes to one Sink, either the idle picture on a timer or live
// frames as they arrive.
type Feed struct {
	sink     Sink
	interval time.Duration
	now      func() time.Time

	mu        sync.Mutex
	idle      Source // nil while live frames flow
	idleSince time.Time
	err       error // last write error of the idle loop
}

// New returns a Feed that writes the idle picture every interval.
func New(sink Sink, interval time.Duration) *Feed {
	return &Feed{sink: sink, interval: interval, now: time.Now}
}

// Idle switches to src until the next Live call. Its first frame is written
// at once, so the device has a picture immediately.
func (f *Feed) Idle(src Source) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idle, f.idleSince = src, f.now()
	return f.sink.WriteFrame(src.Frame(0))
}

// Swap replaces the sink while no frame is written. swap gets the current
// sink and returns the new one with the idle picture for it, which is
// written at once. If swap fails, the feed keeps the current sink, so swap
// must leave that one usable.
func (f *Feed) Swap(swap func(old Sink) (Sink, Source, error)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sink, src, err := swap(f.sink)
	if err != nil {
		return err
	}
	f.sink, f.idle, f.idleSince, f.err = sink, src, f.now(), nil
	return sink.WriteFrame(src.Frame(0))
}

// Live writes a camera frame and stops the idle repeats.
func (f *Feed) Live(frame []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idle = nil
	return f.sink.WriteFrame(frame)
}

// Run writes the idle picture until ctx ends. Write errors are kept for Err;
// a missing device ends the process elsewhere.
func (f *Feed) Run(ctx context.Context) {
	t := time.NewTicker(f.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		f.mu.Lock()
		if f.idle != nil {
			f.err = f.sink.WriteFrame(f.idle.Frame(f.now().Sub(f.idleSince)))
		}
		f.mu.Unlock()
	}
}

// Err returns the last error of an idle write, or nil.
func (f *Feed) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}
