# 0003 — gw drži loopback uređaj i radi kao user servis

- **Status:** Accepted 2026-10-05. `gw` je jedini pisac u `/dev/video42`, drži ga otvorenim sve vreme rada i piše zamensku sliku dok kamere nema; `gw run` radi kao systemd user servis, bez udev pravila. Kod napisan, na kameri delimično isproban (`second-slice-gw-run.md`).
- **Date:** 2026-10-05
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `first-slice-gw-start.md` §7.2 (Zoom ne vidi kameru); `second-slice-gw-run.md`; `gopro-fork-review-and-handover.md` §5 (prvobitni tok preko udev-a i sistemskog servisa), §10.1 (notifikacije)

## Context

Zoom ne vidi GoPro ako je pokrenut pre `gw`-a. Uzrok je izmeren 2026-10-05 (`first-slice-gw-start.md` §7.2):

- Modul je učitan sa `exclusive_caps=1`, jer Chrome i slične aplikacije inače odbijaju uređaj. Uz tu opciju `/dev/video42` prijavljuje "Video Capture" samo dok ga neki pisac drži otvorenim, a inače "Video Output".
- Kad pisac otvori ili zatvori uređaj, kernel ne šalje nijedan udev događaj. Aplikacija koja je već pokrenuta zato ne može da sazna da se kamera pojavila.

U prvom preseku je pisac bio ffmpeg, pokrenut tek kad se kamera priključi, pa je uređaj bio "kamera" samo dok stream radi.

Prvobitni plan (predajna beleška §5) bio je udev pravilo koje pri priključenju pokreće sistemski `gw@<interfejs>.service`. Notifikacije na desktopu (§10.1) traže session D-Bus, koji ima samo korisnikov servis.

## Decision

1. `gw` otvara `/dev/videoN` jednom, pri startu, i drži ga otvorenim do izlaska. Sam postavlja format (`VIDIOC_S_FMT`, YU12, veličina po `-res`) i sam piše frejmove.
2. ffmpeg više ne piše u uređaj. Dekodira stream, skalira ga na veličinu uređaja i šalje sirove `yuv420p` frejmove `gw`-u kroz pipe (`-f rawvideo -flush_packets 1 pipe:1`).
3. Dok kamera ne šalje video, `gw` ponavlja zamensku sliku 10 puta u sekundi: "gw - GoPro webcam for Linux" i ispod stanje ("Camera not connected", "Connecting to camera", "No video from camera, retrying"). Sliku jednom iscrta ffmpeg (`drawtext`), a ako to ne uspe, koristi se jednobojna tamna slika.
4. Nova komanda `gw run` radi stalno: na svakih 0.5 s traži GoPro interfejs u sysfs-u, pokreće sesiju kad ga nađe, a kad interfejs nestane, za oko 0.5 s vraća zamensku sliku. `gw start` ostaje za jednu sesiju.
5. `gw run` radi kao systemd **user** servis (`contrib/systemd/gw.service`), pod korisnikom koji je prijavljen. Pristup uređaju daje uaccess ACL, root ne treba, a udev pravilo nije potrebno, jer servis sam čeka kameru.

## Consequences

**Positive**

- Uređaj je "Video Capture" od starta servisa, pa ga Zoom i ostale aplikacije vide nezavisno od redosleda pokretanja.
- Aplikacija dobija sliku i kad kamera nije priključena, i odmah zna u kom je stanju.
- Posle izvlačenja kabla zamenska slika se pojavi za oko 0.5 s, umesto da poslednji frejm stoji dok ffmpeg-ov timeout od 5 s ne istekne.
- `gw` broji i piše frejmove sam, pa watchdog za pakete koji ne stižu dobija prirodno mesto.
- User servis ima session D-Bus za notifikacije i ne traži root ni udev pravila.

**Negative**

- `gw` sadrži dva ioctl-a sa strukturama prevedenim iz `videodev2.h`. Raspored je proveren testom prema zaglavljima kernela na x86_64.
- Svaki frejm prolazi kroz pipe i još jednu kopiju (oko 93 MB/s za 1080p30). Na merenju 2026-10-05 stari i novi put dali su isto kašnjenje (1.13 s i 1.12 s).
- Zamenska slika troši malo procesora i dok niko ne gleda: oko 31 MB/s kopiranja u v4l2loopback.
- Dok `gw run` radi, `/dev/video42` je zauzet i drugi program ne može da piše u njega.

**Risks**

- Ako se modul učita bez `exclusive_caps=1` ili sa drugim brojem uređaja, `gw` ne može da otvori uređaj i servis se restartuje na 10 s, dok se modul ne podesi.
- Promena `-res` menja format uređaja; aplikacija koja je otvorila uređaj sa starim formatom mora ponovo da ga otvori.

## Alternatives considered

- **`exclusive_caps=0`:** uređaj stalno prijavljuje i capture i output. Chrome i WebRTC aplikacije takav uređaj odbijaju, a to je razlog zbog kog `exclusive_caps=1` postoji.
- **Pravljenje uređaja tek pri priključenju kamere (`v4l2loopback-ctl add`):** nov uređaj daje udev događaj, ali kontrolni uređaj traži root, a `gw` u radu ne sme da ga ima.
- **Ugrađena timeout slika v4l2loopback-a:** uređaj i dalje mora da ima pisca od starta, a podešava se spoljnim alatom.
- **Dva ffmpeg procesa, jedan za zamensku sliku, drugi za kameru:** uređaj se između njih zatvori, pa se capture na trenutak izgubi, a aplikacija koja čita može da ga ispusti.
- **udev pravilo i sistemski servis po interfejsu:** bez session D-Bus-a, a ne rešava problem sa Zoom-om.

## Out of scope

- Notifikacije na desktopu (predajna beleška §10.1).
- Više kamera istovremeno.
