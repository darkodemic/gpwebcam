package main

import (
	"testing"

	"github.com/darkodemic/gpwebcam/internal/usbnet"
)

func TestModelName(t *testing.T) {
	for product, want := range map[string]string{
		"HERO13 Black":        "GoPro HERO13 Black",
		"GoPro HERO9":         "GoPro HERO9",
		"":                    "GoPro",
		"HERO'; rm -rf /%{x}": "GoPro HERO rm -rf x",
	} {
		if got := modelName(usbnet.Interface{Product: product}); got != want {
			t.Errorf("modelName(%q) = %q, want %q", product, got, want)
		}
	}
}
