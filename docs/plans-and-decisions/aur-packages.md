# AUR packages

- **Status:** Accepted 2026-10-20: `gpwebcam` and `gpwebcam-bin` through GoReleaser (§3); the release workflow publishes the release itself, without a draft, and pushes to the AUR in the same run (§5, option B); the first AUR version is release 0.3.1 (§6, `release-0.3.1.md`).
- **Date:** 2026-10-20
- **Owner:** Darko
- **Related:** `packaging-and-release.md` §6 (channels), §3 (file layout); ADR 0005 (name and packages through GoReleaser); `release-0.3.0.md` §3; `release-0.3.1.md`

## 1. Scope

Two packages on the AUR, maintained by Darko:

| Package | Built from | Binary |
|---|---|---|
| `gpwebcam-bin` | the release archive `gpwebcam_<version>_linux_<arch>.tar.gz` | the static release binary, as in the other packages |
| `gpwebcam` | GoReleaser's source archive `gpwebcam-<version>.tar.gz` from the release | built by makepkg with the Arch Go flags: PIE, external linking, `-trimpath`, `-mod=readonly` |

Both install the same files as the Arch package on the GitHub release, conflict with each other and provide `gpwebcam`. A `-git` package is out of scope.

## 2. Facts

| Fact | Source |
|---|---|
| `gpwebcam`, `gpwebcam-bin` and `gpwebcam-git` are free on the AUR | AUR RPC `info`, 2026-10-20 |
| GoReleaser 2.18.2 (from `mise.toml`) publishes binary packages (`aurs`) and source packages (`aur_sources`, since v2.5); both push with `private_key`, which must not have a passphrase; the default SSH command uses `StrictHostKeyChecking=accept-new` | goreleaser.com, `aurs` and `aur_sources`, 2026-10-20 |
| `aur_sources` needs GoReleaser's source archive (`source.enabled`), and its PKGBUILD downloads it from the release | snapshot build, 2026-10-20 |
| `aurs` and `aur_sources` have no `check()` and no `options`; makepkg's default `debug` option therefore also builds a `-debug` package | GoReleaser output, Arch container, 2026-10-20 |
| The key: an SSH key used only for the AUR, without a passphrase; its public half is in the maintainer's AUR account, the private half is the secret `AUR_KEY` of the GitHub environment "Arch Linux User Repository", which allows only tags `v*`; the repository has no other secrets | Darko and the GitHub API, 2026-10-20 |
| GoReleaser pushes to the AUR during `goreleaser release`, while the GitHub release is still a draft (`release.draft: true`); a draft's assets cannot be downloaded publicly | `.goreleaser.yaml`; GitHub drafts |

## 3. Configuration

- `.goreleaser.yaml`:
  - the release archives also carry `packaging/desktop/gpwebcam.desktop` and `packaging/icons/gpwebcam.svg`, which `gpwebcam-bin` installs;
  - `source.enabled` with the prefix `gpwebcam-<version>/`;
  - `aurs` (`gpwebcam-bin`) and `aur_sources` (`gpwebcam`): description, URL, `Apache-2.0`, maintainer `Darko Demić <mail at darkodemic dot com>`, commit author Darko, `depends` `ffmpeg` and `hicolor-icon-theme`, `optdepends` `v4l2loopback-dkms` and `libnotify`, the install script, `skip_upload: auto` (no AUR push for a prerelease tag such as `v0.4.0-rc1`), `private_key` from `AUR_KEY` (empty, so nothing is pushed, without the secret).
- `v4l2loopback-dkms` is an optional dependency, like `Recommends` in the `.deb` and `.rpm`: without the module gpwebcam does not work, but the module can come from elsewhere, and `gpwebcam doctor` and the install message say what is missing.
- `packaging/aur/gpwebcam.install`: the message of `packaging/scripts/postinstall.sh` after an installation, and the `daemon-reload` and restart hint after an upgrade. The two texts are kept in step by hand.
- The release workflow gets `environment: Arch Linux User Repository` and passes `AUR_KEY` to GoReleaser.
- The CI artifact `packages` also holds the generated PKGBUILDs (`aur/`), so they can be read in the release pull request before the tag.

## 4. Test, 2026-10-20

Snapshot build of the worktree; both generated PKGBUILDs built in `archlinux:latest` as an unprivileged user, from local copies of the archives:

- `makepkg` builds both; `namcap` on both PKGBUILDs reports nothing.
- `namcap` on the packages: for `gpwebcam-bin` the known RELRO and PIE warnings of the static binary and "ffmpeg may not be needed"; for `gpwebcam` only "glibc implicitly satisfied" and the ffmpeg warning. The `-debug` packages get "empty directory" and "symlink points to a missing file", which namcap reports for a debug package checked on its own.
- The `.SRCINFO` files from GoReleaser match `makepkg --printsrcinfo` apart from the order of the lines.
- Installing `gpwebcam-bin` shows the install message, puts every file where the Arch package does, and `gpwebcam version` answers. Replacing it with `gpwebcam` and back asks to remove the other package and works both ways. The source build is a PIE executable.

## 5. Publishing and the draft release

Today the release workflow makes a draft and Darko publishes it after checking it. If GoReleaser also pushes to the AUR in that run, the AUR has the new version while its downloads still return 404, until the draft is published; if the draft turns out wrong and is deleted, the AUR keeps a version that does not exist.

| Option | How | For | Against |
|---|---|---|---|
| A. Keep the draft, push at the tag | as configured now | nothing changes in the release steps | the AUR points at missing files until the draft is published, and at a dead version if the draft is dropped |
| B. No draft; the workflow publishes the release with notes from the repository (recommended) | `release.draft: false`; notes in `docs/release-notes/<version>.md`, written before the tag (as `release-0.3.0.md` §4 already is) and passed with `--release-notes`; the workflow fails if the file is missing | the release, its notes and the AUR appear together; nothing to do by hand after the tag | the check of the packages moves before the tag: the snapshot packages of the release pull request (CI artifact) and the container tests |
| C. Keep the draft, push to the AUR when the release is published | a second workflow on `release: published` that publishes only the AUR packages | the draft stays | GoReleaser takes the PKGBUILD checksums from the archives it builds in the same run, so the second run would rebuild them, and they would have to match the published files byte for byte; otherwise the PKGBUILDs need a script of our own |

Decision, Darko, 2026-10-20: **B**. In GoReleaser `release.draft: false`, `prerelease: auto` (a tag such as `v0.4.0-rc1` is a prerelease, and `skip_upload: auto` keeps it off the AUR) and `replace_existing_artifacts: true`, so that running the release job again after a failed AUR push replaces the files of the published release. The workflow finds `docs/release-notes/<version>.md`, stops if it is missing or empty, and passes it with `--release-notes`. CONTRIBUTING describes the steps.

## 6. The first AUR version

0.3.0 is already released, and its archives lack the desktop entry and the icon that `gpwebcam-bin` installs. The proposal is a release 0.3.1 with this configuration and the outline icon (PR #7): it puts both packages on the AUR and tests the whole path once. The alternative, pushing 0.3.0 by hand, would need the CI key on Darko's machine and a hand-made PKGBUILD for an archive without the icon. Decision, Darko, 2026-10-20: release 0.3.1 (`release-0.3.1.md`).

## 7. Where we are and what is next

- 2026-10-20: the AUR key and the environment set up. Configuration and the container test (§3, §4) in the worktree `.worktrees/aur`.
- 2026-10-20: Darko chose B (§5) and release 0.3.1 (§6). The release workflow, CONTRIBUTING, the README (Arch users install from the AUR), `packaging-and-release.md` §6 and `release-0.3.1.md` updated in the same worktree.
- Next: `release-0.3.1.md` §2.
