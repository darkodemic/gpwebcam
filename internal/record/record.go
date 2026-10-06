// Package record copies the camera's MPEG-TS stream into a Matroska file
// with ffmpeg, without decoding it: the camera's H.264 goes to disk as it
// is, and the CPU hardly works.
package record

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// queueLen bounds the datagrams waiting for ffmpeg, about 3.5 s of the
	// camera's 6 Mb/s. A disk that falls further behind loses datagrams,
	// which are counted.
	queueLen = 2048
	// MinFree is the free space a recording needs to start, 1 GB: about
	// 20 minutes at 6 Mb/s.
	MinFree = 1_000_000_000
)

// SizeText formats a file size in decimal units with one decimal, as file
// managers such as Nautilus and Dolphin show it: 9165466 bytes is "9.2 MB".
func SizeText(n uint64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d bytes", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1f kB", float64(n)/1e3)
	case n < 1_000_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/1e9)
	}
}

// ErrLowSpace means the folder's file system has less than MinFree left.
var ErrLowSpace = errors.New("not enough free space to record")

// Recorder writes one recording.
type Recorder struct {
	cmd     *exec.Cmd // set before Start returns
	path    string
	started time.Time
	queue   chan []byte
	done    chan struct{}
	err     error // set before done closes
	dropped atomic.Int64
	stopped atomic.Bool
}

// FileName is the name of a recording started at t.
func FileName(t time.Time) string { return "GoPro-" + t.Format("2006-01-02-150405") + ".mkv" }

// Start creates dir if needed and starts ffmpeg on a new file in it, named
// by the time. ffmpeg's messages go to logs.
func Start(ffmpeg, dir string, logs io.Writer) (*Recorder, error) {
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("recordings folder %q: must be an absolute path", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if errors.Is(err, syscall.EROFS) {
			return nil, fmt.Errorf("%w; the service may write only to the folders its unit allows (ReadWritePaths)", err)
		}
		return nil, err
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err == nil {
		if free := st.Bavail * uint64(st.Bsize); free < MinFree {
			return nil, fmt.Errorf("%w in %s: %s left, %s needed", ErrLowSpace, dir, SizeText(free), SizeText(MinFree))
		}
	}
	now := time.Now()
	path, err := create(dir, now)
	if err != nil {
		return nil, err
	}
	r := &Recorder{path: path, started: now, queue: make(chan []byte, queueLen), done: make(chan struct{})}
	started := make(chan error, 1)
	go r.run(ffmpeg, logs, started)
	if err := <-started; err != nil {
		os.Remove(path)
		return nil, err
	}
	return r, nil
}

// create makes an empty file with a name no other recording has; ffmpeg
// then writes over it.
func create(dir string, t time.Time) (string, error) {
	base := FileName(t)
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d.mkv", base[:len(base)-len(".mkv")], i+1)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		f.Close()
		return path, nil
	}
	return "", fmt.Errorf("no free file name for %s in %s", base, dir)
}

// Args is ffmpeg's argument list for a recording to path.
func Args(path string) []string {
	return []string{
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "mpegts", "-i", "pipe:0",
		// Only the video: the camera's TS also carries an empty AAC and
		// AC3 track and a private data stream.
		"-map", "0:v:0", "-c", "copy",
		// Matroska stays playable when the recording ends abruptly, for
		// example when the cable is pulled.
		"-f", "matroska", "-y", path,
	}
}

// run starts ffmpeg and waits for it on a locked OS thread: Pdeathsig fires
// when the thread that started the process exits, so it must live as long
// as ffmpeg does.
func (r *Recorder) run(ffmpeg string, logs io.Writer, started chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(r.done)

	inR, inW, err := os.Pipe()
	if err != nil {
		started <- err
		return
	}
	cmd := exec.Command(ffmpeg, Args(r.path)...)
	r.cmd = cmd
	cmd.Stdin = inR
	cmd.Stderr = logs
	// Die with gpwebcam even if gpwebcam is killed without a chance to
	// clean up.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		inR.Close()
		inW.Close()
		started <- fmt.Errorf("start ffmpeg: %w", err)
		return
	}
	inR.Close()
	started <- nil

	go func() {
		for p := range r.queue {
			if _, err := inW.Write(p); err != nil {
				break // ffmpeg is gone; Wait says why
			}
		}
		// The end of its input makes ffmpeg finish the file and exit.
		inW.Close()
		for range r.queue {
		}
	}()
	if err := cmd.Wait(); err != nil {
		r.err = fmt.Errorf("ffmpeg: %w", err)
	} else if !r.stopped.Load() {
		r.err = errors.New("ffmpeg ended by itself")
	}
}

// Packet queues a datagram for the file. It never blocks: when the queue
// is full, the datagram is dropped and counted.
func (r *Recorder) Packet(p []byte) {
	if r.stopped.Load() {
		return
	}
	select {
	case r.queue <- p:
	default:
		r.dropped.Add(1)
	}
}

// Stop ends the input and waits up to timeout for ffmpeg to finish the
// file. It returns the error that ended the recording, if any.
func (r *Recorder) Stop(timeout time.Duration) error {
	if r.stopped.CompareAndSwap(false, true) {
		close(r.queue)
	}
	select {
	case <-r.done:
		return r.err
	case <-time.After(timeout):
		// Matroska written so far stays playable without the end.
		r.cmd.Process.Kill()
		<-r.done
		return fmt.Errorf("ffmpeg did not finish %s within %v", r.path, timeout)
	}
}

// Done is closed when ffmpeg has exited, after Stop or by itself.
func (r *Recorder) Done() <-chan struct{} { return r.done }

// Err is the reason ffmpeg exited, once Done is closed.
func (r *Recorder) Err() error {
	select {
	case <-r.done:
		return r.err
	default:
		return nil
	}
}

// Path is the recording's file.
func (r *Recorder) Path() string { return r.path }

// Started is when the recording began.
func (r *Recorder) Started() time.Time { return r.started }

// Dropped counts datagrams lost because the disk fell behind.
func (r *Recorder) Dropped() int64 { return r.dropped.Load() }
