# 0005 — The name gpwebcam and packages through GoReleaser

- **Status:** Accepted 2026-10-05. The program, the package and the Go module are named `gpwebcam`; packages (`.deb`, `.rpm`, Arch, `tar.gz`) are built with GoReleaser following the layout from `packaging-and-release.md` §3, and v4l2loopback is configured with a file in `/usr/lib/modprobe.d`.
- **Date:** 2026-10-05
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `packaging-and-release.md` (research and options); ADR 0003 (gw owns the loopback device and runs as a user service); ADR 0004 (Apache-2.0 license)

## Context

The project is heading toward a public release through distro packages (`packaging-and-release.md`). Research from 2026-10-05 showed:

- The name `gw` is taken: two AUR packages use it, with `/usr/bin/gw`, and in Debian `greaseweazle` does (§2).
- Distributions require `/usr/lib/...` for files from packages, forbid writing to `/etc` and `$HOME`, and do not enable user services by themselves (§3).
- v4l2loopback is shared by OBS and other programs; RPM Fusion already ships its own `98-v4l2loopback.conf` (§4).

## Decision

1. **The name is `gpwebcam`**: the binary, the package, the systemd unit, the Go module `github.com/darkodemic/gpwebcam` and the future GitHub repo. It is free in the Arch repos, the AUR, Debian and Fedora, and it does not contain the full trademarked word "GoPro". Documents written before 2026-10-05 still say `gw`; it is the same program.
2. **Packages are built with GoReleaser 2.18.2** (pinned in `mise.toml`), which builds them through nFPM: `.deb` and `.rpm` from one entry, Arch from another because of the short description, plus `tar.gz` and `checksums.txt`. Local build without publishing: `goreleaser release --snapshot --clean`.
3. **Layout**: `/usr/bin/gpwebcam`, `/usr/lib/systemd/user/gpwebcam.service`, `/usr/lib/modules-load.d/gpwebcam.conf`, `/usr/lib/modprobe.d/99-gpwebcam.conf`, `/usr/share/doc/gpwebcam/README.md`; the license in `/usr/share/licenses/gpwebcam/` (Arch, rpm) or as DEP-5 `/usr/share/doc/gpwebcam/copyright` (deb). Nothing in `/etc` or `$HOME`.
4. **Module (option A)**: `options v4l2loopback devices=2 video_nr=42,-1 card_label="GoPro,OBS Virtual Camera" exclusive_caps=1,1`. The second device stays for OBS; the administrator changes or disables the setting with a file of the same name in `/etc/modprobe.d/`.
5. **`gpwebcam` finds the device by name** "GoPro" in `/sys/class/video4linux/*/name`; `-video-nr` selects it by number, and `-device-label` changes the name it looks for.
6. **The service is not enabled from the package**. The post-install script only prints what the user has to do (`systemctl --user enable --now gpwebcam.service`).
7. **Dependencies**: deb `Depends: ffmpeg`, `Recommends: v4l2loopback-dkms | v4l2loopback-modules`, `Suggests: v4l2loopback-utils`; rpm `Requires: /usr/bin/ffmpeg`, `Recommends: v4l2loopback`; Arch `depends=(ffmpeg)` (nFPM cannot do `optdepends`, so the AUR gets a hand-written PKGBUILD).
8. **`go.mod` requires `go 1.22`**, so the package can also be built in Debian trixie (Go 1.24). Verified 2026-10-05: `go vet` and all tests pass with go1.22.12.

## Consequences

**Positive**

- The packages follow distribution rules from day one, so submitting to the AUR, COPR and Debian later does not need a new layout.
- OBS and other users of v4l2loopback still have their own device.
- An administrator changing the device number does not break the service.

**Negative**

- The file in `/usr/lib/modprobe.d` changes the module's default behavior for all programs, and takes effect only after a reboot or a reload of the module.
- The name `gpwebcam` also has to be applied to the local project directory and to the GitHub repo; Darko does that.
- The Arch package from GoReleaser has no `optdepends`.

**Risks**

- `card_label` with a comma and a space depends on how v4l2loopback splits the string; this has to be checked after the first load of the module with the new setting (`cat /sys/class/video4linux/*/name`).

## Alternatives considered

- **The names `herocam` and `gopro-webcam`**: both free, but they contain GoPro's trademarks; `gopro-webcam` also has 16 repos of the same name on GitHub.
- **nFPM alone**: builds packages, but with no build, no GitHub release and no AUR publishing.
- **Option B, a root oneshot with `v4l2loopback-ctl add`**: does not touch other programs' options, but brings in root at runtime.
- **Enabling the service from the package (preset or `systemctl --global enable`)**: Arch and Fedora do not do that, and the first logged-in user would take the device for everyone else.
