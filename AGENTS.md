# gpwebcam

Instructions for coding agents (Claude Code, Codex and others) working in this repository. `CLAUDE.md` only imports this file, so this is the one place to edit.

`gpwebcam` makes a GoPro connected over USB usable as a regular Linux webcam, exposed through v4l2loopback as the device labelled "GoPro" (`/dev/video42` by default). First target: HERO13 Black, firmware 02.10. Until 2026-10-05 it was called `gw`; older documents use that name. The local directory is `~/Projects/gpwebcam` (renamed from `~/Projects/gw` on 2026-10-05).

It is a from-scratch rewrite. The old bash tool in `~/Projects/gopro_as_webcam_on_linux` (a fork of `jschmid1/gopro_as_webcam_on_linux`) is reference material only. Do not copy code from it: it is Apache-2.0, and its design is what we are replacing.

## Start here

1. `docs/plans-and-decisions/gopro-fork-review-and-handover.md` covers what we know about the camera, the review findings on the old tool, the design principles, open decisions, and "Where we are and what is next" (current state and next steps). Read it at the start of a session.
2. The numbered files in `docs/plans-and-decisions/` are the decisions: Go (0001), the Open GoPro API (0002), gpwebcam owning the loopback device as a user service (0003), the Apache-2.0 license (0004), the name and packaging (0005).
3. `packaging-and-release.md` holds the distro packaging rules and the release path.

When work changes what a plan says, update that plan in the same session.

## Rules

- Go, a single binary, standard library first. `mise.toml` pins Go and GoReleaser for local builds and CI (`jdx/mise-action`); run tools with `mise exec --` when the shell does not activate mise. `go.mod` stays at `go 1.22`, the oldest supported Go, so Debian can build it.
- No root at runtime. Root is needed only to install the package, which ships the module config in `/usr/lib/modules-load.d/` and `/usr/lib/modprobe.d/`. Never unload or reload v4l2loopback, because other devices (OBS) may use it.
- Packages put files only under `/usr` (see ADR 0005), never in `/etc` or `$HOME`, and never enable or start the user service themselves.
- Never run anything through a shell. Start ffmpeg with `os/exec` and an argument list.
- gpwebcam receives the camera's stream only on the host's IP address on the GoPro interface, never on `0.0.0.0`, and only from the camera's address; ffmpeg reads it from a pipe and opens no socket.
- Validate every input: enums for resolution and FOV, range-checked numbers for port and device number.
- Give every HTTP call to the camera a timeout. On SIGTERM, send STOP to the camera and stop ffmpeg.
- Do not guess the network interface. Find it by USB vendor ID `2672` in sysfs.

## Documents for people

- `README.md` is for users: install, setup, use, troubleshooting.
- `CONTRIBUTING.md` is for contributors: tools, layout, build, tests, packages, code rules.
- Keep both in sync with the code: flags, messages, paths and package layout.
- Everything in the repository is in English: design documents in `docs/plans-and-decisions/`, code, comments, README and CONTRIBUTING.
