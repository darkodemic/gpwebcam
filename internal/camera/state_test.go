package camera

import (
	"errors"
	"testing"
)

func TestParseStatus(t *testing.T) {
	r, err := parseStatus("status", []byte(`{"status":2,"error":0}`))
	if err != nil || r.Status != StatusHighPowerPreview || !r.Status.Streaming() {
		t.Errorf("streaming reply = %+v, %v", r, err)
	}

	// HERO13, OpenGoPro issue #818.
	r, err = parseStatus("status", []byte(`{"status":4,"error":7}`))
	var re *ReplyError
	if !errors.As(err, &re) || re.Code != 7 || r.Status != StatusUnavailable {
		t.Errorf("unavailable reply = %+v, %v", r, err)
	}

	for _, body := range []string{``, `{}`, `{"status":2}`, `[]`, `OK`, `{"status":"2","error":0}`} {
		if _, err := parseStatus("status", []byte(body)); err == nil || errors.As(err, &re) {
			t.Errorf("parseStatus(%q) = %v, want a format error", body, err)
		}
	}
}

func TestCheckCommand(t *testing.T) {
	for _, ok := range []string{``, `{}`, ` {} `, `{"status":2,"error":0}`, `{"error":0}`} {
		if err := checkCommand("start", []byte(ok)); err != nil {
			t.Errorf("checkCommand(%q) = %v", ok, err)
		}
	}
	// Upstream gopro_as_webcam_on_linux issue #28: reported as success.
	var re *ReplyError
	if err := checkCommand("start", []byte(`{"status":1,"error":1}`)); !errors.As(err, &re) || re.Code != 1 {
		t.Errorf("error reply: %v", err)
	}
	if err := checkCommand("start", []byte(`<html>`)); err == nil || errors.As(err, &re) {
		t.Errorf("HTML reply: %v, want a format error", err)
	}
}
