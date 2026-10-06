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
		{stream.ErrNoVideo, 1, placeholder.Retrying(m)},
		{stream.ErrNoVideo, noVideoHint, placeholder.NoVideo},
		{fmt.Errorf("start: %w", camera.ErrNoAnswer), 0, placeholder.NotAnswering},
		{fmt.Errorf("start: %w", camera.ErrCannotCapture), 0, placeholder.CannotCapture},
		{errors.New("something else"), 0, placeholder.Problem},
	} {
		if got := retryStatus(tc.err, tc.noVideo, m); got != tc.want {
			t.Errorf("retryStatus(%v, %d) = %q, want %q", tc.err, tc.noVideo, got, tc.want)
		}
	}
}
