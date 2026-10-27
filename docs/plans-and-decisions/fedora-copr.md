# Fedora: Copr

- **Status:** Accepted 2026-10-25: a spec that builds from source with vendored Go modules, and Packit builds it in Copr (§4, option A). Packit's test build passed on all eight chroots on 2026-10-27 (§3). Open: Packit's builder permission in `darko/gpwebcam`, requested on the first release build (§5, §6).
- **Date:** 2026-10-20
- **Owner:** Darko
- **Related:** `packaging-and-release.md` §6 (channels: COPR third, RPM Fusion later), §3 (file layout); `aur-packages.md` (the same path for Arch); ADR 0005

## 1. Scope

A Copr repository from which Fedora users install and update gpwebcam with dnf:

```sh
sudo dnf copr enable darko/gpwebcam
sudo dnf install gpwebcam
```

- Built from source in Copr, for every active Fedora release and Rawhide, on x86_64 and aarch64.
- The same files as the `.rpm` on the GitHub release, which stays as it is.
- Out of scope: EPEL (ffmpeg comes only from RPM Fusion there), the official Fedora repositories (`packaging-and-release.md` §6, item 5), RPM Fusion.

## 2. Facts

| Fact | Source |
|---|---|
| Creating Copr projects and building needs a Fedora account (FAS) | Copr user documentation, 2026-10-20 |
| Copr builds from SRPM uploads, URLs, SCM (spec in a git repository), DistGit, PyPI, RubyGems and custom scripts; GitHub webhooks rebuild an SCM package on push, or on tag creation if the webhook sends "Branch or tag creation", and the tag should be `PKGNAME-VERSION` or the webhook URL must name the package | Copr user documentation, 2026-10-20 |
| A Copr API token is valid for 180 days and then has to be generated again on the Copr API page | copr.fedorainfracloud.org/api, 2026-10-20 |
| Packit, a GitHub app, builds in Copr: `copr_build` jobs on `pull_request` (a temporary project of Packit's) and on `release` (any owner and project). In a project of the maintainer's, Packit asks for builder permission on its first build, and the project must list the repository under "Packit allowed forge projects". Builds have no network unless `enable_net` is set | packit.dev, `copr_build`, 2026-10-20 |
| Packit onboarding: install the app, and have a FAS account whose "GitHub Username" field is set; Packit checks it, or on `/packit verify-fas <fas>` | packit.dev, onboarding guide, 2026-10-20 |
| When the FAS user name differs from the GitHub user name, the check at installation fails: Packit answers builds with "not on our allowlist" and opens an issue in `packit/notifications`, where the comment `/packit verify-fas <fas>` approves the GitHub namespace within minutes | onboarding of `darkodemic` with FAS `darko`, 2026-10-27 |
| Packit's SRPM build has network: `go mod vendor` in the `create-archive` action downloads the modules; the RPM build in Copr then runs offline | SRPM build log of the test build, 2026-10-27 |
| Fedora's `ffmpeg-free` provides `/usr/bin/ffmpeg` but not the name `ffmpeg`; v4l2loopback is only in RPM Fusion (`v4l2loopback`, `akmod-v4l2loopback`) | `packaging-and-release.md` §3, §5 |

## 3. The package

`packaging/rpm/gpwebcam.spec`:

- Source: `gpwebcam-<version>.tar.gz` from `packaging/rpm/make-archive.sh`, which takes the commit with `git archive` and adds the Go modules with `go mod vendor`. The RPM build then needs no network, as in Copr's and Fedora's mock.
- Build: Fedora's `%gobuild` (`go-rpm-macros`) in module mode with `-mod=vendor`: PIE, external linking with Fedora's hardening flags, a build ID. Not `-trimpath`, because debugedit needs the source paths for the `debugsource` package.
- `%check`: `desktop-file-validate` and `go test ./...` with the real ffmpeg.
- `License: Apache-2.0 AND BSD-2-Clause AND BSD-3-Clause` (gpwebcam and the systray fork; godbus; `golang.org/x/sys`), with the vendored licenses installed and `Provides: bundled(golang(...))` for the three modules.
- `Requires: /usr/bin/ffmpeg` and `hicolor-icon-theme`; `Recommends: v4l2loopback` and `libnotify`. Files as the nFPM packages: binary, user unit, modules-load and modprobe configuration, menu entry, icon, man page, README, licenses.
- No post-install message, unlike the nFPM packages; the README and `gpwebcam doctor` cover the setup.

`.packit.yaml` runs `make-archive.sh` as the `create-archive` action, with `git` and `golang` in the SRPM build.

Test, 2026-10-20, `fedora:44`:

- `rpmbuild -ba` with `GOPROXY=off` builds the package, `-debuginfo` and `-debugsource`; all Go tests pass with `ffmpeg-free`.
- `rpmlint`: `invalid-url` for `Source0`, since the archive is made by the script and not downloaded; a spelling "error" for "systemd" in the description; `files-duplicate`, because the systray fork's license is the same Apache-2.0 text as gpwebcam's.
- Installed with dnf: `gpwebcam version` answers `0.3.1`, the binary is PIE, the files and dependencies are as listed above.
- `packit srpm` on a test commit makes the vendored archive and an SRPM, with the version from the spec.

Packit test build, 2026-10-27, for the pull request that adds `.packit.yaml` (#11), in Packit's project `packit/darkodemic-gpwebcam-11`:

- All eight chroots built: `fedora-43`, `44`, `45` and `rawhide`, each on x86_64 and aarch64. Each made the package, `-debuginfo` and `-debugsource`, with a release such as `1.<timestamp>.pr11.3.g533792d.fc43`.
- The fedora-43 x86_64 log shows `%gobuild` with `-mod=vendor`, PIE, external linking with Fedora's hardening flags and `-X main.version=0.3.1`; `desktop-file-validate` passes; every Go package's tests pass.

Upgrade from the GoReleaser RPM, 2026-10-27, `fedora:44` with dnf5 5.4.3: with `gpwebcam-0.3.1-1` from the release page installed and the pull request's Copr project enabled, `dnf install gpwebcam` answers "already installed" and changes nothing; `dnf upgrade gpwebcam` replaces it with the Copr package. The README and the 0.3.2 notes tell users with the release RPM to run `dnf upgrade`.

## 4. How builds reach Copr

| Option | How | For | Against |
|---|---|---|---|
| A. Packit (recommended) | `.packit.yaml`; the Packit GitHub app builds every pull request in a temporary Copr project and every release in `darko/gpwebcam` | no secret in the repository; a test RPM for every pull request, so a broken spec shows before the release; the usual way for upstream projects in Fedora | another GitHub app with access to the repository (code read, checks and comments write); onboarding with the FAS account |
| B. Copr webhook | an SCM package in the Copr project, built by Copr's `make srpm` method from `.copr/Makefile`, and a GitHub webhook on tag creation | no app; only the webhook URL in the repository settings | the webhook also fires on every new branch; no test builds for pull requests; tags must carry the package name, or the URL must |
| C. `copr-cli` in the release workflow | a Copr API token as a secret in the release environment | everything in our workflow | the token expires every 180 days and has to be renewed by hand |

Decision, Darko, 2026-10-25: **A**, Packit; one more GitHub app is acceptable.

## 5. What only Darko can do

1. A Fedora account at accounts.fedoraproject.org, with the "GitHub Username" field set.
2. A Copr project `gpwebcam` at copr.fedorainfracloud.org: chroots `fedora-43`, `fedora-44`, `fedora-45` and `fedora-rawhide`, each for `x86_64` and `aarch64` (the active Fedora releases in Copr's chroot list, 2026-10-25), with "Follow Fedora branching" on so that new releases are added; description and install instructions from §1. Not i386, ppc64le, s390x or riscv64, for which there are no release binaries either; not ELN, EPEL or other distributions (§1).
3. For option A: install the Packit app on `darkodemic/gpwebcam` only, and confirm the FAS account with `/packit verify-fas darko` in the issue Packit opens, because the FAS and GitHub user names differ (§2); in the Copr project settings, add `github.com/darkodemic/gpwebcam` to "Packit allowed forge projects", and approve Packit's builder request on its first build.
4. Tell the FAS user name, which goes into `.packit.yaml` (`owner`) and the README.

## 6. The first Copr version

Packit builds on a GitHub release, so the first build in `darko/gpwebcam` comes with the next release, 0.3.2, which also takes the Go module cache fix to the AUR (`aur-packages.md` §8). The pull request that adds `.packit.yaml` already gets a test build in Packit's own project.

Decision, Darko, 2026-10-27: the release job sets `update_release: false`. Packit otherwise appends its snapshot suffix to Release in every SRPM it makes, so a release would reach users as `0.3.2-1.<timestamp>.<branch>.g<sha>.fc43`; with the option it is `0.3.2-1.fc43`, the same version-release as the GoReleaser RPM. Pull request builds keep the suffix. Packit still sets Version from the tag, and every release sets `Version` and a `%changelog` entry in the spec (CONTRIBUTING, "To release"). The release plan is `release-0.3.2.md`.

## 7. Where we are and what is next

- 2026-10-20: Darko asked to continue with Fedora. The spec, the archive script and `.packit.yaml` written and tested (§3) in the worktree `.worktrees/fedora-copr`.
- 2026-10-25: Darko chose Packit (§4) and is setting up the accounts and the Copr project (§5). The Copr owner is `darko`, in `.packit.yaml`.
- 2026-10-25: Darko made the FAS account and the Copr project `darko/gpwebcam` (§5, steps 1, 2 and 4). Checked through the Copr API: chroots `fedora-43`, `44`, `45` and `rawhide` for x86_64 and aarch64 (Rawhide added afterwards, because Follow Fedora branching forks the new release's chroots from it), network off, Follow Fedora branching on, `github.com/darkodemic/gpwebcam` allowed for Packit, install instructions set. The README, CONTRIBUTING and `packaging-and-release.md` §6 describe Copr.
- 2026-10-27: Darko installed the Packit app on `darkodemic/gpwebcam` and confirmed the FAS account (§5, step 3). Packit's test build of the pull request passed on all eight chroots (§3), with the SRPM step downloading the Go modules (§2).
- 2026-10-27: release 0.3.2 prepared (`release-0.3.2.md`), with `update_release: false` for release builds (§6).
- Next: release 0.3.2, the first build in `darko/gpwebcam`, where Darko approves Packit's builder permission (§5, step 3; §6).
