# Drugi presek: `gw run` i user servis

- **Status:** U izradi. Na kameri provereno 2026-10-05: kašnjenje u Zoom-u 0.18 s (bilo 1.1 s, §4), Zoom vidi uređaj i zamensku sliku bez restarta, izvlačenje i vraćanje kabla rade uz watchdog (§3.1). Servis pod systemd-om i README još nisu urađeni.
- **Date:** 2026-10-05
- **Owner:** Darko
- **Related:** ADR 0003 (gw drži loopback uređaj i radi kao user servis); `first-slice-gw-start.md` §7.2 (Zoom i kašnjenje), §8; ADR 0002 (Open GoPro HTTP API za upravljanje kamerom)

## 1. Obim

- `gw` drži `/dev/videoN` otvorenim i sam piše frejmove; ffmpeg šalje dekodirane frejmove kroz pipe (ADR 0003).
- Zamenska slika dok kamere nema, sa stanjem u drugom redu.
- `gw run`: stalno radi, čeka kameru, pokreće sesiju, posle izvlačenja kabla vraća zamensku sliku za oko 0.5 s, posle neuspele sesije sa priključenom kamerom čeka 5 s pa pokušava ponovo.
- `contrib/systemd/gw.service` (od ADR 0005 `packaging/systemd/gpwebcam.service`): systemd user servis za `gw run`.
- Uzrok kašnjenja od oko 0.8 s na putu kroz `gw` (§4).

Van obima: notifikacije, snimanje, više kamera, instalacija `modprobe.d` konfiguracije.

## 2. Paketi

| Paket | Promena |
|---|---|
| `internal/v4l2` | `OpenOutput`: QUERYCAP provera, `VIDIOC_S_FMT` YU12, `WriteFrame` |
| `internal/feed` | nov: ponavlja zamensku sliku na 100 ms dok nema živih frejmova |
| `internal/placeholder` | nov: ffmpeg `drawtext` iscrta jedan YU12 frejm; dozvoljeni znakovi u tekstu su slova, cifre, razmak, `-`, `,` i `.`, pa nema escape-ovanja |
| `internal/stream` | izlaz `-vf scale=W:H,format=yuv420p -f rawvideo -flush_packets 1 pipe:1`; `Run` čita cele frejmove i predaje ih funkciji |
| `internal/camera` | `Resolution.Size()` |
| `cmd/gw` | `run` i `start` dele `serve.go`; sesija se prekida kad interfejs nestane |

## 3. Test plan

Automatski, bez kamere:

- `v4l2`: veličina i raspored `v4l2_format` (208 bajtova, unija na offsetu 8) i broj `VIDIOC_S_FMT`, prema zaglavljima kernela.
- `feed`: zamenska slika se piše odmah i ponavlja, ne piše se dok idu živi frejmovi, i ponovo se piše posle `Idle`.
- `placeholder`: tekst se iscrta (svetli pikseli na tamnoj pozadini), nedozvoljeni znakovi se odbijaju.
- `stream`: frejmovi tačne veličine stižu do funkcije, kraj streama daje poruku "no video from the camera", greška pri upisu zaustavlja ffmpeg.

Ručno, sa kamerom:

1. `gw run` bez kamere: Zoom ili `ffplay -f v4l2 /dev/video42` pokazuje "Camera not connected".
2. Priključiti kameru: slika se prebaci na kameru; Zoom pokrenut pre toga i dalje vidi uređaj.
3. Izvući kabl: za oko 0.5 s vraća se "Camera not connected".
4. Servis: `systemd-run --user` sa istim podešavanjima kao `gw.service`, da se provere ograničenja (`ProtectSystem=strict`, `PrivateTmp` i ostala), pa instalacija i `systemctl --user enable --now gw.service`.

### 3.1 Rezultati na kameri, 2026-10-05

| Test | Rezultat |
|---|---|
| `gw run` bez kamere | odmah "Video Capture"; zamenska slika "Camera not connected" |
| priključivanje | "Connecting to camera", pa stream za oko 2.5 s |
| Zoom pokrenut posle `gw run` | vidi kameru GoPro; kašnjenje 0.18 s (§4) |
| restart `gw`-a dok Zoom radi | Zoom zadrži uređaj, slika se vrati za oko 3 s |
| izvlačenje kabla dok Zoom radi | zamenska slika u Zoom-u za najviše 1.6 s (snimak ekrana na oko 1.2 s) |
| vraćanje kabla | dva puta od dva: kamera prijavi status 2 posle START-a, a video ne stigne. Prvi put (bez watchdog-a) ffmpeg je odustao posle 18 s, a novi pokušaj 5 s kasnije je uspeo. Drugi put (watchdog od 6 s) "No video from camera, retrying" posle 7 s, novi pokušaj 2 s kasnije uspe; video oko 15 s posle vraćanja kabla |
| jedna greška dekodiranja | oko jednom u 5 minuta ("corrupt decoded frame"); slika se sama oporavi |

Usput uočeno:

- Posle priključivanja kernel prvo nazove interfejs `eth0`, a udev ga posle oko 0.5 s preimenuje u `enp0s20f0u1`. `gw` ponekad uhvati `eth0`, sesija se prekine kao "unplugged" i odmah krene sa novim imenom. Radi, ali log izgleda kao lažno izvlačenje.
- Status kamere pre START-a posle vraćanja kabla bio je oba puta "idle", a posle neuspelog pokušaja "off". Prvo priključivanje tog dana takođe je dalo "idle", ali START je tada uspeo. GoPro FAQ za "idle" posle novog USB povezivanja predlaže start pa stop; to bi moglo da skrati oporavak, ali nije provereno.
- ffplay ne može da otvori `/dev/video42` dok ga `mpv` čita ("Device or resource busy"). Treba proveriti da li Zoom i browser mogu istovremeno.

## 4. Kašnjenje

Rešeno 2026-10-05. Kamera direktno u ffplay daje 0.15 do 0.25 s, a put kroz `gw` i `/dev/video42` je davao oko 1.1 s (`first-slice-gw-start.md` §7.2). Merenja istim načinom (GoPro usmeren u sat sa milisekundama, snimak ekrana):

| Šta | Rezultat |
|---|---|
| `mpv --profile=low-latency --untimed` kao čitač umesto ffplay-a | 1.15 s: čitač nije kriv |
| u `gw`-ovom ffmpeg-u, od paketa do frejma posle filtera (`-debug_ts`, `showinfo`, `-loglevel +datetime`) | medijana 5 ms, najviše 15 ms; paketi se čitaju u realnom vremenu |
| v4l2loopback: ffmpeg piše test-sliku, drugi ffmpeg čita, frejmovi upareni po checksum-u | 35 do 40 ms |
| port otvoren 2 s posle START-a, umesto odmah | 0.15 do 0.18 s: redosled ne utiče |
| ffmpeg kao `gw` u `/dev/video42`, bez izmena | 1.02 s |
| isto, bez `timeout` na UDP-u | 1.18 s |
| isto, bez `scale` | 1.13 s |
| **isto, sa `-fps_mode passthrough`** | **0.18 s** |
| `gw run` sa `-fps_mode passthrough`, čitač `mpv` | 0.18 do 0.20 s |
| isto, Zoom (self view u sastanku) | 0.18 s, ugnežđeni snimak 0.20 s |

Uzrok: ffmpeg za izlaz u `rawvideo` i `v4l2` podrazumevano koristi konstantan frame rate (CFR), i sa kamerinim streamom to drži frejmove oko 0.85 s. `gw` sada daje `-fps_mode passthrough`, pa svaki dekodirani frejm odmah ide dalje. Lokalni test-stream iz §5.1 prvog preseka ovo nije pokazao, jer je merio izlaz `-f null` sa savršenim vremenskim oznakama.

## 5. Gde smo i šta sledi

- 2026-10-05: napisani `feed`, `placeholder`, `v4l2.OpenOutput`, `gw run` i `gw.service`; testovi prolaze. Izmerena kašnjenja po podešavanjima i bez `gw`-a.
- 2026-10-05: kašnjenje rešeno sa `-fps_mode passthrough` (§4). Dodat watchdog: ako 6 s posle START-a ne stigne frejm, sesija se prekida i ponavlja posle 2 s. Zamenska slika se prikaže čim interfejs nestane, ne čeka ffmpeg. Status kamere pre START-a se upisuje u log. Ručni testovi sa Zoom-om prošli (§3.1).
- Sledeće: servis pod systemd-om (§3, korak 4); README za `gw run` i servis; po želji start pa stop kad je kamera "idle" posle priključivanja, i stabilno ime interfejsa pre sesije (§3.1).
