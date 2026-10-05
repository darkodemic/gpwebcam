# Contributing to gpwebcam

This file is for people who build, test or change gpwebcam. To install and use it, see [README.md](README.md).

## Tools

- **Go.** `go.mod` requires Go 1.22, so Debian stable can build the package; any newer Go works too. Only the standard library is used.
- **ffmpeg**, for the tests that run the real ffmpeg and for running gpwebcam.
- **GoReleaser**, for building packages. It is pinned in `mise.toml`; with [mise](https://mise.jdx.dev), `mise install` in the repository installs it.
- For trying it with a camera: the v4l2loopback module and a GoPro; see the README.

## Layout

| Path | What it holds |
|---|---|
| `cmd/gpwebcam` | The command line: `run`, `start`, `list`, `version`; the session loop and the watchdog. |
| `internal/usbnet` | Finds GoPro network interfaces by USB vendor ID `2672` in sysfs and waits for their IPv4 address. |
| `internal/camera` | Open GoPro HTTP client: webcam start, stop, status, keep-alive. |
| `internal/stream` | Runs ffmpeg, which decodes the camera's MPEG-TS stream and hands raw frames over a pipe. |
| `internal/feed` | Keeps the loopback device supplied: live frames, or the placeholder between them. |
| `internal/placeholder` | Renders the "Camera not connected" picture. |
| `internal/v4l2` | Opens the v4l2loopback device, sets its format and finds it by label. |
| `packaging/` | Files the packages install: systemd user unit, module configuration, Debian copyright, post-install message. |
| `.goreleaser.yaml` | Builds binaries, archives and `.deb`, `.rpm` and Arch packages. |
| `docs/plans-and-decisions/` | Design notes, decisions (numbered files) and test results, written in Serbian. |

## Build and test

```sh
go build -o gpwebcam ./cmd/gpwebcam
go test ./...
go test -race ./...
```

- Some tests start the real ffmpeg on the loopback network interface; they are skipped when ffmpeg is missing. No camera is needed.
- To embed a version: `go build -ldflags "-X main.version=$(git describe --always --dirty)" -o gpwebcam ./cmd/gpwebcam`.
- To check that the code still builds with the oldest supported Go: `mise exec go@1.22 -- go test ./...`.

### Run it against a camera

Only one program can write to the loopback device, so stop the service first:

```sh
systemctl --user stop gpwebcam
./gpwebcam run
```

## Packages

Build every package locally without publishing anything:

```sh
goreleaser release --snapshot --clean
```

The packages land in `dist/`: `.deb`, `.rpm` and Arch `.pkg.tar.zst` for amd64 and arm64, plus `tar.gz` archives and `checksums.txt`. To look inside them without installing:

```sh
bsdtar -tvf dist/gpwebcam-*-x86_64.pkg.tar.zst                          # Arch
bsdtar -xOf dist/gpwebcam-*-x86_64.pkg.tar.zst .PKGINFO
ar p dist/gpwebcam_*_amd64.deb control.tar.gz | tar -xzO ./control      # Debian
bsdtar -tvf dist/gpwebcam-*.x86_64.rpm                                  # RPM
```

The packages follow the distributions' rules (ADR 0005, `docs/plans-and-decisions/packaging-and-release.md`):

- Files go only under `/usr`: `/usr/bin`, `/usr/lib/systemd/user`, `/usr/lib/modules-load.d`, `/usr/lib/modprobe.d`, `/usr/share/doc`, `/usr/share/licenses`. Nothing goes into `/etc` or a home directory.
- The package never enables or starts the user service, and never loads the kernel module; the post-install script only prints instructions.

## Rules for the code

- **No root at runtime.** gpwebcam runs as the logged-in user. It never loads, unloads or reloads v4l2loopback, because other programs such as OBS may use it.
- **No shell.** ffmpeg is started with `os/exec` and an argument list.
- **ffmpeg listens only on the host's address on the GoPro interface**, never on `0.0.0.0`.
- **Every input is validated**: resolution and field of view are enums, ports and device numbers are range-checked, interface names are checked before they reach a sysfs path.
- **Every HTTP request to the camera has a timeout.** On exit gpwebcam stops the camera's stream and ffmpeg.
- **The network interface is never guessed**: it is found by USB vendor ID in sysfs.
- **Standard library first.** Add a dependency only when it saves real work.
- **Code comments and documentation in the code are in English.**

## Design notes

`docs/plans-and-decisions/` records what was decided, why, and what was measured: camera behavior, the Open GoPro API, latency measurements, packaging research. Start with `gopro-fork-review-and-handover.md`. When a change alters what a plan says, update the plan in the same change.

## License

gpwebcam is licensed under the [Apache License 2.0](LICENSE). Contributions are accepted under the same license.
