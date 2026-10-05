# Izdanje 0.1.0

- **Status:** U izradi. Obim odlučen 2026-10-05: M1 do M4, F1, F2, F4, O1 do O4; F3 (snimanje) ide posle 0.1.0.
- **Date:** 2026-10-05
- **Owner:** Darko
- **Related:** `packaging-and-release.md` §6 (kanali), §7; `second-slice-gw-run.md` §3.1, §5; ADR 0005 (ime gpwebcam i paketi kroz GoReleaser); predajna beleška §10 (ideje)

## 1. Obavezno pre taga

| # | Stavka | Zašto |
|---|---|---|
| M1 | Instalacija Arch paketa, `systemctl --user enable --now gpwebcam.service`, restart; provera da modprobe.d daje `/dev/video42` "GoPro" i drugi uređaj "OBS Virtual Camera", da servis krene pri prijavi i da ga Zoom vidi | paket i servis iz paketa još nisu isprobani zajedno; `card_label` sa zarezom i razmakom nije proveren (ADR 0005, Risks) |
| M2 | Jednom izvući i vratiti kabl sa trikom start pa stop | trik je proveren samo na lažnoj kameri |
| M3 | Čekanje na uređaj "GoPro" umesto izlaska kad modul još nije učitan | sada servis izlazi i systemd ga restartuje na 10 s, bez kraja, sa porukom u journal-u svaki put; i pri boot-u servis može da krene pre modula |
| M4 | GitHub repo `darkodemic/gpwebcam`, CI (vet, test, build paketa na svaki push), GoReleaser na tag `v0.1.0`, prvo kao draft release | bez toga nema javnog izdanja; svaki korak traži Darkovo odobrenje |

## 2. Funkcije, po izboru

| # | Stavka | Procena | Napomena |
|---|---|---|---|
| F1 | `gpwebcam doctor`: modul i imena uređaja, prava na uređaj, ffmpeg i H.264 dekoder (Fedora `ffmpeg-free`), kamera na USB-u i njen režim, IPv4 adresa i NetworkManager "shared", firewall, stanje servisa | srednje | najveća korist za podršku: većina upstream prijava su pogrešno podešavanje (`upstream-issues-review.md` §2, stavka 13) |
| F2 | Notifikacije na desktopu: kamera povezana, isključena, greška | malo uz `notify-send` (bez zavisnosti) | predajna beleška §10.1; user servis ima session D-Bus |
| F3 | Snimanje: kopija H.264 streama u `.mkv` uz webcam, bez ponovnog kodiranja | srednje | predajna beleška §10.2 |
| F4 | Man stranica `gpwebcam.1` | malo | Debian lintian daje upozorenje bez nje; za Debian kasnije svakako treba |

## 3. Optimizacije i dorada, po izboru

| # | Stavka | Procena | Napomena |
|---|---|---|---|
| O1 | Tiši log: ffmpeg upozorenja pri svakom startu (stream 2 i 3, `yuvj420p`), preimenovanje `eth0` → `enp...` kao info umesto lažnog "unplugged" | malo | čisto kozmetika, ali journal je ono što korisnik čita |
| O2 | Izmeriti potrošnju procesora pri 1080p30, pa po potrebi hardversko dekodiranje (VAAPI) | merenje malo, VAAPI srednje | sada se dekodira softverski |
| O3 | Dve aplikacije istovremeno: druga dobija "Device or resource busy" (provereno 2026-10-05 sa dva ffmpeg čitača) | nepoznato | može biti ograničenje v4l2loopback-a; prvo istražiti, inače zapisati kao ograničenje u README. Ishod: pravilo V4L2 API-ja; zapisano u README; drugi uređaj "GoPro 2" posle 0.1.0 (§4) |
| O4 | Više stanja na zamenskoj slici: "kamera nađena, čeka mrežu", "firewall?" | malo | korisnik vidi šta se dešava bez journal-a |

## 4. Posle 0.1.0

- AUR: izvorni PKGBUILD `gpwebcam` (sa `optdepends`) i `gpwebcam-bin` preko GoReleaser `aurs` (`packaging-and-release.md` §6).
- COPR za Fedoru; Debian ITP; RPM Fusion.
- Više kamera istovremeno.
- Drugi uređaj "GoPro 2" u koji `gpwebcam` piše iste frejmove, za dve aplikacije odjednom (npr. streaming); traži treći v4l2loopback uređaj u konfiguraciji modula.
- RTSP kao alternativa za mreže sa firewall-om (direktno je dao isto kašnjenje, 0.18 s).

## 5. Gde smo i šta sledi

- 2026-10-05: spisak napravljen posle commit-a `4934ca6`.
- 2026-10-05: Darko izabrao F1, F2, F4 i O1 do O4; F3 posle 0.1.0. Pitanje uz O2: zašto hardversko dekodiranje nije podrazumevano kad postoji. Odgovor: treba da bude, ako (1) ne poveća kašnjenje, jer neki VAAPI drajveri drže više frejmova, a frejm ionako mora nazad u sistemsku memoriju zbog v4l2loopback-a, i (2) sam padne na softversko kad GPU ili drajver nisu dostupni (`-hwaccel auto` to radi). O2 meri procesor i kašnjenje sa i bez njega i na osnovu toga odlučuje podrazumevanu vrednost.
- 2026-10-05, urađeno (testovi prolaze, i sa `-race`):
  - M3: `gpwebcam run` čeka uređaj (provera na 2 s) umesto da izađe.
  - O1: ffmpeg sa `-loglevel error`; njegove linije idu kroz `slog` sa oznakom `source=ffmpeg`, najviše 20 u minutu, uz broj preskočenih. Preimenovanje interfejsa se prepoznaje po tome što USB uređaj i dalje postoji (druga provera posle 300 ms, jer pri izvlačenju interfejs može nestati pre uređaja) i beleži se kao info.
  - O4: sedam poruka na zamenskoj slici (nije povezana, čeka mrežu, pokreće se, nema videa, posle 3 puta "firewall?", ne odgovara, problem); sve provereno vizuelno na 1080p.
  - F2: `internal/notify` preko `notify-send`, ista poruka najviše jednom u 30 s; `-notify=false` gasi. Na Darkovoj mašini server je Quickshell.
  - F1: `gpwebcam doctor`: ffmpeg i H.264 dekoder, modul, uređaji, konfiguracija modula, pravo pisanja, servis, kamera (USB, interfejs, IPv4, ruta, HTTP info i status samo čitanjem), firewalld i ufw, notify-send. Izlaz 1 ako nešto padne.
  - F4: `packaging/man/gpwebcam.1`, bez groff upozorenja; GoReleaser ga kompresuje (`gzip -n`) u `/usr/share/man/man1/` u sva tri paketa.
  - O2: lokalni stream 1080p30 H.264 High, 6 Mb/s: softversko 13.7 % jednog jezgra i 71 ms, `-hwaccel vaapi` 9.5 % i 74 ms, `-hwaccel auto` (izabrao VAAPI na AMD-u) 8.7 % i 74 ms. Odluka: `-hwdec auto` podrazumevano, `-hwdec none` isključuje; posle dva uzastopna "nema videa" uz GPU, sesije prelaze na softversko do kraja rada.
  - O3: istraženo u izvornom kodu v4l2loopback 0.15.4. Od 0.14 jedan čitač po uređaju: drugi pada na `VIDIOC_S_FMT` (`v4l2loopback.c:1143-1146`, i `REQBUFS` na `:1725-1728`), bez obzira na format; isto važi za UVC kamere i to je pravilo V4L2 API-ja (upstream #635, #310). Chrome radi `S_FMT` bez ponovnog pokušaja. Jedino rešenje je drugi uređaj ("GoPro 2") u koji `gpwebcam` piše iste frejmove; za sada zapisano u README kao ograničenje, a drugi uređaj čeka Darkovu odluku.
- 2026-10-05: Darko: "GoPro 2" posle 0.1.0; ne vidi čest slučaj za dve aplikacije odjednom, osim možda za streaming.
- 2026-10-05, M1 delimično: Darko instalirao Arch paket (`pacman -Ql` pokazuje svih 7 fajlova) i ponovo učitao modul bez restarta (`sudo modprobe -r v4l2loopback && sudo modprobe v4l2loopback`, modul nije koristio nijedan program). Konfiguracija iz paketa daje `/dev/video42` "GoPro" i `/dev/video2` "OBS Virtual Camera", `exclusive_caps=Y,Y`; `card_label` sa zarezom i razmakom se ispravno deli (rizik iz ADR 0005 otpada). `systemctl --user enable --now gpwebcam.service` radi; servis nađe uređaj po imenu i piše zamensku sliku; `gpwebcam doctor`: 0 problema, upozorenje samo da kamera nije priključena. Ostaje: servis pri prijavi posle restarta i Zoom sa paketom.
- 2026-10-05, M2: Darko potvrdio da sa servisom iz paketa radi ceo test: kamera se sama nađe i pokrene, izvlačenje i vraćanje kabla rade, notifikacije stižu.
- 2026-10-05, posle commit-a `a65f5ec`, Darkove dve ideje:
  - Model u nazivu kamere: `card_label` se zadaje pri učitavanju modula i ne menja se bez root-a (novi uređaj), a uređaj postoji i bez kamere. Odluka: uređaj ostaje "GoPro"; model ("GoPro HERO13 Black", iz USB product string-a, očišćen jer dolazi sa uređaja) ide na zamensku sliku, u notifikacije i u log. Model van spiska proverenih (sada samo "HERO13 Black") se jednom prijavi u logu i notifikacijom, a `doctor` ga označi upozorenjem. README opisuje kako se stalni naziv postavlja kroz `/etc/modprobe.d` i `-device-label`.
  - Tačkice: stanja koja čekaju (mreža, pokretanje, ponovni pokušaj) vrte ".", "..", "..." na 0.5 s; poruke o problemima stoje mirno. Razmaci umesto tačkica ne pomažu, jer drawtext ne računa razmake na kraju u `text_w` (tekst se pomerao 6 do 7 px po tačkici); zato se status centrira bez tačkica, a tačkice crtaju odmah iza izmerene desne ivice teksta, na istoj osnovnoj liniji (`y_align=baseline`, ffmpeg 6.1+). Čuva se jedna osnovna slika i traka redova po statusu i koraku: oko 12 MB umesto ranijih ~22 MB. Provereno uživo sa kamerom.
  - Usput: `-hwaccel auto` prvo proba CUDA i na mašini bez NVIDIA drajvera upiše 3 linije greške pri svakom startu. Sada `-hwdec auto` jednom pri startu proba VAAPI uređaj (`-init_hw_device vaapi`, oko 50 ms) i koristi `-hwaccel vaapi` ili softversko. Eksplicitni `-hwaccel vaapi` ne pada sam na softversko kad uređaj ne radi (provereno), zato provera ide unapred.
- 2026-10-05, posle commit-a `6dd7ee1`: kad nova sesija pošalje START manje od sekunde posle `exit`-a prethodne (restart servisa), kamera ponekad prijavi status 2, a ne šalje ništa; tada je watchdog vraćao sliku tek za oko 16 s. Ponovljen START ne pomaže, jer ga kamera u stanju streaminga ignoriše (1 od 4 brza restarta, slika posle 15.7 s). Ispravka: 3 s posle START-a bez ijednog frejma, sesija pošalje stop pa START dok ffmpeg i dalje sluša; watchdog od 6 s ostaje kao poslednja zaštita. Merenje sa 16 brzih restarta: 15 normalnih (slika za 4.4 s od pokretanja `gpwebcam run`), 1 neuspeh koji je ispravka vratila za 8.9 s bez watchdog-a.
- Sledeće: restart kad Darku odgovara (servis pri prijavi, modul pri boot-u); M4 uz odobrenje svakog koraka.
