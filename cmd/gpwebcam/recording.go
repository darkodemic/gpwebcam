package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/darkodemic/gpwebcam/internal/notify"
	"github.com/darkodemic/gpwebcam/internal/record"
	"github.com/darkodemic/gpwebcam/internal/settings"
	"github.com/darkodemic/gpwebcam/internal/tray"
)

// recordStop bounds the wait for ffmpeg to finish a recording's file.
const recordStop = 10 * time.Second

// savedRecording describes a finished recording.
type savedRecording struct {
	File    string        `json:"file"`
	Seconds float64       `json:"seconds"`
	Bytes   int64         `json:"bytes"`
	Dropped int64         `json:"dropped,omitempty"` // datagrams the disk was too slow for
	length  time.Duration // for messages
}

// defaultRecordDir is ~/Videos/gpwebcam, the folder the packaged unit lets
// the service write to.
func defaultRecordDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Videos", "gpwebcam"), nil
}

// setRecording turns recording on or off. A recording needs a connected
// camera whose mode is not off; it keeps the camera streaming, and every
// session of the camera writes a file of its own.
func (s *server) setRecording(on bool) error {
	if on {
		if s.live.Get().Camera == settings.CameraOff {
			return errors.New("the camera is off in gpwebcam; turn it on first")
		}
		if !s.present.Load() {
			return errors.New("no camera is connected")
		}
	}
	s.recMu.Lock()
	if on && !s.recWant {
		s.recSince, s.recErr = time.Now(), nil
	}
	s.recWant = on
	s.recMu.Unlock()
	s.syncRecorder()
	s.kick() // with camera mode demand, an idle camera starts now
	s.updateTray()
	return nil
}

// setRecLive tells whether a session's video flows, so that a recorder may
// run.
func (s *server) setRecLive(live bool) {
	s.recMu.Lock()
	s.recLive = live
	s.recMu.Unlock()
	s.syncRecorder()
}

// recording reports whether the user asked to record.
func (s *server) recording() bool {
	s.recMu.Lock()
	defer s.recMu.Unlock()
	return s.recWant
}

// syncRecorder starts or stops the recorder so that one runs exactly while
// recording is on and a session streams.
func (s *server) syncRecorder() {
	s.recMu.Lock()
	want := s.recWant && s.recLive
	switch {
	case want && s.rec == nil:
		r, err := record.Start(s.f.ffmpeg, s.recDir, newLineLogger(s.log, "ffmpeg-record"))
		if err != nil {
			s.recWant, s.recErr = false, err
			s.recMu.Unlock()
			s.log.Warn("cannot record", "err", err)
			s.note(notify.Normal, "gpwebcam cannot record", err.Error())
			s.updateTray()
			return
		}
		s.rec = r
		s.recPacket.Store(r)
		s.recMu.Unlock()
		s.log.Info("recording", "file", r.Path())
		s.note(notify.Low, "Recording", r.Path())
		go s.watchRecorder(r)
	case !want && s.rec != nil:
		r := s.rec
		s.rec = nil
		s.recPacket.Store(nil)
		saved := s.finish(r, r.Stop(recordStop))
		s.lastSaved = saved
		s.recMu.Unlock()
	default:
		s.recMu.Unlock()
	}
}

// finish reports a recording that ended, with err when ffmpeg failed.
func (s *server) finish(r *record.Recorder, err error) savedRecording {
	saved := savedRecording{File: r.Path(), Dropped: r.Dropped(), length: time.Since(r.Started())}
	saved.Seconds = saved.length.Seconds()
	if info, serr := os.Stat(r.Path()); serr == nil {
		saved.Bytes = info.Size()
	}
	summary := fmt.Sprintf("%s, %s, %s", filepath.Base(saved.File), tray.Clock(saved.length), record.SizeText(uint64(saved.Bytes)))
	switch {
	case err != nil:
		s.log.Warn("recording ended with an error", "file", saved.File, "err", err)
		s.note(notify.Normal, "Recording ended with an error", summary+". "+err.Error())
	case saved.Dropped > 0:
		s.log.Warn("recording saved, but the disk was too slow for some of it", "file", saved.File, "dropped", saved.Dropped)
		s.note(notify.Normal, "Recording saved with gaps", summary)
	default:
		s.log.Info("recording saved", "file", saved.File, "length", saved.length.Round(time.Second), "bytes", saved.Bytes)
		s.note(notify.Low, "Recording saved", summary)
	}
	return saved
}

// watchRecorder notices ffmpeg ending by itself, for example on a full
// disk, and turns recording off.
func (s *server) watchRecorder(r *record.Recorder) {
	<-r.Done()
	s.recMu.Lock()
	if s.rec != r {
		s.recMu.Unlock()
		return // stopped on purpose
	}
	s.rec = nil
	s.recPacket.Store(nil)
	s.recWant, s.recErr = false, r.Err()
	s.lastSaved = s.finish(r, r.Err())
	s.recMu.Unlock()
	s.updateTray()
}

// recPacketTo hands a datagram to the running recorder, if any.
func (s *server) recPacketTo(p []byte) {
	if r := s.recPacket.Load(); r != nil {
		r.Packet(p)
	}
}

// recStatus is the recording part of the status: whether it is on, its
// file once one is open, since when, and why it stopped last.
func (s *server) recStatus() (on bool, file string, since time.Time, lastErr error) {
	s.recMu.Lock()
	defer s.recMu.Unlock()
	if s.rec != nil {
		file = s.rec.Path()
	}
	return s.recWant, file, s.recSince, s.recErr
}

// refreshWhileRecording updates the tray every second while recording, so
// that its stop item shows the running time.
func (s *server) refreshWhileRecording(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if s.recording() {
			s.updateTray()
		}
	}
}
