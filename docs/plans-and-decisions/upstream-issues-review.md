# Upstream issues and PRs: what they mean for gw

- **Status:** Done 2026-09-29. Read all issues and open PRs; the conclusions went into `first-slice-gw-start.md` and into §2 of the handover note `gopro-fork-review-and-handover.md`.
- **Date:** 2026-09-29
- **Owner:** Darko
- **Related:** `gopro-fork-review-and-handover.md` §7 (upstream as a knowledge base); `first-slice-gw-start.md`; `open-gopro-webcam-api.md`

The source is `jschmid1/gopro_as_webcam_on_linux`, read through `gh` on 2026-09-29: 61 issues (36 open), 7 open PRs, and the merged PRs that hold facts (#77, #68, #24, #15, #10, #60, #26). No code is carried over; this covers only the behavior of the camera and the environment. Numbers are issues and PRs in the upstream repo. The marker **[spec]** means the fact comes from the Open GoPro specification, and **[inference]** means it is a deduction, not a user report.

## 1. Camera behavior

### 1.1 Endpoints and responses

- The old endpoints are on HTTP port 80 at the `.51` address: `/gp/gpWebcam/START?res=1080|720|480[&port=N]`, `/gp/gpWebcam/SETTINGS?fov=<id>`, `/gp/gpWebcam/STOP`, `/gp/gpWebcam/EXIT` (#24, #30, #41, #59, #68, #77, PR #80).
- FOV ids: wide 0, narrow 2, superview 3, linear 4. Id 6 for narrow was wrong (#63, fixed in #77).
- On HERO13 Black the old endpoints work: START without `fov`, then `SETTINGS?fov=` (comment in #77, March 2026, firmware not given). They also work on HERO12 Black (#63, #85).
- START without parameters works (PR #80). An unknown path returns HTTP 404 with the body `{}` (#77).
- The response is JSON `{"status":N,"error":N}`. The old script reads any non-empty body as success, so `{"status":1,"error":1}` was reported as success, but there was no picture (#28).
- [spec] status: 0 Off, 1 Idle, 2 High Power Preview, 3 Low Power Preview. error: 0 None, 1 Set Preset, 2 Set Window Size, 3 Exec Stream, 4 Shutter, 5 Com timeout, 6 Invalid param, 7 Unavailable, 8 Exit. So #28 is "idle, preset failed", and `status 2` after START means the stream is running.
- Older firmware has no webcam: HERO8 fw 2.0 has no `gpWebcam`, 2.5 has it (#59); HERO8 fw 01.60 has no menu for USB mode (#9).

### 1.2 Network

- The camera is a DHCP server on `.51` and gives the host `.52` to `.54` in a /24 network; lease about 4.8 days (#30).
- Networks seen: 172.21.112, 172.26.167, 172.27.199, 172.23.118, 172.21.155, 172.22.149, 172.22.133, 172.29.174, 172.20.161, 172.28.103. All match the [spec] scheme `172.2X.1YZ.51`, where XYZ are the last three digits of the serial number.
- Every "wrong IP address" report is in fact the wrong interface (#9, #30, #40, #47, #52, #65, #70) or NetworkManager in `ipv4.method=shared` mode, where the host gets `10.42.0.1` (PR #71).
- Interface without an IPv4 address: an unmanaged or unconfigured interface (#14, #27, #54, PR #79). HERO10 with fw 01.62 does not get an address even manually (#54, unresolved).
- The firewall drops incoming UDP, while the camera is in webcam mode and START returns `status 2`: #2, #7, #42, #55, PR #26, PR #60. A VPN breaks the interface choice or the start (#41, #70).

### 1.3 USB and interface

- Interface names vary: `enx<mac>`, `enp0s20f0u1`, `enp57s0u1u2`, `usb0` (#7, #17, #27, #30, #54, PR #71). So does the product string: "HERO8 BLACK", "GoPro HERO9", "HERO10 Black" (#15, #17, #24, #36). Vendor ID `2672` is always the same; HERO12 Black has product ID `0059` (PR #72).
- The interface exists only in the GoPro Connect USB mode, not MTP (#9, #52, #65). With the Media Mod the interface does not show up (#47).
- The USB network driver is not mentioned anywhere. [spec] Open GoPro over USB requires NCM, so it is probably `cdc_ncm`. To be checked on the camera.
- After the camera is turned off and on with the cable plugged in, the camera shows neither USB nor webcam, and START fails until the cable is unplugged and plugged back in (#74, unresolved).

### 1.4 Timing

- At the moment of the udev `add` event the interface has no IPv4 yet; a service restart 15 s later succeeds (#17, PR #15).
- PRs wait for the address up to 15 s (PR #76) or 10 s DHCP plus 10 s (PR #79). PR #80 waits 3 s after START. PR #76 repeats START after 3 s, then every 5 s, until the picture width on `/dev/video42` exceeds 640; the camera "sometimes does not start after the first START".
- There is no exact measurement from plugging in to the HTTP response.

### 1.5 Stream

- The camera sends MPEG-TS over UDP to host:8554; nothing listens on the camera (#59).
- Content of the TS (#56): H.264 High 1920x1080 `yuvj420p` 29.97 fps, AAC 48 kHz stereo, private stream `0x80` and AC3 with 0 channels. Joining in the middle of a GOP gives "non-existing PPS 0" until the first IDR arrives.
- At most 1080p30, no audio in webcam mode (#36, #44, #84). `-r 720` on HERO8 had no effect (#32).
- Latency: about 500 to 700 ms on x86, including ~700 ms for HERO13 on an i9-12900K; several seconds on a Jetson (#46).
- Stopping the service leaves the camera in webcam mode; after that, HERO8 cannot connect again until the camera is turned off (#33, #43).

### 1.6 v4l2loopback

- OBS Virtual Camera and droidcam share the module, so unloading it fails or breaks them (#12, #48, #53, PR #81).
- With `exclusive_caps=1` applications see the device only while something writes to it (#13, #42). That is why desktop Skype and Teams do not see it (#31).
- `Operation not permitted` when opening `/dev/video42` (#56).

## 2. Requirements for gw

| # | Requirement | Source | Where |
|---|---|---|---|
| 1 | Interface by vendor ID `2672` in sysfs, never by name, the "last" one or "any 172.x" address | §1.3; PR #10, PR #82 | first slice |
| 2 | Wait for IPv4 for at least 20 to 30 s, do not fail at once | #17, PR #76, PR #79 | first slice |
| 3 | The camera address is the host /24 + `.51`, with a check of the `172.2X.1YZ.0/24` scheme; otherwise a clear message (NetworkManager shared, static address) | §1.2; PR #71 | first slice |
| 4 | Parse `status` and `error`; after START expect `status 2`, `error 0` | #28 | first slice |
| 5 | Before START read the status; if the camera stayed in webcam mode, STOP first | #33, #43 | first slice |
| 6 | STOP on SIGTERM and SIGINT, then stop ffmpeg | #33, #43, PR #80 | first slice |
| 7 | ffmpeg: `-map 0:v:0`, low-latency options before `-i` | #56, PR #80 | first slice |
| 8 | Watchdog: if no packets arrive for 3 to 5 s after START, repeat START a few times, then report "the camera sends, packets do not arrive: firewall or VPN" | PR #76; §1.2 | next slice |
| 9 | The route to the camera must go through the GoPro interface (VPN) | #41, #70 | next slice |
| 10 | Recognize "the interface exists, HTTP does not answer" and tell the user to unplug and replug the cable | #74 | next slice |
| 11 | Installation: a NetworkManager keyfile or a systemd-networkd `.network` for GoPro interfaces (ipv4 auto, never-default, no DNS); a spare v4l2loopback device for OBS | PR #71, PR #79, #48, #53, #64 | installation |
| 12 | Firewall: without root we cannot fix it; detect firewalld, ufw and nft and print the exact rule for 8554/udp on the GoPro interface | §1.2 | later |
| 13 | Error messages that tell apart: no `2672` device (MTP, Media Mod, old firmware), no IP, wrong network, HTTP does not answer, START rejected with a decoded code, no packets, v4l2 cannot be opened | #9, #30, #40, #47, #52, #65, #70, #74 | first slice for what already works |
| 14 | One instance per interface; multiple cameras need their own `port` and their own device | #67, PR #68, #85 | later |

## 3. To check on HERO13 with firmware 02.10

- USB driver (`cdc_ncm`?) and product ID.
- HTTP on :80, :8080 or both; the responses of `/gopro/webcam/*` and `/gp/gpWebcam/*`.
- Time from plugging in to the DHCP address and to the first HTTP response.
- Whether the camera's UDP source port is fixed.
- Whether RTSP (`protocol=RTSP`) works and what its latency is.
- Whether the camera goes to sleep without keep-alive calls.
- The behavior from #74 (turning the camera off with the cable plugged in).
