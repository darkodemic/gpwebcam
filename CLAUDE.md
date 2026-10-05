# gw

`gw` makes a GoPro connected over USB usable as a regular Linux webcam, exposed through v4l2loopback as `/dev/video42`. First target: HERO13 Black, firmware 02.10.

It is a from-scratch rewrite. The old bash tool in `~/Projects/gopro_as_webcam_on_linux` (a fork of `jschmid1/gopro_as_webcam_on_linux`) is reference material only. Do not copy code from it: it is Apache-2.0, and its design is what we are replacing.

## Start here

1. `docs/plans-and-decisions/gopro-fork-review-and-handover.md` covers what we know about the camera, the review findings on the old tool, the design principles, open decisions, and "Gde smo i šta sledi" (current state and next steps). Read it at the start of a session.
2. `docs/plans-and-decisions/0001-go-as-implementation-language.md` records why the project uses Go.

When work changes what a plan says, update that plan in the same session.

## Rules

- Go, a single binary, standard library first. The toolchain comes from mise (global `go latest`, currently 1.26.8).
- No root at runtime. Root is needed only to install the module config in `/etc/modules-load.d/` and `/etc/modprobe.d/`, plus the udev and systemd files. Never unload or reload v4l2loopback, because other devices (OBS) may use it.
- Never run anything through a shell. Start ffmpeg with `os/exec` and an argument list.
- ffmpeg listens only on the host's IP address on the GoPro interface, never on `0.0.0.0`.
- Validate every input: enums for resolution and FOV, range-checked numbers for port and device number.
- Give every HTTP call to the camera a timeout. On SIGTERM, send STOP to the camera and stop ffmpeg.
- Do not guess the network interface. Take it from udev, or find it by USB vendor ID `2672` in sysfs.
