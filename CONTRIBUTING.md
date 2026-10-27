# Contributing to gpwebcam

This file is for people who build, test or change gpwebcam. To install and use it, see [README.md](README.md).

## Tools

- **Go.** `mise.toml` pins the Go that builds releases (currently 1.27.1); `go.mod` only requires Go 1.22, the oldest version the code supports, so Debian stable can build the package. Besides the standard library, gpwebcam has two dependencies: [`github.com/darkodemic/systray`](https://github.com/darkodemic/systray) for the tray icon, our fork of `fyne.io/systray` that adds radio menu items and has its own releases since v1.13.0, and `github.com/godbus/dbus/v5`, which the fork uses and gpwebcam calls directly for systemd and the file manager. Both are pure Go, so the binary stays static.
- **ffmpeg**, for the tests that run the real ffmpeg and for running gpwebcam.
- **GoReleaser**, for building packages, also pinned in `mise.toml`.
- **rsvg-convert** (librsvg), only to change the icon: `go generate ./internal/tray` renders the tray image `internal/tray/icon.png` from `packaging/icons/gpwebcam.svg`. Commit both files.
- With [mise](https://mise.jdx.dev), `mise install` in the repository installs both. If your shell does not activate mise, prefix commands with `mise exec --`, e.g. `mise exec -- go test ./...`. CI uses the same `mise.toml`.
- For trying it with a camera: the v4l2loopback module and a GoPro; see the README.

## Layout

| Path | What it holds |
|---|---|
| `cmd/gpwebcam` | The command line: `run`, `start`, `config`, `doctor`, `list`, `version`; the session loop, the watchdog, the ffmpeg log filter, camera on demand (`demand.go`), restart through systemd's D-Bus API (`systemd.go`), recording (`recording.go`), the control API on a Unix socket for `gpwebcam record` (`control.go`, `recordcmd.go`), the file manager over D-Bus (`filemanager.go`), and the live settings (the file, flags on top, a watcher that applies changes while `run` keeps going). |
| `internal/settings` | The settings that the tray menu and `gpwebcam config` change, with their checks, saved as JSON in `~/.config/gpwebcam/settings.json`. |
| `internal/tray` | The tray icon and its menu through `github.com/darkodemic/systray` (StatusNotifierItem over D-Bus). The icon is the application icon from `icon.png`, with a dot for the state and one for a recording, drawn in code. |
| `internal/usbnet` | Finds GoPro network interfaces by USB vendor ID `2672` in sysfs and waits for their IPv4 address. |
| `internal/camera` | Open GoPro HTTP client: webcam start, stop, status, keep-alive. |
| `internal/stream` | Receives the camera's MPEG-TS datagrams over UDP, watches that they keep coming, and runs ffmpeg, which decodes them from one pipe and hands raw frames back over another. |
| `internal/record` | Copies the camera's MPEG-TS datagrams into a Matroska file with a second ffmpeg (`-c copy`), named by the time, without decoding. |
| `internal/feed` | Keeps the loopback device supplied: live frames, or the placeholder between them. |
| `internal/placeholder` | Renders the placeholder: one base frame with the title, plus a band of rows per status line and animation step, so animated dots cost little memory. |
| `internal/notify` | Desktop notifications through `notify-send`, rate limited. |
| `internal/v4l2` | Opens the v4l2loopback device, sets its format, finds it by label, and follows v4l2loopback's client usage event, which says whether an application streams from the device. |
| `packaging/` | Files the packages install: systemd user unit, module configuration, menu entry, application icon (`icons/gpwebcam.svg`, the source of every icon; `icons/gpwebcam-outline.svg` is a monochrome outline icon kept in reserve and not installed), man page, Debian copyright, post-install message, the AUR install script (`aur/gpwebcam.install`), the `gpwebcam-git` PKGBUILD (`aur/gpwebcam-git/`) and the script that checks it (`aur/check-gpwebcam-git.sh`); the RPM spec for Copr and the script that makes its source archive with the Go modules vendored (`rpm/`). |
| `.goreleaser.yaml` | Builds binaries, archives and `.deb`, `.rpm` and Arch packages. |
| `.packit.yaml` | Packit: Copr builds of the RPM spec for pull requests and releases. |
| `docs/plans-and-decisions/` | Design notes, decisions (numbered files) and test results. |
| `docs/release-notes/` | The notes of each release, `<version>.md`, which the release workflow publishes. |

## Build and test

```sh
go build -o gpwebcam ./cmd/gpwebcam
go test ./...
go test -race ./...
```

- Some tests send a stream over the loopback network interface to the receiver and start the real ffmpeg; they are skipped when ffmpeg is missing. No camera is needed.
- `GPWEBCAM_LATENCY=1 go test -run TestLatency -v ./internal/stream` measures the delay from a local H.264 sender to the frames gpwebcam gets (add `GPWEBCAM_LATENCY_HW=vaapi` for GPU decoding). It needs ffmpeg with libx264 and takes about 20 seconds. The sender's encoding is part of the number, so use it to compare two versions of the code, not as the camera's latency.
- To embed a version: `go build -ldflags "-X main.version=$(git describe --always --dirty)" -o gpwebcam ./cmd/gpwebcam`.
- To check that the code still builds with the oldest supported Go: `mise exec go@1.22 -- env -u GOROOT -u GOBIN GOTOOLCHAIN=local go test ./...`. Without unsetting them, a `GOROOT` exported by an activated mise shell makes Go switch to the newer toolchain silently.
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

The linters are clean apart from known, accepted findings: lintian `initial-upload-closes-no-bugs` (only for uploads to the Debian archive), rpmlint `statically-linked-binary` and `position-independent-executable-suggested` and namcap's RELRO and PIE warnings (the binary is static on purpose), namcap's "Dependency included, but may not be needed ('ffmpeg')" (gpwebcam runs ffmpeg as a program, which namcap cannot see), and namcap's "owned by 0:0" (nFPM leaves owner names empty; pacman installs as root). Snapshot builds also get rpmlint `incoherent-version-in-changelog`, because the changelog names the next release.

The packages follow the distributions' rules (ADR 0005, `docs/plans-and-decisions/packaging-and-release.md`):

- Files go only under `/usr`: `/usr/bin`, `/usr/lib/systemd/user`, `/usr/lib/modules-load.d`, `/usr/lib/modprobe.d`, `/usr/share/applications` (the **GoPro Webcam** menu entry, `packaging/desktop/gpwebcam.desktop`; check it with `desktop-file-validate`), `/usr/share/icons/hicolor/scalable/apps` (the application icon, `packaging/icons/gpwebcam.svg`), `/usr/share/doc`, `/usr/share/licenses`. Nothing goes into `/etc` or a home directory.
- The package never enables or starts the user service, and never loads the kernel module; the post-install script only prints instructions.

## CI and releases

- `.github/workflows/ci.yml` runs on every push to `main` and every pull request: `gofmt`, `go vet` and `go test -race` with ffmpeg, on Go 1.22 and on the Go from `mise.toml`, then a snapshot build of all packages with the tools from `mise.toml`, kept as the workflow artifact `packages`, together with the AUR PKGBUILDs in `aur/`; and a build of the `gpwebcam-git` PKGBUILD in an `archlinux` container, from the commit under test.
- `.github/workflows/release.yml` runs when a tag `v*` is pushed: GoReleaser from `mise.toml` builds the packages, publishes the GitHub release with them, the archives, the source archive, `checksums.txt` and the notes from `docs/release-notes/<version>.md`, and pushes `gpwebcam` and `gpwebcam-bin` to the AUR, and `gpwebcam-git` when its PKGBUILD or install script changed. The job runs in the environment "Arch Linux User Repository", whose secret `AUR_KEY` is an SSH key of the maintainer's AUR account, used for nothing else, and which only tags `v*` can use. A tag with a suffix, such as `v0.4.0-rc1`, becomes a prerelease and is not pushed to the AUR.
- Packit, a GitHub app configured in `.packit.yaml`, builds `packaging/rpm/gpwebcam.spec` in Copr: every pull request in a temporary project of Packit's, shown as a check, and every published release in [`darko/gpwebcam`](https://copr.fedorainfracloud.org/coprs/darko/gpwebcam/), which Fedora users install from. The RPM builds offline from the archive of `packaging/rpm/make-archive.sh`; to try the spec locally, run that script and `rpmbuild -ba` in a Fedora container, or `packit srpm`.
- Actions are pinned to commits; Dependabot or a manual update moves them.
- Check workflow changes locally with `mise exec actionlint@1.7.12 shellcheck@0.11.0 -- actionlint`.

To release:

1. In a pull request: add an entry for the version to `packaging/changelog.yml`, newest first (it becomes the Debian changelog and the RPM `%changelog`); write the release notes in `docs/release-notes/<version>.md`, which the release workflow publishes as they are and without which it stops; and write a release plan, as `docs/plans-and-decisions/release-0.3.1.md`. The documents are written for the release before the tag and do not name the release commit, since the tag records it; nothing needs a documentation change after the release.
2. Check the packages in the pull request's `packages` artifact, including the PKGBUILDs in `aur/`. The release is published as soon as the tag is pushed, so this is the last look before it is public.
3. Merge, make sure `main` is green, then tag and push:

   ```sh
   git tag -s v0.3.1 -m "gpwebcam 0.3.1"
   git push origin v0.3.1
   ```

4. Check the release page, the AUR pages of `gpwebcam` and `gpwebcam-bin`, and the release build in Copr. If the AUR push failed, the GitHub release is already out: fix the cause, then run the release job again (`gh run rerun <run id>`), which rebuilds, replaces the release files and pushes to the AUR.

### The AUR package gpwebcam-git

`gpwebcam-git` builds the newest commit on `main` and finds its version at build time, so AUR helpers pick up new commits without any change on the AUR. Its PKGBUILD, `packaging/aur/gpwebcam-git/PKGBUILD`, changes only when the build does; its dependencies and installed files follow the AUR entries in `.goreleaser.yaml`.

- After changing it, refresh `.SRCINFO` next to it with `makepkg --printsrcinfo > .SRCINFO` on Arch. CI builds the PKGBUILD from the commit under test with `packaging/aur/check-gpwebcam-git.sh` and fails if `.SRCINFO` does not match; the script's header shows how to run it locally.
- The release workflow pushes the PKGBUILD, `.SRCINFO` and `packaging/aur/gpwebcam.install` to the AUR when they differ from what is there, so a change reaches the AUR with the next release.

## Rules for the code

- **No root at runtime.** gpwebcam runs as the logged-in user. It never loads, unloads or reloads v4l2loopback, because other programs such as OBS may use it.
- **No shell.** ffmpeg is started with `os/exec` and an argument list.
- **gpwebcam receives the stream only on the host's address on the GoPro interface**, never on `0.0.0.0`, and only from the camera's address. ffmpeg reads it from a pipe and opens no socket.
- **Every input is validated**: resolution and field of view are enums, ports and device numbers are range-checked, interface names are checked before they reach a sysfs path.
- **Every HTTP request to the camera has a timeout.** On exit gpwebcam stops the camera's stream and ffmpeg.
- **The network interface is never guessed**: it is found by USB vendor ID in sysfs.
- **Standard library first.** Add a dependency only when it saves real work, and keep it pure Go so the binary stays static.
- **Code comments and documentation in the code are in English.**

## Design notes

`docs/plans-and-decisions/` records what was decided, why, and what was measured: camera behavior, the Open GoPro API, latency measurements, packaging research. Start with `gopro-fork-review-and-handover.md`. When a change alters what a plan says, update the plan in the same change.

## License

gpwebcam is licensed under the [Apache License 2.0](LICENSE). Contributions are accepted under the same license.
