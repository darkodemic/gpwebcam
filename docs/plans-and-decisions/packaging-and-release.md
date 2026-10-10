# Packaging and public release

- **Status:** In progress. Decided 2026-10-05: Apache-2.0 license (ADR 0004); the name `gpwebcam`, GoReleaser 2.18.2, the layout from §3 and modprobe.d option A (ADR 0005). Packages are built locally; installation and testing are pending.
- **Date:** 2026-10-05
- **Owner:** Darko
- **Related:** ADR 0003 (gw owns the loopback device and runs as a user service); ADR 0004 (Apache-2.0 license); ADR 0005 (The name gpwebcam and packages through GoReleaser); `second-slice-gw-run.md`

## 1. Goal

Packages that follow distribution rules from the start, even though they first go only to a GitHub release: `.deb`, `.rpm`, Arch `.pkg.tar.zst` and `tar.gz` with checksums. They are first built and installed locally, and later submitted to the distribution channels (§6).

The sources were read on 2026-10-05; the mark [D] means official documentation or package source code, and [I] an inference.

## 2. Name

`gw` is taken (checked 2026-10-05):

- AUR: the package `gw` (genome browser, installs `/usr/bin/gw`) and `gw-tools` (git worktree, also `/usr/bin/gw`) [D].
- Debian sid: `greaseweazle` installs `/usr/bin/gw`, and Debian Policy 10.1 does not allow two programs with the same name [D].
- It is free in the official Arch repos, Fedora and Ubuntu noble.

Free names (Arch, AUR, Debian, Fedora; both the package and the `/usr/bin` file): `gpwebcam`, `herocam`, `gopro-webcam`. "GoPro" and "HERO" are trademarks, so `gpwebcam` is the safest [I]. A related project, `action-webcamd` (Rust, a system service for GoPro), was published on the AUR on 2026-09-30.

## 3. File layout

The same on Arch, Debian and Fedora [D]; `<n>` is the new name:

| File | Path |
|---|---|
| program | `/usr/bin/<n>`, 0755 |
| user service | `/usr/lib/systemd/user/<n>.service`, `ExecStart=/usr/bin/<n> run`; never `%config` on Fedora |
| module loading | `/usr/lib/modules-load.d/<n>.conf` |
| module options | `/usr/lib/modprobe.d/99-<n>.conf`; Debian does not allow `/lib/...` (lintian `aliased-location`) |
| man page | `/usr/share/man/man1/<n>.1.gz`; without it Debian gives a lintian warning |
| menu entry | `/usr/share/applications/<n>.desktop`, checked with `desktop-file-validate` |
| application icon | `/usr/share/icons/hicolor/scalable/apps/<n>.svg`; the package depends on `hicolor-icon-theme`, which owns the directories and has `index.theme`, without which the theme is not searched (namcap reports the missing dependency as an error). The icon cache is refreshed by the distributions' own hooks: on Debian the trigger of `hicolor-icon-theme`, on Arch the pacman hook of `gtk-update-icon-cache`, on Fedora its file trigger (checked in containers 2026-10-08) |
| README | `/usr/share/doc/<n>/README.md` |
| license | Arch: `/usr/share/licenses/<n>/LICENSE` (not required for Apache-2.0, the `licenses` package has it); Fedora: the same, as `%license`; Debian: `/usr/share/doc/<n>/copyright` in DEP-5 format, with a reference to `/usr/share/common-licenses/Apache-2.0` |

A package must not write to `$HOME` or `/etc` (`/etc` is for the administrator; a file of the same name in `/etc` hides the one from `/usr/lib`), and it does not start user services from scripts [D]. It does not enable the service for all users: Arch does not use presets, Fedora enables only services that work without configuration, and `systemd.preset(5)` does not recommend a preset in a package [D]. Also, if the service were enabled for everyone, the first logged-in user would take `/dev/video42`, and the other users' services would restart in a loop [I]. The post-install script only prints what the user has to do.

## 4. Configuring the v4l2loopback module

Facts [D]:

- modprobe.d: all options add up, files are sorted by name across all directories, and a file of the same name in `/etc` hides the one from `/usr/lib`.
- Arch `v4l2loopback-dkms` and Debian `v4l2loopback-dkms`/`-utils` ship no modprobe or modules-load configuration. Debian `-utils` ships a udev rule for `/dev/v4l2loopback` (group `video`).
- RPM Fusion `v4l2loopback` ships `/usr/lib/modprobe.d/98-v4l2loopback.conf` (`exclusive_caps=1 card_label="OBS Virtual Camera"`) and loads the module at boot.
- OBS: if the module is not loaded, it runs `pkexec modprobe v4l2loopback exclusive_caps=1 card_label='OBS Virtual Camera'`; if it is, it takes the first `/dev/video*` that accepts output.
- `v4l2loopback-ctl add` needs the control device, which is root-only.

Options:

| Option | What | For | Against |
|---|---|---|---|
| A | the package ships `modules-load.d/<n>.conf` (`v4l2loopback`) and `modprobe.d/99-<n>.conf`: `options v4l2loopback devices=2 video_nr=42,-1 card_label="GoPro,OBS Virtual Camera" exclusive_caps=1,1` | no root at runtime; the second device stays for OBS; it wins over RPM Fusion's `98-`, and the administrator's file in `/etc` wins over it | changes the module's default behavior for all programs; takes effect only after a reboot or a reload of the module |
| B | a system oneshot service that creates the device as root at boot with `v4l2loopback-ctl add` | does not touch other programs' options | root at runtime, against the principle from handover note §4; needs a new ADR |

Recommendation: A [I]. With it, `gw` should find its device by name ("GoPro" in `/sys/class/video4linux/*/name`) instead of by the number 42, so that an administrator changing the number does not break the service.

## 5. Dependencies

| Distribution | ffmpeg | module |
|---|---|---|
| Arch | `depends=(ffmpeg)` | `optdepends=('v4l2loopback-dkms: ...')`, like obs-studio [D]; nFPM's archlinux package cannot write optdepends (source code `arch/arch.go`), so the AUR needs a hand-written PKGBUILD [D] |
| Debian, Ubuntu | `Depends: ffmpeg` | `Recommends: v4l2loopback-dkms \| v4l2loopback-modules`, `Suggests: v4l2loopback-utils`; Ubuntu noble has the module in `linux-modules-*-generic` [D] |
| Fedora | `Requires: /usr/bin/ffmpeg` [I], because `ffmpeg-free` does not carry the name `ffmpeg` | not in Fedora; RPM Fusion `v4l2loopback` and `akmod-v4l2loopback`; in COPR `Recommends: v4l2loopback` [I] |

Fedora's `ffmpeg-free` is built without an H.264 decoder and relies on `libopenh264` from the Cisco repo, which is enabled, but the package is not installed by default [D]. Whether openh264 decodes the GoPro's High profile stream is not verified; the recommendation is RPM Fusion `ffmpeg`, and `gw doctor` should check `ffmpeg -decoders` [I].

`go.mod` requires Go 1.26.8, and Debian trixie has 1.24 [D]. The code uses nothing newer than Go 1.21 (`log/slog`, `slices`), so the directive should be lowered to `go 1.22`.

## 6. Channels

Order [I]:

1. A GitHub release with packages from GoReleaser (`.deb`, `.rpm`, `.pkg.tar.zst`, `tar.gz`, checksums). Testing on Arch, Debian trixie and Ubuntu noble, and on Fedora with RPM Fusion ffmpeg and with `ffmpeg-free` + openh264.
2. AUR: a hand-written source PKGBUILD (name `<n>`, with optdepends); optionally also `<n>-bin` through GoReleaser `aurs` [D: `-bin` suffix required, `.SRCINFO` with every push, SPDX in `license`].
3. COPR for Fedora [D: a Fedora account is enough].
4. Debian: an ITP bug, then mentors and a sponsor; the Go team names programs without the `golang-` prefix [D]. Ubuntu takes it from Debian. Estimate 1 to 3 months [I]. A PPA only if Ubuntu is needed sooner. Every Go dependency must be a Debian package too: `github.com/darkodemic/systray` is not, so it needs a package of its own first, and Debian has godbus as `golang-dbus` 5.1.0 while `go.mod` asks for v5.2.2 (checked 2026-10-07; `tray-and-recording.md` §10).
5. RPM Fusion: a review in Bugzilla, once COPR is stable [D]. Official Fedora does not accept packages that need an out-of-tree kernel module [D].
6. Optionally openSUSE OBS: one place for apt, yum and pacman repos [D].

GoReleaser 2.18.2 builds nFPM packages and publishes `-bin` to the AUR for free; apt and yum repos are only in the Pro version, and Debian, PPA and COPR have no publisher [D].

## 7. Where we are and what is next

- 2026-10-05: Apache-2.0 license and `LICENSE` (ADR 0004); GoReleaser 2.18.2 through `mise.toml`; research on distribution rules (this document).
- 2026-10-05: Darko chose the name `gpwebcam` and option A (ADR 0005). Done: the rename (`cmd/gpwebcam`, module `github.com/darkodemic/gpwebcam`), `go 1.22` (verified with go1.22.12), the device by name "GoPro" (`-device-label`), `packaging/` (unit with `/usr/bin/gpwebcam`, modules-load, modprobe, DEP-5 copyright, post-install message), `.goreleaser.yaml`. `goreleaser release --snapshot --clean` builds deb, rpm and Arch for amd64 and arm64; contents and metadata verified (`.PKGINFO`, deb `control`, file lists, owner root, permissions 0644/0755). The unit with all restrictions (`ProtectSystem=strict`, `PrivateTmp` and the others) works as a transient user service through `systemd-run --user`.
- 2026-10-05: README split: `README.md` for users (installation per distribution, module, service, options, troubleshooting), `CONTRIBUTING.md` for developers (tools, code layout, build, tests, packages, rules). The project rules for agents are in `AGENTS.md`, and `CLAUDE.md` only imports that file.
- Next: install the Arch package and `systemctl --user enable --now gpwebcam.service`; after a reboot, check that modprobe.d gives `/dev/video42` "GoPro" and a second device for OBS; the man page and `gpwebcam doctor`; GitHub repo `darkodemic/gpwebcam`.
