# 0001 — Go as the implementation language

- **Status:** Accepted 2026-09-29. `gw` is written in Go.
- **Date:** 2026-09-29
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `gopro-fork-review-and-handover.md` §3 (review findings), §4 (principles), §6 (open decisions)

## Context

`gw` does a small number of things, but each one has to be done carefully:

- finds the GoPro network interface by USB vendor ID, or gets it from udev;
- waits for an IPv4 address on that interface;
- sends a few HTTP calls to the camera, each with a timeout;
- starts ffmpeg, supervises it and stops it;
- sends STOP to the camera on SIGTERM;
- validates all inputs.

It runs as a systemd service without root.

The old bash script from the fork had exactly the bugs that bash makes easy: arithmetic evaluation of an argument in `[[ -ne ]]`, word splitting in the `modprobe` command, and unvalidated text in the ffmpeg filtergraph (handover note §3.1).

## Decision

1. `gw` is a single Go binary, with no runtime dependencies except ffmpeg.
2. ffmpeg is started through `os/exec` with an argument list, never through a shell.
3. The standard library comes first: `net/http` with timeouts, `os/signal`, `context`. An external dependency comes in only when it saves real work.

## Consequences

**Positive**

- The whole class of shell injections goes away, because there is no shell.
- Inputs are types (enums for resolution and FOV, range-checked numbers), so an invalid value does not reach ffmpeg or the camera.
- Packaging is simple: one file, easy for a PKGBUILD.

**Negative**

- The build needs a Go toolchain, while the script could be run right away.
- The binary is a few MB instead of a few KB.

**Risks**

- ffmpeg still parses network input. Mitigated by running without root and listening only on the GoPro interface (handover note §4).

## Alternatives considered

- **Bash:** the fastest start, but the same kinds of bugs as in the fork.
- **Python:** good, but needs an interpreter and packaging of dependencies on the target machine.
- **Rust:** safe, but too much ceremony for such a small tool.
- **Go without ffmpeg** (own MPEG-TS demux and H.264 decoding): a lot of work with no benefit to the user.

## Out of scope

- GUI and tray icon.
- GStreamer as a replacement for ffmpeg.
