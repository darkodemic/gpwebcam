package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLineLogger(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, nil))
	now := time.Unix(0, 0)
	l := newLineLogger(log, "ffmpeg")
	l.now = func() time.Time { return now }

	// Lines may arrive split across writes.
	fmt.Fprint(l, "first ")
	fmt.Fprint(l, "line\n\nsecond line\n")
	if got := strings.Count(out.String(), "source=ffmpeg"); got != 2 {
		t.Fatalf("logged %d lines, want 2:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), `msg="first line"`) {
		t.Errorf("split line not joined:\n%s", out.String())
	}

	// A burst is cut at lineBurst lines per window.
	out.Reset()
	for i := 0; i < 50; i++ {
		fmt.Fprintf(l, "error %d\n", i)
	}
	if got := strings.Count(out.String(), "source=ffmpeg"); got != lineBurst-2 {
		t.Errorf("logged %d lines of the burst, want %d", got, lineBurst-2)
	}

	// The next window reports how many were dropped.
	out.Reset()
	now = now.Add(lineWindow)
	fmt.Fprint(l, "later\n")
	if !strings.Contains(out.String(), "lines suppressed") || !strings.Contains(out.String(), "count=32") {
		t.Errorf("no suppression report:\n%s", out.String())
	}
}
