# Drugi presek: `gw run` i user servis

- **Status:** U izradi. Kod napisan 2026-10-05 i `go test ./...` prolazi. Na kameri je `gw run` pokrenuo stream; zamenska slika, Zoom posle `gw run` i servis pod systemd-om još nisu isprobani. Otvoreno je i kašnjenje od oko 0.8 s na putu kroz `gw` (§4).
- **Date:** 2026-10-05
- **Owner:** Darko
- **Related:** ADR 0003 (gw drži loopback uređaj i radi kao user servis); `first-slice-gw-start.md` §7.2 (Zoom i kašnjenje), §8; ADR 0002 (Open GoPro HTTP API za upravljanje kamerom)

## 1. Obim

- `gw` drži `/dev/videoN` otvorenim i sam piše frejmove; ffmpeg šalje dekodirane frejmove kroz pipe (ADR 0003).
- Zamenska slika dok kamere nema, sa stanjem u drugom redu.
- `gw run`: stalno radi, čeka kameru, pokreće sesiju, posle izvlačenja kabla vraća zamensku sliku za oko 0.5 s, posle neuspele sesije sa priključenom kamerom čeka 5 s pa pokušava ponovo.
- `contrib/systemd/gw.service`: systemd user servis za `gw run`.
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

Prvi rezultati, 2026-10-05: `gw run` je otvorio uređaj (odmah "Video Capture"), našao kameru i pokrenuo stream za 2.4 s; kašnjenje kroz novi put je isto kao kroz stari (`first-slice-gw-start.md` §7.2).

## 4. Kašnjenje

Kamera direktno u ffplay daje 0.25 s, a put kroz `gw` i `/dev/video42` oko 1.1 s (`first-slice-gw-start.md` §7.2). Razlika od oko 0.8 s nastaje u `gw`-ovom ffmpeg-u, u v4l2loopback-u ili u čitaču uređaja. Redosled provere:

1. Drugi čitač: `mpv av://v4l2:/dev/video42 --profile=low-latency --untimed`. Ako pokaže oko 0.3 s, kriv je ffplay-ev v4l2 ulaz, a Zoom treba meriti posebno.
2. Vreme od dolaska UDP paketa do upisa frejma u `gw`-ovom ffmpeg-u na pravom streamu (`-debug_ts` i `showinfo` uz `-loglevel +datetime`).
3. Razlike između `gw`-ovog ffmpeg-a i ffplay-a koji je dao 0.25 s: `timeout` na UDP ulazu, `-vf scale`, izlazni režim frejm-rate-a (`-fps_mode`).

## 5. Gde smo i šta sledi

- 2026-10-05: napisani `feed`, `placeholder`, `v4l2.OpenOutput`, `gw run` i `gw.service`; testovi prolaze. Izmerena kašnjenja po podešavanjima i bez `gw`-a.
- Sledeće: uzrok kašnjenja (§4), pa ručni testovi iz §3, pa README za `gw run` i servis.
