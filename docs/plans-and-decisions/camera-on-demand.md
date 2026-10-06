# Kamera na zahtev i restart iz menija

- **Status:** Prihvaćen 2026-10-06: podrazumevano `demand`, zadrška 15 s, radi se pre preseka 2 iz `tray-and-recording.md` (§5).
- **Date:** 2026-10-06
- **Owner:** Darko
- **Related:** `tray-and-recording.md` (tray, podešavanja, snimanje); ADR 0003 (gpwebcam drži uređaj kao user servis); predajna beleška §2 (kamera, baterija)

## 1. Problem

Darko, 2026-10-06: čim se kamera priključi, servis je prebacuje u webcam režim i ona strimuje sve dok je servis upaljen, pa se greje bez potrebe. Želi da kamera radi samo kad treba, ručno ili, još bolje, kad je neka aplikacija (Zoom) zatraži, kao obična web kamera. Uz to pita da li u meniju treba stavka za restart servisa.

## 2. Provereno

| Činjenica | Izvor |
|---|---|
| v4l2loopback ima događaj `V4L2_EVENT_PRI_CLIENT_USAGE` (`V4L2_EVENT_PRIVATE_START + 0x08E00000 + 1`) sa telom `{ __u32 count }`: 1 dok neki čitač ima pokrenut video, 0 kad nijedan nema | `v4l2loopback.c` 0.15.4, linije 815-824 i 2147-2160, 2026-10-06 |
| Događaj stiže na `STREAMON` i `STREAMOFF` čitača i na zatvaranje uređaja, jer `close` radi `REQBUFS(0)`, a on `STREAMOFF`; to važi i kad je čitač ubijen sa `kill -9`. Odmah pri pretplati stiže trenutno stanje | isto, linije 2065-2130, 1703-1712, 2163-2170 |
| Samo otvaranje uređaja, na primer kad aplikacija nabraja kamere, ne šalje događaj | isto: događaj se šalje samo iz `streamon` i `streamoff` |
| Od koje verzije v4l2loopback ima ovaj događaj i koju verziju imaju Debian 13 i Ubuntu 24.04, nije provereno | treba proveriti |
| User systemd na session bus-u nudi `org.freedesktop.systemd1.Manager.RestartUnit` i `StopUnit`; servis ima `INVOCATION_ID`, pa zna da radi pod systemd-om | `busctl --user`, 2026-10-06 |
| Od klika do slike kamera se pokreće za 5.4 do 5.8 s, a jednom je trebalo oko 20 s | `tray-and-recording.md` §9 |

## 3. Predlog

### 3.1 Režimi

Nova postavka `camera`, u meniju i u `gpwebcam config`:

| Režim | Ponašanje |
|---|---|
| `demand` (na zahtev) | kamera kreće kad neka aplikacija pokrene video, a staje kad je nijedna ne koristi duže od zadrške (§3.2) |
| `always` (uvek) | kao sada: kamera kreće čim se priključi |
| `off` (pauza) | kamera se ne pokreće; zamenska slika kaže da je pauzirana, ikonica je izbledela |

Odlučeno 2026-10-06 (Darko): podrazumevano `demand`. Ako drajver nema događaj (pretplata vrati `EINVAL`), servis radi kao `always`, upiše to u log, a `doctor` upozori.

Dok kamera ne radi, zamenska slika i dalje ide u uređaj, pa aplikacije vide kameru. Kad aplikacija pokrene video, prvih oko 5 s vidi "Starting GoPro HERO13 Black…", a zatim sliku kamere.

### 3.2 Zadrška pre gašenja

Kamera ne staje odmah kad poslednja aplikacija zaustavi video, nego posle zadrške. Predlog je bio 30 s; Darko je 2026-10-06 izabrao 15 s. Aplikacije ponekad zaustave pa odmah ponovo pokrenu video, na primer pri prelazu iz pregleda u sastanak (nije provereno za Zoom). Bez zadrške bi svaki takav prelaz koštao 5 s crne slike.

### 3.3 Kamera između korišćenja

Servis pošalje STOP i EXIT, a USB mreža ostaje. Treba izmeriti da li se HERO13 sam gasi kad nema `keep_alive`-a. Ako se gasi, servis ga ne može probuditi preko USB-a, pa tada `keep_alive` mora da ide i između korišćenja: kamera je upaljena, ali senzor i enkoder ne rade.

### 3.4 Rezolucija bez restarta

Kad nijedna aplikacija nema pokrenut video, servis može ponovo da otvori uređaj u novoj veličini. Tada napomena "Applies when gpwebcam restarts" uglavnom nestaje. Izuzetak je aplikacija koja je postavila format, a video nije pokrenula: ona i dalje drži format, pa važi ispravka iz `301bf30` i ostaje stara veličina.

### 3.5 Restart iz menija

Stavka "Restart gpwebcam" poziva `RestartUnit("gpwebcam.service", "replace")` preko `godbus`-a, koji je već zavisnost. Prikazuje se samo kad servis radi pod systemd-om. Sa §3.4 služi uglavnom za oporavak, kad nešto zaglavi.

### 3.6 Meni

- Camera: On demand, Always on, Off
- Restart gpwebcam

Snimanje (`tray-and-recording.md` §5) pali kameru bez obzira na režim, dok traje.

## 4. Ikonica

Darko, 2026-10-06: bela dok sve radi, narandžasta kad ima grešku, izbledela bela kad kamera ne radi. Urađeno: bela dok video teče; narandžasta za probleme; bela na 45 % za sva ostala stanja (nema kamere, pokreće se, a sa ovim predlogom i pauza i čekanje na aplikaciju). Tanka tamna ivica drži belu ikonicu vidljivom na svetlom panelu.

## 5. Odluke

Darko, 2026-10-06:

1. Podrazumevani režim je `demand`.
2. Zadrška pre gašenja je 15 s.
3. Rad na zahtev ide pre preseka 2 iz `tray-and-recording.md` (sopstveni UDP prijem): ne zavisi od njega, a rešava zagrevanje.

## 6. Gde smo i šta sledi

- 2026-10-06: Darkovo pitanje, provere iz §2 i ovaj predlog. Ikonica promenjena (§4). Darkove odluke iz §5.
- 2026-10-06, kamera na Darkovoj mašini: setting 59 (auto power down) = 4, što je po Open GoPro specifikaciji 5 minuta. Zato servis i između korišćenja šalje `keep_alive` (§3.3); da li bi se kamera bez njega zaista ugasila na USB-u, nije mereno.
- 2026-10-06, napisano (testovi prolaze sa `-race`, i na Go 1.22), bez §3.4:
  - `v4l2.WatchUsage`: drugi deskriptor uređaja samo za događaje, pretplata sa `V4L2_EVENT_SUB_FL_SEND_INITIAL` (bez te zastavice početno stanje ne stiže), `select` na izuzetnom uslovu i `VIDIOC_DQEVENT`. Veličine struktura i brojevi ioctl-ova provereni C programom protiv `linux/videodev2.h` (136 i 32 bajta, `0x80885659`, `0x4020565a`). Uživo na `/dev/video42`: početno "ne koristi" odmah, "koristi" 1.05 s posle pokretanja ffmpeg čitača, "ne koristi" kad je završio posle 2 s.
  - Postavka i flag `camera` (`demand`, `always`, `off`); meni Camera i "Restart gpwebcam" (`GetUnitByPID` pa `Unit.Restart` preko `godbus`-a, samo uz `INVOCATION_ID`).
  - Servis: dok kamera ne treba da radi, zamenska slika "<model> ready" ili "Camera off…", uz `keep_alive`; sesija se završava 15 s posle poslednje aplikacije (`errIdle`), a promena režima odmah. Notifikacija "connected" sada stiže pri priključenju, sa rečenicom o tome šta sledi, umesto pri svakom početku videa.
  - `doctor` proverava da li modul javlja upotrebu.
- 2026-10-06, proba uživo (build iz radnog stabla umesto servisa, ffmpeg čitač umesto Zoom-a, 720p):

  | Korak | Rezultat |
  |---|---|
  | servis krene, niko ne koristi uređaj | kamera ostaje u statusu 0 (off) |
  | čitač pokrene video | webcam start posle 2.4 s, video posle 4.0 s; čitač prve 4 s dobija zamensku sliku (YAVG oko 45), zatim frejmove kamere (YAVG 8 do 10) |
  | čitač završi | kamera stane posle 16.3 s (zadrška 15 s plus provera na 0.5 s) |
  | `camera always` | video posle 3.5 s |
  | `camera off` | kamera stane odmah |
  | `camera demand` | kamera ostaje off |

- Ispravke posle probe: nepoznat ključ u `settings.json` više ne obara ceo fajl, nego se prijavi kao upozorenje (`UnknownKeysError`), a `Save` ga zadrži; inače bi stariji build fajl sa ključem `camera` odbacio i vratio podrazumevane vrednosti (tako bi se ponašao build `dbde29f` instaliran 2026-10-06). Režim off ima svoj razlog prekida (`errOff`) i poruku u logu; "found camera" se upisuje samo pri priključenju i preimenovanju interfejsa.
- Usput 2026-10-06: lokalne provere "na Go 1.22" kroz `mise exec go@1.22` u ovoj sesiji nisu bile na 1.22, jer je shell izvozio `GOROOT` za 1.27.1, pa je Go prešao na 1.27.1. Prava provera: `mise exec go@1.22 -- env -u GOROOT -u GOBIN GOTOOLCHAIN=local GOWORK=off go test ./...`. CI koristi pravi Go 1.22 i bio je zelen.
- 2026-10-06: commit `ee8ee18`, CI zelen. Darko instalirao paket napravljen iz `ee8ee18` (worktree bez `go.work`, sa čekboksovima) i probao sa Zoom-om, meni Camera i restart iz menija: "sve lepo radi".
- 2026-10-06, §3.4 napisano (testovi prolaze sa `-race`, i na pravom Go 1.22, protiv `tray.go` i `go.mod` iz commit-a):
  - `feed.Swap` menja uređaj pod lock-om feed-a, pa se između zatvaranja starog i otvaranja novog ne piše nijedan frejm. Stari izlaz mora prvi da se zatvori, jer v4l2loopback ima jedan izlazni format token.
  - `maybeResize` radi samo između sesija: u čekanju na kameru, u čekanju na aplikaciju i pre nove sesije. Uslov je da se podešena rezolucija razlikuje od one u upotrebi, da modul javlja upotrebu i da nijedna aplikacija nema pokrenut video. Ako uređaj zadrži staru veličinu (aplikacija postavila format bez videa), ta rezolucija se ne pokušava ponovo dok se upotreba ne promeni ili dok se rezolucija ne izabere iznova. Ako se uređaj ne otvori ni u jednoj veličini, upis u zatvoren izlaz pada i servis izlazi, pa ga systemd pokreće ponovo.
  - U režimu always sesija se sama ne završava, pa je `watchDemand` prekida kad promena čeka, a aplikacija nema.
  - Napomena u meniju i tekstovi u README-u, man stranici i `gpwebcam config -h`: nova rezolucija važi čim nijedna aplikacija ne koristi kameru.
- 2026-10-06, proba §3.4 uživo (build sa čekboksovima, bez fork-a; ffmpeg čitač umesto aplikacije): `res 1080` dok niko ne koristi uređaj, pa uređaj pređe na 1920x1080 za 0.1 s; čitač uključi video, kamera krene u 1080p, video posle 4.2 s; `res 720` dok čitač radi ne menja uređaj; čitač završi, kamera stane posle 17 s (zadrška), a uređaj odmah pređe na 1280x720.
- Sledeće: commit, pa presek 2 iz `tray-and-recording.md`.
