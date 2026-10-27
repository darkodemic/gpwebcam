# Release 0.3.2

- **Status:** Release 0.3.2, prepared 2026-10-27: the first release that Packit builds into the Copr project `darko/gpwebcam` (`fedora-copr.md`), and the one that takes the Go module cache fix to the AUR package `gpwebcam` (`aur-packages.md` §8). The signed tag `v0.3.2` goes on `main` once this preparation is merged, and the tag, not this document, records the commit. Scope: everything on `main` since `v0.3.1`.
- **Date:** 2026-10-27
- **Owner:** Darko
- **Related:** `fedora-copr.md` (the Copr package and Packit); `aur-packages.md` §8 (`gpwebcam-git` and the module cache fix); `release-0.3.1.md` (the previous release); `packaging-and-release.md` §6 (channels)

## 1. What 0.3.2 brings

- Copr for Fedora: Packit builds the RPM spec `packaging/rpm/gpwebcam.spec` from the release tag into `darko/gpwebcam`, for Fedora 43, 44, 45 and Rawhide on x86_64 and aarch64 (`fedora-copr.md` §3 to §6). The README already sends Fedora users there, and now also tells those with the release page's `.rpm` to move with `dnf upgrade`, because `dnf install` leaves an installed package as it is (`fedora-copr.md` §3).
- The AUR package `gpwebcam` builds with `GOPATH` in the build directory and `-modcacherw`, so it no longer leaves a read-only module cache in `~/go` (`aur-packages.md` §8).
- `gpwebcam-git` on the AUR, published by hand on 2026-10-20 and kept up to date by the release workflow (`aur-packages.md` §8).
- From PR #9: the release workflow loads the notes again and checks that the published release has them (`aur-packages.md` §7).

## 2. Before the tag

| # | Item | Why |
|---|---|---|
| R1 | `packaging/changelog.yml`: an entry `0.3.2-1` | the Debian changelog and the `%changelog` of the GoReleaser RPM |
| R2 | `packaging/rpm/gpwebcam.spec`: `Version: 0.3.2` and a `%changelog` entry `0.3.2-1`, which replaces the `0.3.1-1` entry, since 0.3.1 was never built in Copr | the Copr RPM |
| R3 | `.packit.yaml`: `update_release: false` on the release job | Fedora users get `0.3.2-1.fc43`, not a release with Packit's snapshot suffix; pull request builds keep the suffix |
| R4 | `docs/release-notes/0.3.2.md` | the workflow publishes it as the notes and stops without it |
| R5 | Packit's test build of this pull request passes on all eight chroots, and Darko checks the `packages` artifact | the release is public as soon as the tag is pushed |
| R6 | CI green on the merge commit; signed annotated tag `v0.3.2`; the workflow publishes the release and pushes the AUR packages | as in 0.3.1 |
| R7 | Packit starts the release build in `darko/gpwebcam` and asks for builder permission; Darko approves it in the Copr project settings (Permissions). If the build failed meanwhile, it is started again from the Packit check on the tagged commit (Re-run) or with `/packit build --release v0.3.2` in a pull request | Packit has never built in `darko/gpwebcam` (`fedora-copr.md` §5, step 3) |
| R8 | After the tag: the release page, the AUR page of `gpwebcam` (its PKGBUILD sets `GOPATH`), and the Copr build with `0.3.2-1` in every chroot | the first Copr release, and the first use of `update_release: false` |

The systemd unit did not change, so the upgrade needs only a restart of the service.

Risk: the release build in Copr fails or waits for the permission. The GitHub release and the AUR packages are then already out, and only Fedora users wait; the fix is R7, without a new tag.

## 3. After 0.3.2

From `release-0.3.1.md` §3, without Copr:

- RPM Fusion, once Copr is stable.
- A Debian ITP, which first needs `github.com/darkodemic/systray` as a Debian package and godbus v5.2.2 in Debian, or a build with 5.1.0 (`tray-and-recording.md` §10).
- A second device "GoPro 2" for two applications at once.
- RTSP for networks with a firewall.
- Several cameras at once.

## 4. Release notes

In `docs/release-notes/0.3.2.md`, published by the workflow as they are.

## 5. Where we are and what is next

- 2026-10-27: Darko asked for the release right after the Copr pull request was merged. R1 to R4 written; R3 decided by Darko on the same day. The upgrade from the release page's RPM to Copr tested in `fedora:44` (`fedora-copr.md` §3).
- Next: R5 to R8.
