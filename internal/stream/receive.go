package stream

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync/atomic"
	"time"
)

const (
	// queueLen bounds the datagrams waiting for ffmpeg: about 3.5 s of the
	// camera's 6 Mb/s in 1316-byte datagrams. When ffmpeg falls further
	// behind, new datagrams are dropped and counted, as ffmpeg's own UDP
	// input did with overrun_nonfatal.
	queueLen = 2048
	// readBuffer is the socket receive buffer gpwebcam asks for; the kernel
	// caps it at net.core.rmem_max.
	readBuffer = 4 << 20
)

// Stats counts what the receiver saw during one run.
type Stats struct {
	Packets int64 // datagrams from the camera
	Bytes   int64
	Dropped int64 // datagrams dropped because ffmpeg fell behind
	Foreign int64 // datagrams from another address, ignored
}

// receiver reads the camera's MPEG-TS datagrams from a UDP socket bound to
// the host's address on the GoPro link, and queues them for ffmpeg so that
// a slow ffmpeg never blocks the socket.
type receiver struct {
	conn   *net.UDPConn
	camera netip.Addr // only datagrams from here count; any when invalid
	queue  chan []byte
	tap    func([]byte) // gets every datagram from the camera, if set

	packets, bytes, dropped, foreign atomic.Int64
	last                             atomic.Int64 // UnixNano of the last datagram
}

func listen(addr netip.AddrPort, camera netip.Addr, tap func([]byte)) (*receiver, error) {
	conn, err := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(addr))
	if err != nil {
		return nil, fmt.Errorf("listen for the camera's stream: %w", err)
	}
	_ = conn.SetReadBuffer(readBuffer) // best effort
	return &receiver{conn: conn, camera: camera, queue: make(chan []byte, queueLen), tap: tap}, nil
}

// read queues datagrams until the socket is closed, then closes the queue.
func (r *receiver) read() {
	defer close(r.queue)
	buf := make([]byte, 64<<10)
	for {
		n, from, err := r.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			return
		}
		if r.camera.IsValid() && from.Addr().Unmap() != r.camera {
			r.foreign.Add(1)
			continue
		}
		p := make([]byte, n)
		copy(p, buf[:n])
		r.packets.Add(1)
		r.bytes.Add(int64(n))
		r.last.Store(time.Now().UnixNano())
		if r.tap != nil {
			r.tap(p)
		}
		select {
		case r.queue <- p:
		default:
			r.dropped.Add(1)
		}
	}
}

// pump writes queued datagrams to w until the queue closes or a write fails,
// which happens when ffmpeg is gone.
func (r *receiver) pump(w io.Writer) error {
	for p := range r.queue {
		if _, err := w.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// sinceLast is the time since the last datagram, or false before the first.
func (r *receiver) sinceLast() (time.Duration, bool) {
	t := r.last.Load()
	if t == 0 {
		return 0, false
	}
	return time.Since(time.Unix(0, t)), true
}

func (r *receiver) stats() Stats {
	return Stats{Packets: r.packets.Load(), Bytes: r.bytes.Load(), Dropped: r.dropped.Load(), Foreign: r.foreign.Load()}
}

func (r *receiver) close() error { return r.conn.Close() }
