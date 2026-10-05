# 0002 — Open GoPro HTTP API za upravljanje kamerom

- **Status:** Accepted 2026-09-29, potvrđeno na kameri 2026-10-05. `gw` upravlja kamerom preko `/gopro/webcam/*` na portu 8080. Na HERO13 Black sa firmverom 02.10 to radi (`first-slice-gw-start.md` §7.1), pa stari `/gp/gpWebcam/*` nije potreban.
- **Date:** 2026-09-29
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `open-gopro-webcam-api.md` (nalazi iz specifikacije); `upstream-issues-review.md` §1.1; `first-slice-gw-start.md`; ADR 0001 (Go kao jezik implementacije)

## Context

Kamera se u webcam režim prebacuje HTTP pozivima. Postoje dva skupa endpoint-a:

- **Open GoPro:** `/gopro/webcam/{start,stop,exit,status,...}` na portu 8080. Dokumentovan je u Open GoPro HTTP API 2.0, a trenutna verzija specifikacije navodi HERO13 Black kao podržan model. `res` je kod (7 = 720p, 12 = 1080p), a `fov` se šalje u istom pozivu kao start. Tu su i `status` sa kodovima stanja i grešaka, `wired_usb` i `keep_alive` (`open-gopro-webcam-api.md` §2 do §5).
- **Stari:** `/gp/gpWebcam/{START,SETTINGS,STOP,EXIT}` na portu 80, koji koristi stari alat. `res` je broj linija (1080, 720), a `fov` ide posebnim `SETTINGS` pozivom. Nema ga ni u jednoj verziji specifikacije. Po upstream issue-ima radi na HERO12 Black i HERO13 Black, ali za HERO13 firmver nije naveden (`upstream-issues-review.md` §1.1).

Prva ciljna kamera je HERO13 Black sa firmverom 02.10. U trenutku odluke nijedan od dva skupa nije bio isproban na njoj.

## Decision

1. `gw` koristi Open GoPro HTTP API 2.0 na portu 8080.
2. Redosled pri startu prati state machine iz specifikacije:
   - `wired_usb?p=0`, sa ponavljanjem dok kamera ne odgovori;
   - `status`, pa `stop` ako je kamera ostala u preview stanju;
   - `start?res=..&fov=..&port=..&protocol=TS`, parametri tim redom;
   - `status` dok ne pokaže High Power Preview ili Low Power Preview.
3. Dok stream radi, `keep_alive` ide na svake 3 s.
4. Na kraju se šalju `stop` i `exit`, i drugi se šalje i kad prvi ne uspe.
5. Svaki odgovor se parsira. `error` različit od 0 je greška i u odgovoru na komandu i u odgovoru na `status`.
6. Stari API se ne implementira unapred. Ako na HERO13 02.10 Open GoPro ne radi, a stari radi, dodaje se kao drugi dijalekt iza opcije, i ova odluka dobija amandman.

## Consequences

**Positive**

- API je dokumentovan, sa kodovima stanja i grešaka, pa `gw` može da kaže šta tačno nije uspelo, umesto da svako neprazno telo tumači kao uspeh (upstream #28).
- FOV ide u istom pozivu kao start, bez drugog poziva i bez međustanja.
- `status` daje proveru da je stream zaista krenuo.

**Negative**

- Stari alat je radio sa starim API-jem, a Open GoPro preko USB-a na HERO13 u upstream issue-ima niko nije probao.
- HERO9 do HERO12 su u starijim buildovima specifikacije, a u trenutnom nisu; podrška za njih nije cilj, ali može da zavisi od firmvera.

**Risks**

- HERO13 v01.10.00 je vraćao HTTP 500 na webcam start (Open GoPro FAQ). GoPro kaže da je ispravljeno, ali na 02.10 to nije provereno. Ublaženo pravilom 6 i jasnom porukom sa HTTP kodom.
- Kodovi rezolucije za HERO13 nisu u tabeli specifikacije; 7 i 12 dolaze iz FAQ-a i SDK-a.

## Alternatives considered

- **Samo stari API:** potvrđeno od korisnika na HERO13, ali nedokumentovan, bez statusa i bez garancije da ostaje u novom firmveru.
- **Oba API-ja odmah, sa automatskim prelaskom:** više koda i testova pre nego što znamo da li je drugi uopšte potreban.
- **RTSP (`protocol=RTSP`):** kamera je server na portu 554, pa host ne mora da prima dolazni UDP, što zaobilazi problem sa firewall-om (upstream #2, #7, #42). Kašnjenje i stabilnost nisu poznati. Ostaje za merenje na kameri.

## Out of scope

- Podešavanja kamere van webcam poziva (Hypersmooth, preset-i).
- Wi-Fi i COHN.
