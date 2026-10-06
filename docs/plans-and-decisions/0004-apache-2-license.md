# 0004 — Apache-2.0 license

- **Status:** Accepted 2026-10-05. `gw` is released under the Apache License 2.0; the text is in `LICENSE`.
- **Date:** 2026-10-05
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `gopro-fork-review-and-handover.md` §1 (why code from the fork is not copied), §6 (the license was an open question); `packaging-and-release.md`

## Context

The project is heading toward a public release and distro packages, and so far it has had no license. Without a license nobody, distributions included, has the right to share or modify it. Darko proposed MPL-2.0 or Apache-2.0.

`gw` is a program for end users, not a library. Code from the fork `gopro_as_webcam_on_linux`, which is under Apache-2.0, was not copied (handover note §1), so the fork's license does not constrain the choice.

## Decision

1. The license is **Apache License 2.0**, SPDX identifier `Apache-2.0`.
2. The repository root has `LICENSE` with the canonical text from `https://www.apache.org/licenses/LICENSE-2.0.txt` (sha256 `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`, downloaded 2026-10-05). The text is the same as `/usr/share/licenses/spdx/Apache-2.0.txt` from the Arch package `licenses` when line breaks are ignored.
3. A `NOTICE` file is not added for now; it is not required, and the license applies in full without it.
4. Packages carry the SPDX identifier in the license field and install the license text where the distribution requires it (`packaging-and-release.md`).

## Consequences

**Positive**

- The most common license in the Go ecosystem; distributions, companies and contributors know it well.
- An explicit patent grant from everyone who contributes code.
- No obligations for those who modify and share `gw`, except to keep the license notice and mark their changes.

**Negative**

- Someone can make a closed modified version and not publish it.

## Alternatives considered

- **MPL-2.0:** file-level copyleft; whoever shares a modified `gw` has to publish the modified files. Darko chose Apache-2.0.
- **GPL:** stronger copyleft than a tool like this needs.
- **MIT:** similarly permissive, but without an explicit patent grant.
