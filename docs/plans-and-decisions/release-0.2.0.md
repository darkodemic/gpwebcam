# Release 0.2.0

- **Status:** Planned 2026-10-07. Scope: everything on `main` since `v0.1.0` (15 commits, up to `86a5fea`). Open: the release notes (§4) and how GoReleaser writes them (R4).
- **Date:** 2026-10-07
- **Owner:** Darko
- **Related:** `release-0.1.0.md` (the same path for 0.1.0); `tray-and-recording.md`; `camera-on-demand.md`; `packaging-and-release.md` §6 (channels)

## 1. What 0.2.0 brings

- A tray icon with a menu: camera mode, field of view, resolution, hardware decoding, notifications, recording, the recordings folder, restart, hide and quit. White while video flows, orange on a problem, faded otherwise, with a red dot while recording.
- Camera on demand, the default: the GoPro streams only while an application has its video on, and stops 15 s after the last one. The modes always and off are in the menu.
- Recording: the camera's H.264 goes into a Matroska file in `~/Videos/gpwebcam` without decoding, from the menu or with `gpwebcam record`.
- Settings that apply while the service runs, from the menu or `gpwebcam config`; a new resolution applies once no application uses the camera.
- "GoPro Webcam" in the application menu (`gpwebcam launch`) starts the service, for example after Quit.
- gpwebcam receives the camera's stream itself, only from the camera's address, and tells "no packets" (firewall) apart from "packets but no picture".
- Clearer messages when the camera cannot start (error 4, for example without its battery), and no garbled picture when an application holds the device at another size.
- `gpwebcam doctor` also checks the settings, the tray, the recordings folder and whether v4l2loopback reports applications.
- The systemd unit changed: `ConfigurationDirectory`, `RuntimeDirectory` and `ReadWritePaths=-%h/Videos`.
- Dependencies: `github.com/darkodemic/systray` v1.13.0 and `github.com/godbus/dbus/v5` v5.2.2.

## 2. Before the tag

| # | Item | Why |
|---|---|---|
| R1 | `packaging/changelog.yml`: an entry `0.2.0-1` with the main changes from §1 | the Debian changelog and the RPM `%changelog` come from it |
| R2 | README: the Status line (there are releases now; what 0.2.0 is tested on) and step 1 of Install (download from the releases page; building is only for contributors) | the README still says there is no release |
| R3 | The post-install message: after an upgrade, `systemctl --user daemon-reload` and `systemctl --user restart gpwebcam`, because the unit changed | otherwise a user who upgrades from 0.1.0 keeps running the old unit and binary until the next login |
| R4 | GoReleaser: `changelog.disable: true`; the notes from §4 go onto the draft before it is published | for 0.1.0, `changelog.sort: asc` listed the commits alphabetically by title, and the notes had to be replaced by hand |
| R5 | Test the packages built from the release commit: the Arch package on the test machine with the camera (tray, camera on demand, recording, Quit and the launcher), and `.deb` and `.rpm` in containers (Debian 13, Ubuntu 24.04, Fedora 44: install, `gpwebcam doctor`, the desktop entry, uninstall), with lintian, rpmlint and namcap | the new unit lines and the desktop entry have not been in a `.deb` or `.rpm` yet |
| R6 | Close Dependabot PR #1 (godbus v5.2.2), which `86a5fea` supersedes, unless Dependabot closes it first | a clean list of pull requests for the public repository |
| R7 | CI green on the release commit; a signed annotated tag `v0.2.0`; the release workflow makes a draft; Darko checks it and publishes it | the path that worked for 0.1.0 |

## 3. After 0.2.0

Collected from `release-0.1.0.md` §4 and the later plans:

- AUR: a source PKGBUILD and `gpwebcam-bin` (`packaging-and-release.md` §6).
- COPR for Fedora; RPM Fusion.
- A Debian ITP, which first needs `github.com/darkodemic/systray` as a Debian package and godbus v5.2.2 in Debian, or a build with 5.1.0 (`tray-and-recording.md` §10).
- A second device "GoPro 2" for two applications at once.
- RTSP for networks with a firewall.
- Several cameras at once.
- Darko's own icon, as an SVG, for the tray and the launcher.

## 4. Release notes, draft

```markdown
gpwebcam 0.2.0 adds a tray icon, a camera that streams only while an application uses it, and recording.

- **Tray icon** with a menu for the camera mode, field of view, resolution, hardware decoding, notifications, recording, restart and quit. White while video flows, orange on a problem, with a red dot while recording.
- **Camera on demand**, the default: the GoPro streams only while an application has its video on, and stops 15 seconds after the last one, so it does not heat up for nothing. "Always on" and "Off" are in the menu.
- **Recording**: the camera's video goes into a Matroska file in `~/Videos/gpwebcam` as the camera sends it, without re-encoding, from the menu or with `gpwebcam record start|stop`.
- **Settings while it runs**: from the menu or with `gpwebcam config`; a new resolution applies as soon as no application uses the camera.
- **GoPro Webcam** in the application menu starts the service again after Quit.
- gpwebcam now receives the camera's stream itself, only from the camera, and tells a firewall problem apart from a camera that sends no picture.
- Clearer messages when the camera cannot start, for example without its battery.
- `gpwebcam doctor` checks more: settings, tray, recordings folder, and whether v4l2loopback reports applications.

**Upgrading from 0.1.0**: the systemd unit changed, so after installing run:

    systemctl --user daemon-reload
    systemctl --user restart gpwebcam

Install and setup: see [README.md](https://github.com/darkodemic/gpwebcam/blob/v0.2.0/README.md).
```

## 5. Where we are and what is next

- 2026-10-07: list made after `86a5fea`; `v0.1.0` is published in the new public repository.
- 2026-10-07: Darko accepted R4 and the notes in §4. R1 to R4 done in the worktree `.worktrees/release-0.2.0`; `goreleaser check` passes. CONTRIBUTING describes the release steps with hand-written notes.
- 2026-10-07, R5 in containers (snapshot packages from the worktree; Debian 13, Ubuntu 24.04, Fedora 44, Arch): install, `version`, `help`, `config` and `doctor` work; files are `root:root`, 0755 and 0644; the unit has the new lines; `desktop-file-validate` passes on the desktop entry; removing the package leaves nothing behind. The man page is missing after installation in the Ubuntu, Fedora and Arch images only because those images exclude `/usr/share/man`; it is in the packages. Two fixes:
  - lintian: `debian-changelog-line-too-long` for the first R1 notes; the notes are now shorter than 80 characters per line, and lintian reports nothing.
  - `doctor` in a container said "gpwebcam.service is disabled and " with an empty state: `is-enabled` reads unit files without the user manager, `is-active` cannot. An empty `is-active` now counts as "cannot reach this user's systemd".
  - Known and accepted: rpmlint `statically-linked-binary`, `position-independent-executable-suggested` and, for snapshots only, `incoherent-version-in-changelog`; namcap RELRO, PIE and "owned by 0:0"; namcap also suggests that `ffmpeg` may not be needed, because gpwebcam starts it at run time; Fedora's `ffmpeg-free` decodes H.264 only through openh264, as the README says.
- 2026-10-07: R6 done by Dependabot, which closed PR #1: "Looks like github.com/godbus/dbus/v5 is up-to-date now, so this is no longer needed."
- Next: merge the release preparation into `main`; Darko tests the Arch package built from that commit with the camera (R5); the tag (R7).
