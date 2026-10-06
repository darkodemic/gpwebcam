package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkodemic/gpwebcam/internal/tray"
)

// The control API lets "gpwebcam record" and other local programs talk to
// a running "gpwebcam run": HTTP over a Unix socket in the user's runtime
// directory, which only the user can open.
const (
	controlSocket = "control.sock"
	controlHost   = "http://gpwebcam" // any host; the socket decides
)

// controlSocketPath is $RUNTIME_DIRECTORY/control.sock, which systemd sets
// for the user service (RuntimeDirectory=gpwebcam), or else
// $XDG_RUNTIME_DIR/gpwebcam/control.sock. Both are the same path for the
// service, so the command line finds the service's socket.
func controlSocketPath() (string, error) {
	if d := os.Getenv("RUNTIME_DIRECTORY"); d != "" && filepath.IsAbs(d) && !strings.Contains(d, ":") {
		return filepath.Join(d, controlSocket), nil
	}
	d := os.Getenv("XDG_RUNTIME_DIR")
	if d == "" || !filepath.IsAbs(d) {
		return "", errors.New("XDG_RUNTIME_DIR is not set, so there is no place for the control socket")
	}
	return filepath.Join(d, "gpwebcam", controlSocket), nil
}

// controlStatus is the reply of GET /v1/status.
type controlStatus struct {
	Camera    string    `json:"camera"` // what the tray's first line says
	State     string    `json:"state"`  // off, live or trouble
	Recording bool      `json:"recording"`
	File      string    `json:"file,omitempty"` // once the recording's file is open
	Since     time.Time `json:"since,omitempty"`
	Error     string    `json:"error,omitempty"` // why the last recording stopped by itself
}

// controller is what the control API needs from the server.
type controller interface {
	status() controlStatus
	setRecording(on bool) error
	stopRecording() (savedRecording, error)
}

func (s *server) status() controlStatus {
	v := s.view()
	st := controlStatus{Camera: v.Status, State: map[tray.State]string{tray.Off: "off", tray.Live: "live", tray.Trouble: "trouble"}[v.State]}
	var err error
	st.Recording, st.File, st.Since, err = s.recStatus()
	if err != nil {
		st.Error = err.Error()
	}
	if !st.Recording {
		st.Since = time.Time{}
	}
	return st
}

// stopRecording turns recording off and returns the file it saved.
func (s *server) stopRecording() (savedRecording, error) {
	s.recMu.Lock()
	on, hadFile := s.recWant, s.rec != nil
	s.recMu.Unlock()
	if !on {
		return savedRecording{}, errors.New("not recording")
	}
	if err := s.setRecording(false); err != nil {
		return savedRecording{}, err
	}
	if !hadFile {
		return savedRecording{}, nil // stopped before the camera streamed
	}
	s.recMu.Lock()
	defer s.recMu.Unlock()
	return s.lastSaved, nil
}

func controlHandler(c controller) http.Handler {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(w http.ResponseWriter, err error) {
		reply(w, http.StatusConflict, map[string]string{"error": err.Error()})
	}
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, c.status())
	})
	mux.HandleFunc("POST /v1/record/start", func(w http.ResponseWriter, r *http.Request) {
		if err := c.setRecording(true); err != nil {
			fail(w, err)
			return
		}
		reply(w, http.StatusOK, c.status())
	})
	mux.HandleFunc("POST /v1/record/stop", func(w http.ResponseWriter, r *http.Request) {
		saved, err := c.stopRecording()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, http.StatusOK, saved)
	})
	return mux
}

// serveControl serves the control API until ctx ends. Without a socket the
// service works on; only "gpwebcam record" cannot reach it.
func serveControl(ctx context.Context, c controller, log *slog.Logger) {
	path, err := controlSocketPath()
	if err != nil {
		log.Warn("no control socket", "err", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Warn("no control socket", "err", err)
		return
	}
	// A socket that answers belongs to another gpwebcam; one that does
	// not is left over from a gpwebcam that was killed.
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		conn.Close()
		log.Warn("another gpwebcam serves the control socket; gpwebcam record talks to that one", "socket", path)
		return
	}
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		log.Warn("no control socket", "err", err)
		return
	}
	defer os.Remove(path)
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		log.Warn("no control socket", "err", err)
		return
	}
	srv := &http.Server{
		Handler:           controlHandler(c),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Stopping a recording waits for ffmpeg to finish the file.
		WriteTimeout: recordStop + 10*time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Warn("control socket", "err", err)
	}
}

// controlClient talks to the control socket of a running gpwebcam.
func controlClient() (*http.Client, string, error) {
	path, err := controlSocketPath()
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, "", fmt.Errorf("gpwebcam run is not running (no %s); start it with: systemctl --user start gpwebcam", path)
	}
	var d net.Dialer
	return &http.Client{
		Timeout: recordStop + 15*time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return d.DialContext(ctx, "unix", path)
			},
		},
	}, path, nil
}
