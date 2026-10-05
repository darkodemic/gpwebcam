# 0001 — Go kao jezik implementacije

- **Status:** Accepted 2026-09-29. `gw` se piše u Go-u.
- **Date:** 2026-09-29
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `gopro-fork-review-and-handover.md` §3 (nalazi iz review-a), §4 (principi), §6 (otvorene odluke)

## Context

`gw` radi mali broj stvari, ali svaku treba uraditi pažljivo:

- pronađe GoPro mrežni interfejs po USB vendor ID-u ili ga dobije od udev-a;
- sačeka IPv4 adresu na tom interfejsu;
- pošalje nekoliko HTTP poziva kameri, svaki sa timeout-om;
- pokrene ffmpeg, nadgleda ga i ugasi;
- na SIGTERM pošalje STOP kameri;
- proveri sve ulaze.

Radi kao systemd servis bez root-a.

Stari bash skript iz forka imao je baš one greške koje bash olakšava: aritmetičku evaluaciju argumenta u `[[ -ne ]]`, deljenje reči u `modprobe` komandi i neprovereni tekst u ffmpeg filtergraph-u (§3.1 beleške).

## Decision

1. `gw` je jedan Go binarni fajl, bez runtime zavisnosti osim ffmpeg-a.
2. ffmpeg se pokreće kroz `os/exec` sa listom argumenata, nikad kroz shell.
3. Prednost ima standardna biblioteka: `net/http` sa timeout-ima, `os/signal`, `context`. Spoljna zavisnost ulazi samo kad štedi pravi posao.

## Consequences

**Positive**

- Cela klasa shell injekcija nestaje, jer nema shell-a.
- Ulazi su tipovi (enum za rezoluciju i FOV, brojevi u opsegu), pa neispravna vrednost ne stigne do ffmpeg-a ni do kamere.
- Pakovanje je jednostavno: jedan fajl, lako za PKGBUILD.

**Negative**

- Za build treba Go toolchain, a skript se mogao pokrenuti odmah.
- Binarni fajl ima nekoliko MB umesto nekoliko KB.

**Risks**

- ffmpeg i dalje parsira mrežni ulaz. Ublaženo time što radi bez root-a i sluša samo na GoPro interfejsu (§4 beleške).

## Alternatives considered

- **Bash:** najbrži početak, ali iste vrste grešaka kao u forku.
- **Python:** dobar, ali traži interpreter i pakovanje zavisnosti na ciljnoj mašini.
- **Rust:** bezbedan, ali previše ceremonije za ovako mali alat.
- **Go bez ffmpeg-a** (sopstveni MPEG-TS demux i H.264 dekodiranje): mnogo posla bez koristi za korisnika.

## Out of scope

- GUI i tray ikonica.
- GStreamer kao zamena za ffmpeg.
