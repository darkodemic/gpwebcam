# 0005 — Ime gpwebcam i paketi kroz GoReleaser

- **Status:** Accepted 2026-10-05. Program, paket i Go modul se zovu `gpwebcam`; paketi (`.deb`, `.rpm`, Arch, `tar.gz`) prave se GoReleaser-om po rasporedu iz `packaging-and-release.md` §3, a v4l2loopback se podešava fajlom u `/usr/lib/modprobe.d`.
- **Date:** 2026-10-05
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `packaging-and-release.md` (istraživanje i opcije); ADR 0003 (gw drži loopback uređaj i radi kao user servis); ADR 0004 (licenca Apache-2.0)

## Context

Projekat ide ka javnom izdanju preko paketa za distribucije (`packaging-and-release.md`). Istraživanje od 2026-10-05 je pokazalo:

- Ime `gw` je zauzeto: na AUR-u ga imaju dva paketa sa `/usr/bin/gw`, a u Debian-u `greaseweazle` (§2).
- Distribucije traže `/usr/lib/...` za fajlove iz paketa, zabranjuju pisanje u `/etc` i `$HOME` i ne uključuju user servise same (§3).
- v4l2loopback dele OBS i drugi programi; RPM Fusion već donosi svoj `98-v4l2loopback.conf` (§4).

## Decision

1. **Ime je `gpwebcam`**: binarni fajl, paket, systemd unit, Go modul `github.com/darkodemic/gpwebcam` i budući GitHub repo. Slobodno je u Arch repoima, AUR-u, Debian-u i Fedori, a ne sadrži punu zaštićenu reč "GoPro". Dokumenti napisani pre 2026-10-05 i dalje kažu `gw`; to je isti program.
2. **Paketi se prave GoReleaser-om 2.18.2** (zakucan u `mise.toml`), koji ih pravi kroz nFPM: `.deb` i `.rpm` iz jedne stavke, Arch iz druge zbog kratkog opisa, plus `tar.gz` i `checksums.txt`. Lokalni build bez objavljivanja: `goreleaser release --snapshot --clean`.
3. **Raspored**: `/usr/bin/gpwebcam`, `/usr/lib/systemd/user/gpwebcam.service`, `/usr/lib/modules-load.d/gpwebcam.conf`, `/usr/lib/modprobe.d/99-gpwebcam.conf`, `/usr/share/doc/gpwebcam/README.md`; licenca u `/usr/share/licenses/gpwebcam/` (Arch, rpm) ili kao DEP-5 `/usr/share/doc/gpwebcam/copyright` (deb). Ništa u `/etc` i `$HOME`.
4. **Modul (opcija A)**: `options v4l2loopback devices=2 video_nr=42,-1 card_label="GoPro,OBS Virtual Camera" exclusive_caps=1,1`. Drugi uređaj ostaje za OBS; administrator menja ili isključuje podešavanje fajlom istog imena u `/etc/modprobe.d/`.
5. **`gpwebcam` nalazi uređaj po imenu** "GoPro" u `/sys/class/video4linux/*/name`; `-video-nr` ga bira po broju, a `-device-label` menja ime koje se traži.
6. **Servis se ne uključuje iz paketa**. Skripta posle instalacije samo ispiše šta korisnik treba da uradi (`systemctl --user enable --now gpwebcam.service`).
7. **Zavisnosti**: deb `Depends: ffmpeg`, `Recommends: v4l2loopback-dkms | v4l2loopback-modules`, `Suggests: v4l2loopback-utils`; rpm `Requires: /usr/bin/ffmpeg`, `Recommends: v4l2loopback`; Arch `depends=(ffmpeg)` (nFPM ne ume `optdepends`, pa AUR dobija ručno pisan PKGBUILD).
8. **`go.mod` traži `go 1.22`**, da paket može da se prevede i u Debian trixie (Go 1.24). Provereno 2026-10-05: `go vet` i svi testovi prolaze sa go1.22.12.

## Consequences

**Positive**

- Paketi poštuju pravila distribucija od prvog dana, pa predaja AUR-u, COPR-u i Debian-u kasnije ne traži novi raspored.
- OBS i drugi korisnici v4l2loopback-a i dalje imaju svoj uređaj.
- Promena broja uređaja kod administratora ne kvari servis.

**Negative**

- Fajl u `/usr/lib/modprobe.d` menja podrazumevano ponašanje modula za sve programe, i važi tek posle restarta ili ponovnog učitavanja modula.
- Ime `gpwebcam` treba uneti i u lokalni direktorijum projekta i u GitHub repo; to radi Darko.
- Arch paket iz GoReleaser-a nema `optdepends`.

**Risks**

- `card_label` sa zarezom i razmakom zavisi od toga kako v4l2loopback deli niz; to treba proveriti posle prvog učitavanja modula sa novim podešavanjem (`cat /sys/class/video4linux/*/name`).

## Alternatives considered

- **Imena `herocam` i `gopro-webcam`**: oba slobodna, ali sadrže GoPro-ove zaštićene znakove; `gopro-webcam` ima i 16 istoimenih repoa na GitHub-u.
- **Samo nFPM**: pravi pakete, ali bez builda, GitHub release-a i AUR objave.
- **Opcija B, root oneshot sa `v4l2loopback-ctl add`**: ne dira tuđe opcije, ali uvodi root u radu.
- **Uključivanje servisa iz paketa (preset ili `systemctl --global enable`)**: Arch i Fedora to ne rade, a prvi prijavljeni korisnik bi zauzeo uređaj za sve ostale.
