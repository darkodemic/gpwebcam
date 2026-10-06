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
- Sledeće: implementacija; merenje da li se HERO13 bez `keep_alive`-a sam gasi (§3.3).
