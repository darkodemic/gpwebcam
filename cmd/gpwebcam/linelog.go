package main

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	// lineBurst is how many lines a lineLogger passes per lineWindow; a
	// burst of decoder errors after packet loss must not flood the journal.
	lineBurst  = 20
	lineWindow = time.Minute
	// maxLine bounds a line that never ends.
	maxLine = 4096
)

// lineLogger turns a child process's output into log records, one per line,
// at most lineBurst per lineWindow.
type lineLogger struct {
	log    *slog.Logger
	source string
	now    func() time.Time

	mu      sync.Mutex
	buf     []byte
	start   time.Time // of the current window
	n       int       // lines logged in the window
	dropped int       // lines dropped in the window
}

func newLineLogger(log *slog.Logger, source string) *lineLogger {
	return &lineLogger{log: log, source: source, now: time.Now}
}

func (l *lineLogger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		l.emit(string(l.buf[:i]))
		l.buf = l.buf[i+1:]
	}
	if len(l.buf) > maxLine {
		l.emit(string(l.buf))
		l.buf = l.buf[:0]
	}
	return len(p), nil
}

func (l *lineLogger) emit(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if now := l.now(); now.Sub(l.start) >= lineWindow {
		if l.dropped > 0 {
			l.log.Warn("lines suppressed", "source", l.source, "count", l.dropped)
		}
		l.start, l.n, l.dropped = now, 0, 0
	}
	if l.n >= lineBurst {
		l.dropped++
		return
	}
	l.n++
	l.log.Warn(line, "source", l.source)
}
