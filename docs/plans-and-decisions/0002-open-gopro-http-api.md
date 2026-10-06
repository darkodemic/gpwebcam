# 0002 — Open GoPro HTTP API for camera control

- **Status:** Accepted 2026-09-29, confirmed on the camera 2026-10-05. `gw` controls the camera through `/gopro/webcam/*` on port 8080. This works on HERO13 Black with firmware 02.10 (`first-slice-gw-start.md` §7.1), so the old `/gp/gpWebcam/*` is not needed.
- **Date:** 2026-09-29
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `open-gopro-webcam-api.md` (findings from the specification); `upstream-issues-review.md` §1.1; `first-slice-gw-start.md`; ADR 0001 (Go as the implementation language)

## Context

The camera is switched into webcam mode with HTTP calls. There are two sets of endpoints:

- **Open GoPro:** `/gopro/webcam/{start,stop,exit,status,...}` on port 8080. It is documented in Open GoPro HTTP API 2.0, and the current version of the specification lists HERO13 Black as a supported model. `res` is a code (7 = 720p, 12 = 1080p), and `fov` is sent in the same call as start. There are also `status` with state and error codes, `wired_usb` and `keep_alive` (`open-gopro-webcam-api.md` §2 to §5).
- **Old:** `/gp/gpWebcam/{START,SETTINGS,STOP,EXIT}` on port 80, used by the old tool. `res` is the number of lines (1080, 720), and `fov` goes in a separate `SETTINGS` call. It is not in any version of the specification. According to upstream issues it works on HERO12 Black and HERO13 Black, but for HERO13 the firmware is not stated (`upstream-issues-review.md` §1.1).

The first target camera is HERO13 Black with firmware 02.10. At the time of the decision, neither of the two sets had been tried on it.

## Decision

1. `gw` uses Open GoPro HTTP API 2.0 on port 8080.
2. The start sequence follows the state machine from the specification:
   - `wired_usb?p=0`, repeated until the camera responds;
   - `status`, then `stop` if the camera was left in a preview state;
   - `start?res=..&fov=..&port=..&protocol=TS`, parameters in that order;
   - `status` until it shows High Power Preview or Low Power Preview.
3. While the stream runs, `keep_alive` is sent every 3 s.
4. At the end, `stop` and `exit` are sent, and the second is sent even when the first fails.
5. Every response is parsed. An `error` other than 0 is an error both in the response to a command and in the response to `status`.
6. The old API is not implemented in advance. If Open GoPro does not work on HERO13 02.10 and the old one does, it is added as a second dialect behind an option, and this decision gets an amendment.

## Consequences

**Positive**

- The API is documented, with state and error codes, so `gw` can say exactly what failed, instead of treating every non-empty body as success (upstream #28).
- FOV goes in the same call as start, with no second call and no intermediate state.
- `status` gives a check that the stream really started.

**Negative**

- The old tool worked with the old API, and in the upstream issues nobody had tried Open GoPro over USB on HERO13.
- HERO9 to HERO12 are in older builds of the specification but not in the current one; supporting them is not a goal, but it may depend on the firmware.

**Risks**

- HERO13 v01.10.00 returned HTTP 500 on webcam start (Open GoPro FAQ). GoPro says it is fixed, but this is not verified on 02.10. Mitigated by rule 6 and a clear message with the HTTP code.
- The resolution codes for HERO13 are not in the specification's table; 7 and 12 come from the FAQ and the SDK.

## Alternatives considered

- **Only the old API:** confirmed by users on HERO13, but undocumented, with no status and no guarantee that it stays in new firmware.
- **Both APIs right away, with automatic switching:** more code and tests before we know whether the second one is needed at all.
- **RTSP (`protocol=RTSP`):** the camera is the server on port 554, so the host does not have to receive incoming UDP, which avoids the firewall problem (upstream #2, #7, #42). Latency and stability are unknown. Left for measurement on the camera.

## Out of scope

- Camera settings outside the webcam calls (Hypersmooth, presets).
- Wi-Fi and COHN.
