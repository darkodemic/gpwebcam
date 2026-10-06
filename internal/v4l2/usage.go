package v4l2

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// eventSubscription mirrors struct v4l2_event_subscription.
type eventSubscription struct {
	Type     uint32
	ID       uint32
	Flags    uint32
	Reserved [5]uint32
}

// event mirrors struct v4l2_event on 64-bit Linux: the union after Type is
// 8-byte aligned and 64 bytes long, and the timestamp is a struct timespec
// of two 64-bit fields (136 bytes in total, checked against the kernel
// headers 2026-10-06).
type event struct {
	Type     uint32
	_        uint32
	U        [64]byte
	Pending  uint32
	Sequence uint32
	Sec      int64
	Nsec     int64
	ID       uint32
	Reserved [8]uint32
	_        uint32
}

const (
	// vidiocSubscribeEvent is _IOW('V', 90, struct v4l2_event_subscription).
	vidiocSubscribeEvent = 1<<30 | uint32(unsafe.Sizeof(eventSubscription{}))<<16 | 'V'<<8 | 90
	// vidiocDQEvent is _IOR('V', 89, struct v4l2_event).
	vidiocDQEvent = 2<<30 | uint32(unsafe.Sizeof(event{}))<<16 | 'V'<<8 | 89

	// eventClientUsage is v4l2loopback's V4L2_EVENT_PRI_CLIENT_USAGE:
	// V4L2_EVENT_PRIVATE_START + 0x08E00000 + 1. Its payload is a __u32,
	// 1 while an application streams from the device and 0 otherwise.
	eventClientUsage = 0x08000000 + 0x08E00000 + 1
	// eventSendInitial is V4L2_EVENT_SUB_FL_SEND_INITIAL: queue the
	// current state at subscription.
	eventSendInitial = 1
)

// usagePoll bounds how long Usage.Run waits for an event before it looks
// at its context again.
const usagePoll = 500 * time.Millisecond

// ErrNoUsageEvents means the v4l2loopback module does not report when
// applications start and stop streaming.
var ErrNoUsageEvents = errors.New("this v4l2loopback does not report when applications use the device")

// Usage reports whether applications stream from a v4l2loopback device,
// through its client usage event. v4l2loopback sends the event when a
// reader starts or stops streaming, also when it closes the device or is
// killed, and, asked with eventSendInitial, once at subscription with the
// current state. Merely opening the device, as applications do to list
// cameras, sends nothing.
type Usage struct {
	f *os.File
}

// WatchUsage opens path a second time, only for events.
func WatchUsage(path string) (*Usage, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	sub := eventSubscription{Type: eventClientUsage, Flags: eventSendInitial}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(vidiocSubscribeEvent), uintptr(unsafe.Pointer(&sub))); errno != 0 {
		f.Close()
		if errno == syscall.EINVAL {
			return nil, ErrNoUsageEvents
		}
		return nil, fmt.Errorf("%s: VIDIOC_SUBSCRIBE_EVENT: %w", path, errno)
	}
	return &Usage{f: f}, nil
}

// Run calls used with the current state, then with every change, until
// ctx ends or reading events fails.
func (u *Usage) Run(ctx context.Context, used func(bool)) error {
	fd := int(u.f.Fd())
	if fd >= syscall.FD_SETSIZE {
		return fmt.Errorf("event file descriptor %d is too large for select", fd)
	}
	for ctx.Err() == nil {
		// v4l2 events raise an exceptional condition (POLLPRI).
		var except syscall.FdSet
		except.Bits[fd/64] |= 1 << (uint(fd) % 64)
		tv := syscall.NsecToTimeval(usagePoll.Nanoseconds())
		n, err := syscall.Select(fd+1, nil, nil, &except, &tv)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("wait for device events: %w", err)
		}
		if n == 0 {
			continue
		}
		for {
			var ev event
			_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.f.Fd(), uintptr(vidiocDQEvent), uintptr(unsafe.Pointer(&ev)))
			if errno == syscall.ENOENT || errno == syscall.EAGAIN {
				break // no more events queued
			}
			if errno != 0 {
				return fmt.Errorf("VIDIOC_DQEVENT: %w", errno)
			}
			if ev.Type == eventClientUsage {
				used(clientCount(ev) > 0)
			}
		}
	}
	return ctx.Err()
}

// clientCount reads the payload of a client usage event.
func clientCount(ev event) uint32 { return binary.NativeEndian.Uint32(ev.U[:4]) }

// Close releases the event file.
func (u *Usage) Close() error { return u.f.Close() }
