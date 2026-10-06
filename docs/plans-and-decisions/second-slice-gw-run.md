# Second slice: `gw run` and the user service

- **Status:** In progress. Verified on the camera 2026-10-05: latency in Zoom 0.18 s (was 1.1 s, §4), Zoom sees the device and the placeholder without a restart, unplugging and replugging the cable work with the watchdog (§3.1). The service under systemd and the README are not done yet.
- **Date:** 2026-10-05
- **Owner:** Darko
- **Related:** ADR 0003 (gw owns the loopback device and runs as a user service); `first-slice-gw-start.md` §7.2 (Zoom and latency), §8; ADR 0002 (Open GoPro HTTP API for camera control)

## 1. Scope

- `gw` keeps `/dev/videoN` open and writes the frames itself; ffmpeg sends decoded frames through a pipe (ADR 0003).
- A placeholder while there is no camera, with the state on the second line.
- `gw run`: runs all the time, waits for the camera, starts a session, brings the placeholder back within about 0.5 s after the cable is unplugged, and after a failed session with the camera plugged in waits 5 s and tries again.
- `contrib/systemd/gw.service` (since ADR 0005 `packaging/systemd/gpwebcam.service`): systemd user service for `gw run`.
- The cause of the latency of about 0.8 s on the path through `gw` (§4).

Out of scope: notifications, recording, multiple cameras, installing the `modprobe.d` configuration.

## 2. Packages

| Package | Change |
|---|---|
| `internal/v4l2` | `OpenOutput`: QUERYCAP check, `VIDIOC_S_FMT` YU12, `WriteFrame` |
| `internal/feed` | new: repeats the placeholder every 100 ms while there are no live frames |
| `internal/placeholder` | new: ffmpeg `drawtext` draws one YU12 frame; the allowed characters in the text are letters, digits, space, `-`, `,` and `.`, so there is no escaping |
| `internal/stream` | output `-vf scale=W:H,format=yuv420p -f rawvideo -flush_packets 1 pipe:1`; `Run` reads whole frames and passes them to a function |
| `internal/camera` | `Resolution.Size()` |
| `cmd/gw` | `run` and `start` share `serve.go`; the session stops when the interface disappears |

## 3. Test plan

Automated, without the camera:

- `v4l2`: size and layout of `v4l2_format` (208 bytes, union at offset 8) and the `VIDIOC_S_FMT` number, per the kernel headers.
- `feed`: the placeholder is written at once and repeated, is not written while live frames flow, and is written again after `Idle`.
- `placeholder`: the text is drawn (bright pixels on a dark background), disallowed characters are rejected.
- `stream`: frames of the exact size reach the function, the end of the stream gives the message "no video from the camera", a write error stops ffmpeg.

Manual, with the camera:

1. `gw run` without the camera: Zoom or `ffplay -f v4l2 /dev/video42` shows "Camera not connected".
2. Plug in the camera: the picture switches to the camera; Zoom, started before that, still sees the device.
3. Unplug the cable: "Camera not connected" comes back within about 0.5 s.
4. Service: `systemd-run --user` with the same settings as `gw.service`, to check the restrictions (`ProtectSystem=strict`, `PrivateTmp` and the others), then install and `systemctl --user enable --now gw.service`.

### 3.1 Results on the camera, 2026-10-05

| Test | Result |
|---|---|
| `gw run` without the camera | "Video Capture" at once; placeholder "Camera not connected" |
| plugging in | "Connecting to camera", then the stream within about 2.5 s |
| Zoom started after `gw run` | sees the GoPro camera; latency 0.18 s (§4) |
| restarting `gw` while Zoom runs | Zoom keeps the device, the picture comes back within about 3 s |
| unplugging the cable while Zoom runs | placeholder in Zoom within at most 1.6 s (screenshot at about 1.2 s) |
| replugging the cable | two times out of two: the camera reports status 2 after START, but no video arrives. The first time (without the watchdog) ffmpeg gave up after 18 s, and a new attempt 5 s later succeeded. The second time (6 s watchdog) "No video from camera, retrying" after 7 s, a new attempt 2 s later succeeds; video about 15 s after replugging the cable |
| a single decoding error | about once every 5 minutes ("corrupt decoded frame"); the picture recovers on its own |

Noticed along the way:

- After plugging in, the kernel first names the interface `eth0`, and udev renames it after about 0.5 s to a stable name based on the USB port (for example `enp0s20f0u1`). `gw` sometimes catches `eth0`, the session stops as "unplugged" and starts again at once with the new name. It works, but the log looks like a false unplug.
- The camera status before START after replugging the cable was "idle" both times, and "off" after a failed attempt. The first plug-in that day also gave "idle", but START succeeded then. For "idle" after a new USB connection the GoPro FAQ suggests start then stop; that could shorten the recovery, but it is not verified.
- ffplay cannot open `/dev/video42` while `mpv` reads it ("Device or resource busy"). It remains to check whether Zoom and a browser can open it at the same time.

## 4. Latency

Solved 2026-10-05. The camera straight into ffplay gives 0.15 to 0.25 s, and the path through `gw` and `/dev/video42` gave about 1.1 s (`first-slice-gw-start.md` §7.2). Measurements with the same method (GoPro pointed at a clock with milliseconds, screenshot):

| What | Result |
|---|---|
| `mpv --profile=low-latency --untimed` as the reader instead of ffplay | 1.15 s: the reader is not the cause |
| in `gw`'s ffmpeg, from packet to frame after the filter (`-debug_ts`, `showinfo`, `-loglevel +datetime`) | median 5 ms, at most 15 ms; packets are read in real time |
| v4l2loopback: ffmpeg writes a test picture, a second ffmpeg reads, frames matched by checksum | 35 to 40 ms |
| port opened 2 s after START, instead of at once | 0.15 to 0.18 s: the order has no effect |
| ffmpeg as `gw` runs it, into `/dev/video42`, unchanged | 1.02 s |
| same, without `timeout` on UDP | 1.18 s |
| same, without `scale` | 1.13 s |
| **same, with `-fps_mode passthrough`** | **0.18 s** |
| `gw run` with `-fps_mode passthrough`, reader `mpv` | 0.18 to 0.20 s |
| same, Zoom (self view in a meeting) | 0.18 s, nested screenshot 0.20 s |

Cause: for `rawvideo` and `v4l2` output ffmpeg uses a constant frame rate (CFR) by default, and with the camera's stream that holds frames back for about 0.85 s. `gw` now passes `-fps_mode passthrough`, so every decoded frame goes on at once. The local test stream from §5.1 of the first slice did not show this, because it measured `-f null` output with perfect timestamps.

## 5. Where we are and what is next

- 2026-10-05: wrote `feed`, `placeholder`, `v4l2.OpenOutput`, `gw run` and `gw.service`; tests pass. Measured the latency per setting and without `gw`.
- 2026-10-05: latency solved with `-fps_mode passthrough` (§4). Added a watchdog: if no frame arrives within 6 s after START, the session stops and is retried after 2 s. The placeholder shows as soon as the interface disappears; it does not wait for ffmpeg. The camera status before START is written to the log. Manual tests with Zoom passed (§3.1).
- Next: the service under systemd (§3, step 4); README for `gw run` and the service; optionally start then stop when the camera is "idle" after plugging in, and a stable interface name before the session (§3.1).
