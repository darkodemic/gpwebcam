# Upstream issue-i i PR-ovi: šta znače za gw

- **Status:** Završeno 2026-09-29. Pročitani svi issue-i i otvoreni PR-ovi; zaključci su uneti u `first-slice-gw-start.md` i u §2 beleške `gopro-fork-review-and-handover.md`.
- **Date:** 2026-09-29
- **Owner:** Darko
- **Related:** `gopro-fork-review-and-handover.md` §7 (upstream kao baza znanja); `first-slice-gw-start.md`; `open-gopro-webcam-api.md`

Izvor je `jschmid1/gopro_as_webcam_on_linux`, pročitano kroz `gh` 2026-09-29: 61 issue (36 otvorenih), 7 otvorenih PR-ova i spojeni PR-ovi sa činjenicama (#77, #68, #24, #15, #10, #60, #26). Kod se ne prenosi; ovde je samo ponašanje kamere i okruženja. Brojevi su issue-i i PR-ovi u upstream repou. Oznaka **[spec]** znači da činjenica dolazi iz Open GoPro specifikacije, a **[zaključak]** da je to izvod, ne prijava korisnika.

## 1. Ponašanje kamere

### 1.1 Endpoint-i i odgovori

- Stari endpoint-i su na HTTP portu 80 na adresi `.51`: `/gp/gpWebcam/START?res=1080|720|480[&port=N]`, `/gp/gpWebcam/SETTINGS?fov=<id>`, `/gp/gpWebcam/STOP`, `/gp/gpWebcam/EXIT` (#24, #30, #41, #59, #68, #77, PR #80).
- FOV id-evi: wide 0, narrow 2, superview 3, linear 4. Id 6 za narrow je bio pogrešan (#63, ispravljeno u #77).
- Na HERO13 Black stari endpoint-i rade: START bez `fov`, pa `SETTINGS?fov=` (komentar u #77, mart 2026, firmver nije naveden). Rade i na HERO12 Black (#63, #85).
- START bez parametara radi (PR #80). Nepoznata putanja vraća HTTP 404 sa telom `{}` (#77).
- Odgovor je JSON `{"status":N,"error":N}`. Stari skript svako neprazno telo tumači kao uspeh, pa je `{"status":1,"error":1}` prijavljen kao uspeh, a slike nije bilo (#28).
- [spec] status: 0 Off, 1 Idle, 2 High Power Preview, 3 Low Power Preview. error: 0 None, 1 Set Preset, 2 Set Window Size, 3 Exec Stream, 4 Shutter, 5 Com timeout, 6 Invalid param, 7 Unavailable, 8 Exit. Znači #28 je "idle, preset nije uspeo", a `status 2` posle START-a znači da stream ide.
- Stariji firmveri nemaju webcam: HERO8 fw 2.0 nema `gpWebcam`, 2.5 ima (#59); HERO8 fw 01.60 nema meni za USB režim (#9).

### 1.2 Mreža

- Kamera je DHCP server na `.51` i hostu daje `.52` do `.54` u /24 mreži; zakup oko 4.8 dana (#30).
- Viđene mreže: 172.21.112, 172.26.167, 172.27.199, 172.23.118, 172.21.155, 172.22.149, 172.22.133, 172.29.174, 172.20.161, 172.28.103. Sve odgovaraju [spec] šemi `172.2X.1YZ.51`, gde je XYZ poslednje tri cifre serijskog broja.
- Svaka prijava "pogrešna IP adresa" je u stvari pogrešan interfejs (#9, #30, #40, #47, #52, #65, #70) ili NetworkManager u režimu `ipv4.method=shared`, gde host dobije `10.42.0.1` (PR #71).
- Interfejs bez IPv4 adrese: neupravljan ili nepodešen interfejs (#14, #27, #54, PR #79). HERO10 sa fw 01.62 ne dobije adresu ni ručno (#54, nerešeno).
- Firewall odbacuje dolazni UDP, a kamera je u webcam režimu i START vraća `status 2`: #2, #7, #42, #55, PR #26, PR #60. VPN kvari izbor interfejsa ili start (#41, #70).

### 1.3 USB i interfejs

- Imena interfejsa su raznolika: `enx<mac>`, `enp0s20f0u1`, `enp57s0u1u2`, `usb0` (#7, #17, #27, #30, #54, PR #71). Product string takođe: "HERO8 BLACK", "GoPro HERO9", "HERO10 Black" (#15, #17, #24, #36). Vendor ID `2672` je uvek isti; HERO12 Black ima product ID `0059` (PR #72).
- Interfejs postoji samo u USB režimu GoPro Connect, ne MTP (#9, #52, #65). Sa Media Mod-om interfejs se ne pojavi (#47).
- USB mrežni drajver se ne pominje nigde. [spec] Open GoPro preko USB-a traži NCM, pa je verovatno `cdc_ncm`. Treba proveriti na kameri.
- Posle gašenja i paljenja kamere sa uključenim kablom kamera ne prikazuje ni USB ni webcam, i START ne uspeva dok se kabl ne izvuče i vrati (#74, nerešeno).

### 1.4 Vreme

- U trenutku udev `add` događaja interfejs još nema IPv4; restart servisa 15 s kasnije uspe (#17, PR #15).
- PR-ovi čekaju adresu do 15 s (PR #76) ili 10 s DHCP plus 10 s (PR #79). PR #80 čeka 3 s posle START-a. PR #76 ponavlja START posle 3 s, pa na svakih 5 s, dok širina slike na `/dev/video42` ne pređe 640; kamera "ponekad ne krene posle prvog START-a".
- Tačno merenje od priključenja do HTTP odgovora ne postoji.

### 1.5 Stream

- Kamera šalje MPEG-TS preko UDP-a na host:8554, na kameri ništa ne sluša (#59).
- Sadržaj TS-a (#56): H.264 High 1920x1080 `yuvj420p` 29.97 fps, AAC 48 kHz stereo, privatni stream `0x80` i AC3 sa 0 kanala. Ulazak usred GOP-a daje "non-existing PPS 0" dok ne stigne prvi IDR.
- Najviše 1080p30, bez zvuka u webcam režimu (#36, #44, #84). `-r 720` na HERO8 nije imao efekat (#32).
- Kašnjenje: oko 500 do 700 ms na x86, od toga ~700 ms za HERO13 na i9-12900K; nekoliko sekundi na Jetson-u (#46).
- Zaustavljanje servisa ostavlja kameru u webcam režimu; HERO8 posle toga ne može ponovo da se poveže do gašenja kamere (#33, #43).

### 1.6 v4l2loopback

- Modul dele OBS Virtual Camera i droidcam, pa izbacivanje ne uspeva ili ih kvari (#12, #48, #53, PR #81).
- Sa `exclusive_caps=1` aplikacije vide uređaj samo dok neko piše u njega (#13, #42). Desktop Skype i Teams ga zato ne vide (#31).
- `Operation not permitted` pri otvaranju `/dev/video42` (#56).

## 2. Zahtevi za gw

| # | Zahtev | Izvor | Gde |
|---|---|---|---|
| 1 | Interfejs po vendor ID-u `2672` u sysfs-u, nikad po imenu, "poslednjem" ili "bilo kojoj 172.x" adresi | §1.3; PR #10, PR #82 | prvi presek |
| 2 | Čekati IPv4 bar 20 do 30 s, ne pasti odmah | #17, PR #76, PR #79 | prvi presek |
| 3 | Adresa kamere je host /24 + `.51`, uz proveru šeme `172.2X.1YZ.0/24`; inače jasna poruka (NetworkManager shared, statička adresa) | §1.2; PR #71 | prvi presek |
| 4 | Parsirati `status` i `error`; posle START-a očekivati `status 2`, `error 0` | #28 | prvi presek |
| 5 | Pre START-a pročitati status; ako je kamera ostala u webcam režimu, prvo STOP | #33, #43 | prvi presek |
| 6 | STOP na SIGTERM i SIGINT, pa ugasiti ffmpeg | #33, #43, PR #80 | prvi presek |
| 7 | ffmpeg: `-map 0:v:0`, opcije za nisku latenciju pre `-i` | #56, PR #80 | prvi presek |
| 8 | Watchdog: ako posle START-a nema paketa za 3 do 5 s, ponoviti START nekoliko puta, pa prijaviti "kamera šalje, paketi ne stižu: firewall ili VPN" | PR #76; §1.2 | sledeći presek |
| 9 | Ruta ka kameri mora ići preko GoPro interfejsa (VPN) | #41, #70 | sledeći presek |
| 10 | Prepoznati "interfejs postoji, HTTP ne odgovara" i reći korisniku da izvuče i vrati kabl | #74 | sledeći presek |
| 11 | Instalacija: NetworkManager keyfile ili systemd-networkd `.network` za GoPro interfejse (ipv4 auto, never-default, bez DNS-a); rezervni v4l2loopback uređaj za OBS | PR #71, PR #79, #48, #53, #64 | instalacija |
| 12 | Firewall: bez root-a ga ne možemo popraviti; otkriti firewalld, ufw i nft i ispisati tačno pravilo za 8554/udp na GoPro interfejsu | §1.2 | kasnije |
| 13 | Poruke o grešci koje razlikuju: nema uređaja `2672` (MTP, Media Mod, stari firmver), nema IP-a, pogrešna mreža, HTTP ne odgovara, START odbijen sa dekodiranim kodom, nema paketa, v4l2 ne može da se otvori | #9, #30, #40, #47, #52, #65, #70, #74 | prvi presek za ono što već radi |
| 14 | Jedna instanca po interfejsu; više kamera traži svoj `port` i svoj uređaj | #67, PR #68, #85 | kasnije |

## 3. Za proveru na HERO13 sa firmverom 02.10

- USB drajver (`cdc_ncm`?) i product ID.
- HTTP na :80, :8080 ili oba; odgovori `/gopro/webcam/*` i `/gp/gpWebcam/*`.
- Vreme od priključenja do DHCP adrese i do prvog HTTP odgovora.
- Da li je UDP izvorni port kamere stalan.
- Da li RTSP (`protocol=RTSP`) radi i kakvo mu je kašnjenje.
- Da li kamera zaspi bez keep-alive poziva.
- Ponašanje iz #74 (gašenje kamere sa uključenim kablom).
