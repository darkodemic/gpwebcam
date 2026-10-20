# Release 0.3.1

- **Status:** Release 0.3.1, prepared 2026-10-20: the first release that the workflow publishes by itself and pushes to the AUR (`aur-packages.md`). The signed tag `v0.3.1` goes on `main` once this preparation is merged, and the tag, not this document, records the commit. Scope: everything on `main` since `v0.3.0`.
- **Date:** 2026-10-20
- **Owner:** Darko
- **Related:** `aur-packages.md` (the packages and the publishing decision); `release-0.3.0.md` (the previous release); `packaging-and-release.md` §6 (channels)

## 1. What 0.3.1 brings

- The AUR packages `gpwebcam` (built from source) and `gpwebcam-bin` (the release binary), pushed by the release workflow (`aur-packages.md` §3).
- The release archives also carry the desktop entry and the icon; the release has a source archive, `gpwebcam-<version>.tar.gz`, which the `gpwebcam` PKGBUILD builds from.
- The release is published by the workflow, not left as a draft, with the notes from `docs/release-notes/0.3.1.md` (`aur-packages.md` §5, option B).
- From PR #7: the outline icon kept in `packaging/icons/gpwebcam-outline.svg`, not installed.
- The README sends Arch users to the AUR.

## 2. Before the tag

| # | Item | Why |
|---|---|---|
| R1 | PR #7 (outline icon) merged, and this preparation on top of it | 0.3.1 includes it |
| R2 | `packaging/changelog.yml`: an entry `0.3.1-1` | the Debian changelog and the RPM `%changelog` |
| R3 | `docs/release-notes/0.3.1.md` | the workflow publishes it as the notes and stops without it |
| R4 | Darko checks the `packages` artifact of this pull request, including `aur/` with the two PKGBUILDs | the release is public as soon as the tag is pushed |
| R5 | CI green on the merge commit; signed annotated tag `v0.3.1`; the workflow publishes the release and pushes both AUR packages | the new path, tested once end to end |
| R6 | After the tag: the release page, the AUR pages of `gpwebcam` and `gpwebcam-bin`, and `yay -S gpwebcam-bin` on the test machine, which replaces the Arch package installed from the release page | the AUR key has never been used, and the packages have not been installed from the AUR |

The systemd unit did not change, so the upgrade needs only a restart of the service.

Risk: the AUR key or the environment is wrong. The release is then public without the AUR packages, and the fix is to correct the cause and rerun the release job, which replaces the release files and pushes again (`replace_existing_artifacts`, CONTRIBUTING).

## 3. After 0.3.1

From `release-0.3.0.md` §3, without the AUR:

- COPR for Fedora; RPM Fusion.
- A Debian ITP, which first needs `github.com/darkodemic/systray` as a Debian package and godbus v5.2.2 in Debian, or a build with 5.1.0 (`tray-and-recording.md` §10).
- A second device "GoPro 2" for two applications at once.
- RTSP for networks with a firewall.
- Several cameras at once.

## 4. Release notes

In `docs/release-notes/0.3.1.md`, published by the workflow as they are.

## 5. Where we are and what is next

- 2026-10-20: Darko chose option B (no draft) and a release 0.3.1 for the first AUR version (`aur-packages.md` §5, §6). R2 and R3 written; the configuration tested in an Arch container (`aur-packages.md` §4).
- 2026-10-20: R1 to R5 done; the signed tag `v0.3.1` pushed, the workflow published the release in 82 s and pushed both AUR packages. Its files match `checksums.txt`, the binary reports `gpwebcam 0.3.1`, and both packages build from the AUR (`aur-packages.md` §7). The notes came out empty and were added by hand; the cause and the fix are in `aur-packages.md` §7.
- Next: R6 on the test machine, then the list in §3.
