# Contributing to gpwebcam

This file is for people who build, test or change gpwebcam. To install and use it, see [README.md](README.md).

## Tools

- **Go.** `mise.toml` pins the Go that builds releases (currently 1.27.1); `go.mod` only requires Go 1.22, the oldest version the code supports, so Debian stable can build the package. Besides the standard library, only the tray icon has dependencies: `fyne.io/systray` and, through it, `github.com/godbus/dbus/v5`. Both are pure Go, so the binary stays static.
- **ffmpeg**, for the tests that run the real ffmpeg and for running gpwebcam.
- **GoReleaser**, for building packages, also pinned in `mise.toml`.
- With [mise](https://mise.jdx.dev), `mise install` in the repository installs both. If your shell does not activate mise, prefix commands with `mise exec --`, e.g. `mise exec -- go test ./...`. CI uses the same `mise.toml`.
- For trying it with a camera: the v4l2loopback module and a GoPro; see the README.

## Layout

| Path | What it holds |
|---|---|
| `cmd/gpwebcam` | The command line: `run`, `start`, `config`, `doctor`, `list`, `version`; the session loop, the watchdog, the ffmpeg log filter, camera on demand (`demand.go`), restart through systemd's D-Bus API (`systemd.go`), and the live settings (the file, flags on top, a watcher that applies changes while `run` keeps going). |
| `internal/settings` | The settings that the tray menu and `gpwebcam config` change, with their checks, saved as JSON in `~/.config/gpwebcam/settings.json`. |
| `internal/tray` | The tray icon and its menu through `fyne.io/systray` (StatusNotifierItem over D-Bus), and the icons, drawn in code. |
| `internal/usbnet` | Finds GoPro network interfaces by USB vendor ID `2672` in sysfs and waits for their IPv4 address. |
| `internal/camera` | Open GoPro HTTP client: webcam start, stop, status, keep-alive. |
| `internal/stream` | Runs ffmpeg, which decodes the camera's MPEG-TS stream and hands raw frames over a pipe. |
| `internal/feed` | Keeps the loopback device supplied: live frames, or the placeholder between them. |
| `internal/placeholder` | Renders the placeholder: one base frame with the title, plus a band of rows per status line and animation step, so animated dots cost little memory. |
| `internal/notify` | Desktop notifications through `notify-send`, rate limited. |
| `internal/v4l2` | Opens the v4l2loopback device, sets its format, finds it by label, and follows v4l2loopback's client usage event, which says whether an application streams from the device. |
| `packaging/` | Files the packages install: systemd user unit, module configuration, man page, Debian copyright, post-install message. |
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
- The man page is `packaging/man/gpwebcam.1`; preview it with `man -l packaging/man/gpwebcam.1` and check it with `groff -man -ww -z packaging/man/gpwebcam.1`. GoReleaser compresses it before packaging.

### Run it against a camera

Only one program can write to the loopback device, so stop the service first:

```sh
systemctl --user stop gpwebcam
./gpwebcam run
```

The tray icon of a gpwebcam started this way uses the same settings file as the service. To look at the icon's menu without a panel, find gpwebcam's name on the session bus with `busctl --user list | grep gpwebcam`, then call `busctl --user call <name> /StatusNotifierItem/menu com.canonical.dbusmenu GetLayout iias -- 0 -1 0`.

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

The linters are clean apart from known, accepted findings: lintian `initial-upload-closes-no-bugs` (only for uploads to the Debian archive), rpmlint `statically-linked-binary` and namcap's RELRO and PIE warnings (the binary is static on purpose), and namcap's "owned by 0:0" (nFPM leaves owner names empty; pacman installs as root). Snapshot builds also get rpmlint `incoherent-version-in-changelog`, because the changelog names the next release.

The packages follow the distributions' rules (ADR 0005, `docs/plans-and-decisions/packaging-and-release.md`):

- Files go only under `/usr`: `/usr/bin`, `/usr/lib/systemd/user`, `/usr/lib/modules-load.d`, `/usr/lib/modprobe.d`, `/usr/share/doc`, `/usr/share/licenses`. Nothing goes into `/etc` or a home directory.
- The package never enables or starts the user service, and never loads the kernel module; the post-install script only prints instructions.

## CI and releases

- `.github/workflows/ci.yml` runs on every push to `main` and every pull request: `gofmt`, `go vet` and `go test -race` with ffmpeg, on Go 1.22 and on the Go from `mise.toml`, then a snapshot build of all packages with the tools from `mise.toml`, kept as a workflow artifact.
- `.github/workflows/release.yml` runs when a tag `v*` is pushed: GoReleaser from `mise.toml` builds the packages and creates a **draft** GitHub release with them, the archives and `checksums.txt`. A maintainer checks the draft and publishes it.
- Actions are pinned to commits; Dependabot or a manual update moves them.
- Check workflow changes locally with `mise exec actionlint@1.7.12 shellcheck@0.11.0 -- actionlint`.

To release: add an entry for the version to `packaging/changelog.yml` (it becomes the Debian changelog and the RPM `%changelog`), make sure `main` is green, then tag and push:

```sh
git tag -a v0.1.0 -m "gpwebcam 0.1.0"
git push origin v0.1.0
```

## Rules for the code

- **No root at runtime.** gpwebcam runs as the logged-in user. It never loads, unloads or reloads v4l2loopback, because other programs such as OBS may use it.
- **No shell.** ffmpeg is started with `os/exec` and an argument list.
- **ffmpeg listens only on the host's address on the GoPro interface**, never on `0.0.0.0`.
- **Every input is validated**: resolution and field of view are enums, ports and device numbers are range-checked, interface names are checked before they reach a sysfs path.
- **Every HTTP request to the camera has a timeout.** On exit gpwebcam stops the camera's stream and ffmpeg.
- **The network interface is never guessed**: it is found by USB vendor ID in sysfs.
- **Standard library first.** Add a dependency only when it saves real work, and keep it pure Go so the binary stays static.
- **Code comments and documentation in the code are in English.**

## Design notes

`docs/plans-and-decisions/` records what was decided, why, and what was measured: camera behavior, the Open GoPro API, latency measurements, packaging research. Start with `gopro-fork-review-and-handover.md`. When a change alters what a plan says, update the plan in the same change.

## License

gpwebcam is licensed under the [Apache License 2.0](LICENSE). Contributions are accepted under the same license.
