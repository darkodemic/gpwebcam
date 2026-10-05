# Pakovanje i javno izdanje

- **Status:** U izradi. Odlučeno 2026-10-05: licenca Apache-2.0 (ADR 0004); ime `gpwebcam`, GoReleaser 2.18.2, raspored iz §3 i modprobe.d opcija A (ADR 0005). Paketi se prave lokalno; instalacija i test čekaju.
- **Date:** 2026-10-05
- **Owner:** Darko
- **Related:** ADR 0003 (gw drži loopback uređaj i radi kao user servis); ADR 0004 (licenca Apache-2.0); ADR 0005 (ime gpwebcam i paketi kroz GoReleaser); `second-slice-gw-run.md`

## 1. Cilj

Paketi koji poštuju pravila distribucija od početka, iako prvo idu samo na GitHub release: `.deb`, `.rpm`, Arch `.pkg.tar.zst` i `tar.gz` sa checksum-ovima. Prvo se prave i instaliraju lokalno, a kasnije se predaju kanalima distribucija (§6).

Izvori su pročitani 2026-10-05; oznaka [D] znači zvanična dokumentacija ili izvorni kod paketa, a [I] zaključak.

## 2. Ime

`gw` je zauzet (proveravano 2026-10-05):

- AUR: paket `gw` (genome browser, instalira `/usr/bin/gw`) i `gw-tools` (git worktree, takođe `/usr/bin/gw`) [D].
- Debian sid: `greaseweazle` instalira `/usr/bin/gw`, a Debian Policy 10.1 ne dozvoljava dva programa istog imena [D].
- Slobodno je u zvaničnim Arch repoima, Fedori i Ubuntu noble-u.

Slobodna imena (Arch, AUR, Debian, Fedora; i paket i `/usr/bin` fajl): `gpwebcam`, `herocam`, `gopro-webcam`. "GoPro" i "HERO" su zaštićeni znakovi, pa je `gpwebcam` najbezbedniji [I]. Na AUR-u je 2026-09-30 objavljen srodan projekat `action-webcamd` (Rust, sistemski servis za GoPro).

## 3. Raspored fajlova

Isti na Arch-u, Debian-u i Fedori [D]; `<n>` je novo ime:

| Fajl | Putanja |
|---|---|
| program | `/usr/bin/<n>`, 0755 |
| user servis | `/usr/lib/systemd/user/<n>.service`, `ExecStart=/usr/bin/<n> run`; na Fedori nikad `%config` |
| učitavanje modula | `/usr/lib/modules-load.d/<n>.conf` |
| opcije modula | `/usr/lib/modprobe.d/99-<n>.conf`; Debian ne dozvoljava `/lib/...` (lintian `aliased-location`) |
| man stranica | `/usr/share/man/man1/<n>.1.gz`; Debian bez nje daje lintian upozorenje |
| README | `/usr/share/doc/<n>/README.md` |
| licenca | Arch: `/usr/share/licenses/<n>/LICENSE` (nije obavezno za Apache-2.0, `licenses` paket ga ima); Fedora: isto, kao `%license`; Debian: `/usr/share/doc/<n>/copyright` u DEP-5 formatu, sa pozivom na `/usr/share/common-licenses/Apache-2.0` |

Paket ne sme da piše u `$HOME` ni u `/etc` (`/etc` je za administratora; fajl istog imena u `/etc` sakriva onaj iz `/usr/lib`), i ne pokreće user servise iz skripti [D]. Ne uključuje servis za sve korisnike: Arch ne koristi presets, Fedora uključuje samo servise koji rade bez podešavanja, a `systemd.preset(5)` ne preporučuje preset u paketu [D]. Uz to, ako bi servis bio uključen za sve, prvi prijavljeni korisnik bi zauzeo `/dev/video42`, a servisi ostalih bi se restartovali u krug [I]. Skripta posle instalacije samo ispisuje šta korisnik treba da uradi.

## 4. Podešavanje v4l2loopback modula

Činjenice [D]:

- modprobe.d: sve opcije se sabiraju, fajlovi se sortiraju po imenu kroz sve direktorijume, a fajl istog imena u `/etc` sakriva onaj iz `/usr/lib`.
- Arch `v4l2loopback-dkms` i Debian `v4l2loopback-dkms`/`-utils` ne donose modprobe ni modules-load konfiguraciju. Debian `-utils` donosi udev pravilo za `/dev/v4l2loopback` (grupa `video`).
- RPM Fusion `v4l2loopback` donosi `/usr/lib/modprobe.d/98-v4l2loopback.conf` (`exclusive_caps=1 card_label="OBS Virtual Camera"`) i učitava modul pri boot-u.
- OBS: ako modul nije učitan, pokrene `pkexec modprobe v4l2loopback exclusive_caps=1 card_label='OBS Virtual Camera'`; ako jeste, uzme prvi `/dev/video*` koji prima izlaz.
- `v4l2loopback-ctl add` traži kontrolni uređaj koji je samo za root-a.

Opcije:

| Opcija | Šta | Za | Protiv |
|---|---|---|---|
| A | paket donosi `modules-load.d/<n>.conf` (`v4l2loopback`) i `modprobe.d/99-<n>.conf`: `options v4l2loopback devices=2 video_nr=42,-1 card_label="GoPro,OBS Virtual Camera" exclusive_caps=1,1` | bez root-a u radu; drugi uređaj ostaje za OBS; pobeđuje RPM Fusion-ov `98-`, a administratorov fajl u `/etc` pobeđuje njega | menja podrazumevano ponašanje modula za sve programe; važi tek posle restarta ili ponovnog učitavanja modula |
| B | sistemski oneshot servis koji pri boot-u kao root napravi uređaj sa `v4l2loopback-ctl add` | ne dira tuđe opcije | root u radu, protivno principu iz predajne beleške §4; traži nov ADR |

Preporuka: A [I]. Uz nju `gw` treba da nađe svoj uređaj po imenu ("GoPro" u `/sys/class/video4linux/*/name`) umesto po broju 42, da administratorova promena broja ne bi pokvarila servis.

## 5. Zavisnosti

| Distribucija | ffmpeg | modul |
|---|---|---|
| Arch | `depends=(ffmpeg)` | `optdepends=('v4l2loopback-dkms: ...')`, kao obs-studio [D]; nFPM-ov archlinux paket ne ume da upiše optdepends (izvorni kod `arch/arch.go`), pa za AUR treba ručno pisan PKGBUILD [D] |
| Debian, Ubuntu | `Depends: ffmpeg` | `Recommends: v4l2loopback-dkms \| v4l2loopback-modules`, `Suggests: v4l2loopback-utils`; Ubuntu noble ima modul u `linux-modules-*-generic` [D] |
| Fedora | `Requires: /usr/bin/ffmpeg` [I], jer `ffmpeg-free` ne nosi ime `ffmpeg` | ne postoji u Fedori; RPM Fusion `v4l2loopback` i `akmod-v4l2loopback`; u COPR-u `Recommends: v4l2loopback` [I] |

Fedorin `ffmpeg-free` je preveden bez H.264 dekodera i oslanja se na `libopenh264` iz Cisco repoa, koji je uključen, ali paket nije podrazumevano instaliran [D]. Da li openh264 dekodira GoPro-ov High profile stream nije provereno; preporuka je RPM Fusion `ffmpeg`, a `gw doctor` treba da proveri `ffmpeg -decoders` [I].

`go.mod` traži Go 1.26.8, a Debian trixie ima 1.24 [D]. Kod koristi ništa novije od Go 1.21 (`log/slog`, `slices`), pa direktivu treba spustiti na `go 1.22`.

## 6. Kanali

Redosled [I]:

1. GitHub release sa paketima iz GoReleaser-a (`.deb`, `.rpm`, `.pkg.tar.zst`, `tar.gz`, checksum-ovi). Testiranje na Arch-u, Debian trixie i Ubuntu noble-u, i Fedori sa RPM Fusion ffmpeg-om i sa `ffmpeg-free` + openh264.
2. AUR: ručno pisan izvorni PKGBUILD (ime `<n>`, sa optdepends); po želji i `<n>-bin` preko GoReleaser `aurs` [D: `-bin` sufiks obavezan, `.SRCINFO` uz svaki push, SPDX u `license`].
3. COPR za Fedoru [D: dovoljan Fedora nalog].
4. Debian: ITP bug, pa mentors i sponzor; Go tim imenuje programe bez `golang-` prefiksa [D]. Ubuntu preuzima iz Debian-a. Procena 1 do 3 meseca [I]. PPA samo ako Ubuntu treba ranije.
5. RPM Fusion: review u Bugzilli, kad COPR bude stabilan [D]. Zvanična Fedora ne prima pakete kojima treba modul van kernela [D].
6. Po želji openSUSE OBS: jedno mesto za apt, yum i pacman repoe [D].

GoReleaser 2.18.2 besplatno pravi nFPM pakete i objavljuje `-bin` na AUR; apt i yum repoi su samo u Pro verziji, a Debian, PPA i COPR nemaju publisher [D].

## 7. Gde smo i šta sledi

- 2026-10-05: licenca Apache-2.0 i `LICENSE` (ADR 0004); GoReleaser 2.18.2 kroz `mise.toml`; istraživanje pravila distribucija (ovaj dokument).
- 2026-10-05: Darko izabrao ime `gpwebcam` i opciju A (ADR 0005). Urađeno: preimenovanje (`cmd/gpwebcam`, modul `github.com/darkodemic/gpwebcam`), `go 1.22` (provereno sa go1.22.12), uređaj po imenu "GoPro" (`-device-label`), `packaging/` (unit sa `/usr/bin/gpwebcam`, modules-load, modprobe, DEP-5 copyright, poruka posle instalacije), `.goreleaser.yaml`. `goreleaser release --snapshot --clean` pravi deb, rpm i Arch za amd64 i arm64; sadržaj i metapodaci provereni (`.PKGINFO`, deb `control`, liste fajlova, vlasnik root, prava 0644/0755). Unit sa svim ograničenjima (`ProtectSystem=strict`, `PrivateTmp` i ostala) radi kao privremeni user servis kroz `systemd-run --user`.
- 2026-10-05: README podeljen: `README.md` za korisnike (instalacija po distribuciji, modul, servis, opcije, rešavanje problema), `CONTRIBUTING.md` za one koji razvijaju (alati, raspored koda, build, testovi, paketi, pravila). Projektna pravila za agente su u `AGENTS.md`, a `CLAUDE.md` samo uvozi taj fajl.
- Sledeće: instalacija Arch paketa i `systemctl --user enable --now gpwebcam.service`; posle restarta proveriti da modprobe.d daje `/dev/video42` "GoPro" i drugi uređaj za OBS; man stranica i `gpwebcam doctor`; GitHub repo `darkodemic/gpwebcam`.
