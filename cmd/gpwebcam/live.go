package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/darkodemic/gpwebcam/internal/settings"
)

// settingsCheck is how often a running gpwebcam looks at the settings file,
// so that "gpwebcam config" and edits by hand take effect without a restart.
const settingsCheck = 2 * time.Second

// live holds the settings of a running gpwebcam: the settings file, with
// the flags given on the command line on top. The tray menu changes them
// through Set; other changes to the file are noticed by watch.
type live struct {
	path  string
	flags settings.Settings // values of the flags in locked
	// locked are the settings given as flags; the file cannot change them.
	locked map[string]bool
	log    *slog.Logger
	// changed is called after the effective settings changed, outside the
	// lock. It may call Get and Set.
	changed func(old, cur settings.Settings)

	mu      sync.Mutex
	file    settings.Settings // as last read from or written to the file
	stamp   fileStamp
	lastErr string
}

// fileStamp tells whether the file changed since it was last read. Save
// replaces the file by rename, which changes the inode.
type fileStamp struct {
	exists bool
	ino    uint64
	size   int64
	mod    time.Time
}

func stampOf(path string) (fileStamp, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileStamp{}, nil
	}
	if err != nil {
		return fileStamp{}, err
	}
	st := fileStamp{exists: true, size: info.Size(), mod: info.ModTime()}
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		st.ino = sys.Ino
	}
	return st, nil
}

// newLive reads the settings file. A file that cannot be read gives the
// defaults and a warning: a typo must not keep the webcam from starting.
func newLive(path string, f startFlags, log *slog.Logger) *live {
	l := &live{path: path, flags: f.settings(), locked: f.set, log: log, file: settings.Defaults()}
	if l.locked == nil {
		l.locked = map[string]bool{}
	}
	if path == "" {
		return l
	}
	l.stamp, _ = stampOf(path)
	s, err := settings.Load(path)
	if err != nil {
		l.lastErr = err.Error()
		log.Warn("settings file not used, using the defaults", "err", err)
	}
	l.file = s
	return l
}

// Get returns the settings in effect.
func (l *live) Get() settings.Settings {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.effective()
}

// Locked reports whether a flag fixes the setting.
func (l *live) Locked(key string) bool { return l.locked[key] }

func (l *live) effective() settings.Settings {
	s := l.file
	for _, k := range settings.Keys {
		if l.locked[k] {
			_ = s.Set(k, l.flags.Get(k)) // flag values are already checked
		}
	}
	return s
}

// Set changes one setting and saves the file.
func (l *live) Set(key, value string) error {
	if l.locked[key] {
		return errors.New("-" + key + " is given as a flag; the flag wins")
	}
	l.mu.Lock()
	next := l.file
	if err := next.Set(key, value); err != nil {
		l.mu.Unlock()
		return err
	}
	old := l.effective()
	if l.path != "" {
		if err := settings.Save(l.path, next); err != nil {
			l.mu.Unlock()
			return err
		}
		l.stamp, _ = stampOf(l.path)
	}
	l.file = next
	cur := l.effective()
	l.mu.Unlock()
	l.notify(old, cur)
	return nil
}

// watch reloads the file whenever it changes, until ctx ends.
func (l *live) watch(ctx context.Context) {
	if l.path == "" {
		return
	}
	for sleep(ctx, settingsCheck) {
		l.reload()
	}
}

// reload reads the file if it changed since the last read or write.
func (l *live) reload() {
	stamp, err := stampOf(l.path)
	l.mu.Lock()
	if err != nil || stamp == l.stamp {
		l.mu.Unlock()
		return
	}
	old := l.effective()
	s, err := settings.Load(l.path)
	l.stamp = stamp
	if err != nil {
		// Keep the settings in use; say so once per distinct problem.
		msg := err.Error()
		report := msg != l.lastErr
		l.lastErr = msg
		l.mu.Unlock()
		if report {
			l.log.Warn("settings file changed but cannot be used; keeping the current settings", "err", err)
		}
		return
	}
	l.lastErr = ""
	l.file = s
	cur := l.effective()
	l.mu.Unlock()
	l.notify(old, cur)
}

func (l *live) notify(old, cur settings.Settings) {
	if old == cur {
		return
	}
	for _, k := range settings.Keys {
		if old.Get(k) != cur.Get(k) {
			l.log.Info("setting changed", "setting", k, "value", cur.Get(k))
		}
	}
	if l.changed != nil {
		l.changed(old, cur)
	}
}
