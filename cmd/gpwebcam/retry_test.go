package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/placeholder"
	"github.com/darkodemic/gpwebcam/internal/stream"
)

func TestRetryStatus(t *testing.T) {
	const m = "GoPro HERO13 Black"
	for _, tc := range []struct {
		err     error
		noVideo int
		want    string
	}{
		{stream.ErrNoPackets, 1, placeholder.Retrying(m)},
		{stream.ErrNoPackets, noVideoHint, placeholder.NoVideo},
		// Datagrams arrive but nothing decodes: not a firewall.
		{fmt.Errorf("%w: 900 datagrams arrived", stream.ErrNoVideo), noVideoHint, placeholder.Retrying(m)},
		{fmt.Errorf("start: %w", camera.ErrNoAnswer), 0, placeholder.NotAnswering},
		{fmt.Errorf("start: %w", camera.ErrCannotCapture), 0, placeholder.CannotCapture},
		{errors.New("something else"), 0, placeholder.Problem},
	} {
		if got := retryStatus(tc.err, tc.noVideo, m); got != tc.want {
			t.Errorf("retryStatus(%v, %d) = %q, want %q", tc.err, tc.noVideo, got, tc.want)
		}
	}
}
