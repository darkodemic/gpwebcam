# Release 0.3.0

- **Status:** Release 0.3.0, prepared 2026-10-10 together with the icon it brings; the signed tag `v0.3.0` goes on `main` once this preparation is merged, and the tag, not this document, records the commit. Scope: everything on `main` since `v0.2.0`.
- **Date:** 2026-10-10
- **Owner:** Darko
- **Related:** `release-0.2.0.md` (the same path for 0.2.0); `tray-and-recording.md` §10 (the icon, 2026-10-08 and 2026-10-09); `packaging-and-release.md` §3 (the icon's path and dependency), §6 (channels)

## 1. What 0.3.0 brings

- Darko's application icon in the application menu, the tray and notifications, instead of `camera-web` from the icon theme.
- The tray icon is always in color and about 12 % larger in the same slot. A dot at the bottom right shows the state: green while video flows, blue while the camera starts or waits for an application, gray when the camera mode is off, orange on a problem, none without a camera. The red recording dot stays at the top right.
- `GET /v1/status` on the control socket reports six states (`no-camera`, `starting`, `ready`, `paused`, `live`, `trouble`) instead of three.
- The packages install `/usr/share/icons/hicolor/scalable/apps/gpwebcam.svg` and depend on `hicolor-icon-theme`.
- From PR #5: issue templates, `SECURITY.md` and code owners, and the README section on reporting problems.

## 2. Before the tag

| # | Item | Why |
|---|---|---|
| R1 | `packaging/changelog.yml`: an entry `0.3.0-1` | the Debian changelog and the RPM `%changelog` come from it |
| R2 | The release notes in §4 | GoReleaser leaves them empty (`changelog.disable`) |
| R3 | The documents written for the release before the tag, without the release commit (CONTRIBUTING, release steps) | Darko, 2026-10-10: no separate pull request after the release only to record it; the git history has the commit |
| R4 | The packages: the Arch package on the test machine with the camera; `.deb` and `.rpm` resolve the new dependency; lintian, rpmlint and namcap | the icon and the dependency are new in the packages |
| R5 | CI green on the merge commit; a signed annotated tag `v0.3.0`; the release workflow makes a draft; Darko checks it, puts the notes from §4 on it and publishes it | the path from 0.1.0 and 0.2.0 (CONTRIBUTING, "To release") |

The systemd unit did not change, so the upgrade needs only a restart of the service, not `daemon-reload`.

## 3. After 0.3.0

The list from `release-0.2.0.md` §3, without the icon:

- AUR: a source PKGBUILD and `gpwebcam-bin` (`packaging-and-release.md` §6).
- COPR for Fedora; RPM Fusion.
- A Debian ITP, which first needs `github.com/darkodemic/systray` as a Debian package and godbus v5.2.2 in Debian, or a build with 5.1.0 (`tray-and-recording.md` §10).
- A second device "GoPro 2" for two applications at once.
- RTSP for networks with a firewall.
- Several cameras at once.

## 4. Release notes, draft

```markdown
gpwebcam 0.3.0 gives gpwebcam its own icon.

- **New icon** in the application menu, the tray and notifications.
- **The tray icon shows the state with a dot**: green while video flows, blue while the camera starts or waits for an application, gray when the camera is off in gpwebcam, orange on a problem, and no dot without a camera. The red dot at the top right still means recording. The icon itself stays in color.
- The packages now depend on `hicolor-icon-theme`, which holds the standard icon directories.
- Issue templates and a security policy for reporting problems.

**Upgrading from 0.2.0**: restart the service to run the new version:

    systemctl --user restart gpwebcam

If the application menu still shows a generic icon, restart the panel or launcher that was running during the installation, or log out and in again.

Install and setup: see [README.md](https://github.com/darkodemic/gpwebcam/blob/v0.3.0/README.md).
```

## 5. Where we are and what is next

- 2026-10-08 and 2026-10-09: the icon (`tray-and-recording.md` §10). namcap, lintian and rpmlint report only the known findings from CONTRIBUTING; the packaging has not changed since, only the binary.
- 2026-10-09: Darko installed the Arch package with the larger icon and the state dots on the test machine and restarted his bar: "super je sad".
- 2026-10-10: Darko decided to release it. R1 to R3 written. R4: installs simulated on the snapshot packages with `apt-get install -s` (Debian 13, Ubuntu 24.04) and `dnf install --assumeno` (Fedora 44): the dependencies resolve, on Fedora with `hicolor-icon-theme` 0.18 and `ffmpeg-free` 8.1.3.
- Next: R5, then the list in §3.
