package main

import (
	"github.com/darkodemic/gpwebcam/internal/notify"
	"github.com/darkodemic/gpwebcam/internal/settings"
	"github.com/darkodemic/gpwebcam/internal/tray"
)

// settingsChanged applies new settings to the running service. It runs on
// the goroutine that changed them: the tray's or the settings watcher's.
func (s *server) settingsChanged(old, cur settings.Settings) {
	if cur.HWDec == "auto" && old.HWDec != "auto" {
		// Turning hardware decoding on again asks to retry the GPU.
		s.mu.Lock()
		s.gpuFailed = false
		s.mu.Unlock()
	}
	if cur.FOV != old.FOV || cur.HWDec != old.HWDec {
		s.restartSession()
	}
	if cur.Camera != old.Camera {
		switch {
		case cur.Camera == settings.CameraOff:
			s.endSession(errOff)
		case !s.wantCamera():
			s.endSession(errIdle)
		}
		s.kick()
	}
	if cur.Res != old.Res && cur.Res != s.res {
		s.log.Info("the new resolution applies when gpwebcam restarts", "in_use", s.res, "next", cur.Res)
	}
	if cur.Tray != old.Tray {
		if cur.Tray {
			s.startTray()
		} else if t := s.detachTray(); t != nil {
			// Stop waits for the tray's goroutine, which may be the one
			// running this.
			go t.Stop()
			s.note(notify.Low, "gpwebcam icon hidden", "Bring it back with: gpwebcam config tray on")
		}
	}
	s.updateTray()
}

// restartSession ends the running session, if any; the main loop starts
// the next one at once with the current settings.
func (s *server) restartSession() { s.endSession(errReconfigured) }

// setStatus records what the tray shows about the camera.
func (s *server) setStatus(st tray.State, status string) {
	s.mu.Lock()
	s.trayState, s.trayStatus = st, status
	s.mu.Unlock()
	s.updateTray()
}

// view is the tray's picture of the service.
func (s *server) view() tray.View {
	set := s.live.Get()
	locked := map[string]bool{}
	for _, k := range settings.Keys {
		locked[k] = s.live.Locked(k)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return tray.View{
		State: s.trayState, Status: s.trayStatus,
		Settings: set, Locked: locked, ResPending: set.Res != s.res,
		CanRestart: underSystemd(),
	}
}

func (s *server) updateTray() {
	s.trayMu.Lock()
	t := s.tray
	s.trayMu.Unlock()
	if t != nil {
		t.Update(s.view())
	}
}

// startTray shows the icon unless it is already shown.
func (s *server) startTray() {
	if s.once {
		return
	}
	s.trayMu.Lock()
	defer s.trayMu.Unlock()
	if s.tray != nil {
		return
	}
	s.tray = tray.Start(s.log, s.view(), tray.Actions{
		Set: func(key, value string) {
			if err := s.live.Set(key, value); err != nil {
				s.log.Warn("change setting from the tray", "setting", key, "err", err)
			}
		},
		Restart: func() {
			s.log.Info("restart asked from the tray")
			// systemd stops this process, which waits for the tray.
			go func() {
				if err := restartService(); err != nil {
					s.log.Warn("restart through systemd", "err", err)
				}
			}()
		},
	})
	s.log.Info("tray icon shown")
}

// detachTray takes the tray from the server so that no update reaches it.
func (s *server) detachTray() *tray.Tray {
	s.trayMu.Lock()
	defer s.trayMu.Unlock()
	t := s.tray
	s.tray = nil
	return t
}

// stopTray removes the icon and waits until it is gone.
func (s *server) stopTray() {
	if t := s.detachTray(); t != nil {
		t.Stop()
		s.log.Info("tray icon removed")
	}
}
