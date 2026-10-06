# Release 0.1.0

- **Status:** Tag `v0.1.0` 2026-10-06, the draft release waits for Darko to publish it. Left from M1: the service at login after a restart, and Zoom with the package; if something does not work, the fix goes into 0.1.1. Scope decided 2026-10-05: M1 to M4, F1, F2, F4, O1 to O4; F3 (recording) goes into 0.2.0 together with the tray icon (`tray-and-recording.md`).
- **Date:** 2026-10-05
- **Owner:** Darko
- **Related:** `packaging-and-release.md` §6 (channels), §7; `second-slice-gw-run.md` §3.1, §5; ADR 0005 (the name gpwebcam and packages through GoReleaser); handover note §10 (ideas)

## 1. Required before the tag

| # | Item | Why |
|---|---|---|
| M1 | Installing the Arch package, `systemctl --user enable --now gpwebcam.service`, restart; checking that modprobe.d gives `/dev/video42` "GoPro" and a second device "OBS Virtual Camera", that the service starts at login, and that Zoom sees it | the package and the service from the package have not been tried together yet; `card_label` with a comma and a space is not verified (ADR 0005, Risks) |
| M2 | Unplug and plug back the cable once, with the start-then-stop trick | the trick is verified only on a fake camera |
| M3 | Waiting for the device "GoPro" instead of exiting when the module is not loaded yet | now the service exits and systemd restarts it every 10 s, endlessly, with a message in the journal every time; at boot too the service can start before the module |
| M4 | GitHub repo `darkodemic/gpwebcam`, CI (vet, test, package build on every push), GoReleaser on tag `v0.1.0`, first as a draft release | without it there is no public release; every step needs Darko's approval |

## 2. Features, optional

| # | Item | Estimate | Note |
|---|---|---|---|
| F1 | `gpwebcam doctor`: module and device names, device permissions, ffmpeg and the H.264 decoder (Fedora `ffmpeg-free`), the camera on USB and its mode, IPv4 address and NetworkManager "shared", firewall, service state | medium | the biggest benefit for support: most upstream reports are misconfiguration (`upstream-issues-review.md` §2, item 13) |
| F2 | Desktop notifications: camera connected, disconnected, error | small with `notify-send` (no dependency) | handover note §10.1; the user service has the session D-Bus |
| F3 | Recording: a copy of the H.264 stream into `.mkv` alongside the webcam, without re-encoding | medium | handover note §10.2 |
| F4 | Man page `gpwebcam.1` | small | Debian lintian warns without it; Debian needs it later anyway |

## 3. Optimizations and polish, optional

| # | Item | Estimate | Note |
|---|---|---|---|
| O1 | Quieter log: ffmpeg warnings at every start (stream 2 and 3, `yuvj420p`), the rename `eth0` → `enp...` as info instead of a false "unplugged" | small | purely cosmetic, but the journal is what the user reads |
| O2 | Measure CPU usage at 1080p30, then hardware decoding (VAAPI) if needed | measuring small, VAAPI medium | decoding is done in software now |
| O3 | Two applications at once: the second gets "Device or resource busy" (verified 2026-10-05 with two ffmpeg readers) | unknown | may be a v4l2loopback limitation; investigate first, otherwise record it as a limitation in the README. Outcome: a V4L2 API rule; recorded in the README; a second device "GoPro 2" after 0.1.0 (§4) |
| O4 | More states on the placeholder: "camera found, waiting for network", "firewall?" | small | the user sees what is happening without the journal |

## 4. After 0.1.0

- 0.2.0: tray icon with settings, and recording (F3), `tray-and-recording.md`.
- AUR: a source PKGBUILD `gpwebcam` (with `optdepends`) and `gpwebcam-bin` through GoReleaser `aurs` (`packaging-and-release.md` §6).
- COPR for Fedora; Debian ITP; RPM Fusion.
- Several cameras at once.
- A second device "GoPro 2" that `gpwebcam` writes the same frames into, for two applications at once (e.g. streaming); needs a third v4l2loopback device in the module config.
- RTSP as an alternative for networks with a firewall (direct gave the same latency, 0.18 s).

## 5. Where we are and what is next

- 2026-10-05: list made after commit `6effe4d`.
- 2026-10-05: Darko chose F1, F2, F4 and O1 to O4; F3 after 0.1.0. Question with O2: why hardware decoding is not the default when it exists. Answer: it should be, if (1) it does not increase latency, because some VAAPI drivers hold more frames, and the frame has to go back to system memory anyway because of v4l2loopback, and (2) it falls back to software on its own when the GPU or the driver is not available (`-hwaccel auto` does that). O2 measures CPU and latency with and without it, and decides the default on that basis.
- 2026-10-05, done (tests pass, also with `-race`):
  - M3: `gpwebcam run` waits for the device (check every 2 s) instead of exiting.
  - O1: ffmpeg with `-loglevel error`; its lines go through `slog` with the tag `source=ffmpeg`, at most 20 per minute, with the count of skipped ones. An interface rename is recognized by the USB device still existing (a second check after 300 ms, because on unplug the interface can disappear before the device) and is logged as info.
  - O4: seven messages on the placeholder (not connected, waiting for network, starting, no video, after 3 times "firewall?", not responding, problem); all verified visually at 1080p.
  - F2: `internal/notify` through `notify-send`, the same message at most once per 30 s; `-notify=false` turns it off. On the test machine the server is Quickshell.
  - F1: `gpwebcam doctor`: ffmpeg and the H.264 decoder, module, devices, module config, write permission, service, camera (USB, interface, IPv4, route, HTTP info and status, read-only), firewalld and ufw, notify-send. Exit 1 if something fails.
  - F4: `packaging/man/gpwebcam.1`, without groff warnings; GoReleaser compresses it (`gzip -n`) into `/usr/share/man/man1/` in all three packages.
  - O2: local stream 1080p30 H.264 High, 6 Mb/s: software 13.7 % of one core and 71 ms, `-hwaccel vaapi` 9.5 % and 74 ms, `-hwaccel auto` (chose VAAPI on AMD) 8.7 % and 74 ms. Decision: `-hwdec auto` by default, `-hwdec none` turns it off; after two consecutive "no video" with the GPU, sessions switch to software for the rest of the run.
  - O3: investigated in the v4l2loopback 0.15.4 source code. Since 0.14, one reader per device: the second fails on `VIDIOC_S_FMT` (`v4l2loopback.c:1143-1146`, and `REQBUFS` at `:1725-1728`), regardless of the format; the same holds for UVC cameras, and it is a V4L2 API rule (upstream #635, #310). Chrome does `S_FMT` without retrying. The only solution is a second device ("GoPro 2") that `gpwebcam` writes the same frames into; for now recorded in the README as a limitation, and the second device waits for Darko's decision.
- 2026-10-05: Darko: "GoPro 2" after 0.1.0; he does not see a common case for two applications at once, except maybe for streaming.
- 2026-10-05, M1 partly: Darko installed the Arch package (`pacman -Ql` shows all 7 files) and reloaded the module without a restart (`sudo modprobe -r v4l2loopback && sudo modprobe v4l2loopback`, no program was using the module). The config from the package gives `/dev/video42` "GoPro" and `/dev/video2` "OBS Virtual Camera", `exclusive_caps=Y,Y`; `card_label` with a comma and a space splits correctly (the risk from ADR 0005 is gone). `systemctl --user enable --now gpwebcam.service` works; the service finds the device by name and writes the placeholder; `gpwebcam doctor`: 0 problems, only a warning that the camera is not plugged in. Left: the service at login after a restart, and Zoom with the package.
- 2026-10-05, M2: Darko confirmed that the whole test works with the service from the package: the camera is found and started on its own, unplugging and plugging back the cable work, notifications arrive.
- 2026-10-05, after commit `44230e4`, Darko's two ideas:
  - The model in the camera name: `card_label` is set when the module loads and does not change without root (new device), and the device exists without the camera too. Decision: the device stays "GoPro"; the model ("GoPro HERO13 Black", from the USB product string, sanitized because it comes from the device) goes onto the placeholder, into the notifications and into the log. A model outside the list of verified ones (now only "HERO13 Black") is reported once in the log and with a notification, and `doctor` flags it with a warning. The README describes how to set a permanent name through `/etc/modprobe.d` and `-device-label`.
  - Dots: waiting states (network, starting, retry) cycle ".", "..", "..." every 0.5 s; problem messages stay still. Spaces in place of the dots do not help, because drawtext does not count trailing spaces in `text_w` (the text moved 6 to 7 px per dot); so the status is centered without the dots, and the dots are drawn right after the measured right edge of the text, on the same baseline (`y_align=baseline`, ffmpeg 6.1+). One base image is kept, plus a strip of rows per status and step: about 12 MB instead of the earlier ~22 MB. Verified live with the camera.
  - Along the way: `-hwaccel auto` tries CUDA first, and on a machine without the NVIDIA driver it writes 3 error lines at every start. Now `-hwdec auto` probes the VAAPI device once at start (`-init_hw_device vaapi`, about 50 ms) and uses `-hwaccel vaapi` or software. An explicit `-hwaccel vaapi` does not fall back to software on its own when the device does not work (verified), so the check runs up front.
- 2026-10-05, after commit `8ee39fa`: when a new session sends START less than a second after the previous one's `exit` (service restart), the camera sometimes reports status 2 but sends nothing; the watchdog then brought the picture back only after about 16 s. A repeated START does not help, because the camera ignores it in the streaming state (1 of 4 quick restarts, picture after 15.7 s). Fix: 3 s after START without a single frame, the session sends stop then START while ffmpeg is still listening; the 6 s watchdog stays as the last safeguard. Measurement with 16 quick restarts: 15 normal (picture 4.4 s after starting `gpwebcam run`), 1 failure that the fix recovered in 8.9 s without the watchdog.
- 2026-10-05: commit `73eca79` (quick restart). Prepared `.github/workflows/ci.yml` (gofmt, vet, `go test -race` with ffmpeg on Go 1.22 and the latest, snapshot package build as an artifact) and `release.yml` (tag `v*` → GoReleaser 2.18.2 → draft release); actions pinned to a commit (checkout v7.0.1, setup-go v7.0.0, goreleaser-action v7.2.3, upload-artifact v7.0.1, verified through the GitHub API); `actionlint` 1.7.12 with `shellcheck` 0.11.0 without findings. The `.deb` and `.rpm` packages are tested in containers (Debian trixie, Ubuntu 24.04, Fedora, Arch).
- M4, steps (each with Darko's approval): (1) GitHub repo `darkodemic/gpwebcam`; (2) `origin` and push `main`; (3) CI green; (4) annotated tag `v0.1.0` and push of the tag; (5) Darko reviews the draft release and publishes it.
- 2026-10-05, M4 steps 1 to 3: commit `b9c454c`; private repo https://github.com/darkodemic/gpwebcam (Darko: private first, public later); push `main`. First CI: "Test (Go stable)" passed in 2 min; "Test (Go 1.22)" and "Build packages" canceled twice after 15 min without a runner ("The job was not acquired by Runner of type hosted even after multiple attempts"). Cause: GitHub Actions "major outage", incident since 21:09 UTC. The rerun waits for the recovery.
- 2026-10-05, packages in containers (Debian 13, Ubuntu 24.04, Fedora 44, Arch): install, `version`, `help`, `doctor` and removal work everywhere; files root:root, 0755/0644. Findings and fixes:
  - Fedora `ffmpeg-free` has only `libopenh264`, not the built-in `h264`; `doctor` reports that and suggests RPM Fusion.
  - After removal, rpm left empty `/usr/share/doc/gpwebcam` and `/usr/share/licenses/gpwebcam`: added as `type: dir`.
  - lintian: `no-changelog` → `packaging/changelog.yml` (nFPM `changelog`, `release: "1"`); `statically-linked-binary` → GoReleaser `deb.lintian_overrides`; `maintainer-script-ignores-errors` → `set -e`. Only `initial-upload-closes-no-bugs` remains.
  - rpmlint: README and man page as `type: doc`; changelog. `statically-linked-binary` and `position-independent-executable-suggested` remain (the static build stays), and for a snapshot `incoherent-version-in-changelog`.
  - namcap: `pacman -Qkk` reported a time difference for every file → `mtime: "{{ .CommitDate }}"`, now 0 modified files. RELRO/PIE (static build) and "owned by 0:0" remain (nFPM does not write owner names; harmless).
  - `doctor`: a clear message when user systemd is not available (sudo, container) and when `modprobe` does not exist.
  - The snapshot version is now `0.0.1~dev.<commit>`, so in every package manager it sorts before the next release.
  - Message after install: reboot or reload only if the module was loaded before the install (on Arch the systemd hook loads it during the install); added the step `gpwebcam doctor`.
- 2026-10-05: commit `6ae5b5c` pushed; CI green on all three jobs after GitHub Actions recovered (23:35).
- 2026-10-05: Darko asked why Go is not in `mise.toml`. It was an oversight: the global `go latest` gave 1.26.8, and the newest is 1.27.1. Now `mise.toml` pins Go 1.27.1 along with GoReleaser 2.18.2; `go.mod` stays at `go 1.22` as the minimum. With go1.27.1 (through `mise exec`, because the session's shell has Go in PATH outside `mise.toml`), `vet`, `go test -race` and the package build pass, and the binary carries go1.27.1. The CI test and package jobs and the release workflow now install the tools from `mise.toml` (`jdx/mise-action` v5.1.1, pinned to a commit); the test on Go 1.22 stays on `setup-go`.
- 2026-10-06: Darko decided that the tag goes before the restart; the rest of M1 is checked afterwards, and a problem means 0.1.1. Signed annotated tag `v0.1.0` on `ea677f5` (CI green on all three jobs), push of the tag only. The release workflow passed in 49 s; the draft has the `.deb`, `.rpm` and Arch packages for amd64 and arm64, two `tar.gz` archives and `checksums.txt`. Verified: the checksum of the amd64 archive matches, the binary reports `gpwebcam 0.1.0`. GoReleaser `changelog.sort: asc` orders commits alphabetically by title, not by time, and with the full hash; the release text needs cleaning up before publishing, and the setting needs changing for the next release.
- Next: Darko publishes the draft; restart when it suits him (service at login, module at boot, Zoom with the package). The tray icon and recording are the plan for 0.2.0 (`tray-and-recording.md`).
