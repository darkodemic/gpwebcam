# Camera on demand and restart from the menu

- **Status:** Accepted 2026-10-06: `demand` by default, grace period 15 s, done before slice 2 of `tray-and-recording.md` (§5).
- **Date:** 2026-10-06
- **Owner:** Darko
- **Related:** `tray-and-recording.md` (tray, settings, recording); ADR 0003 (gpwebcam owns the device as a user service); handover note §2 (camera, battery)

## 1. Problem

Darko, 2026-10-06: as soon as the camera is plugged in, the service switches it to webcam mode and it streams for as long as the service is running, so it heats up for no reason. He wants the camera to run only when needed, manually or, better still, when an application (Zoom) asks for it, like a regular webcam. He also asks whether the menu needs an item to restart the service.

## 2. Verified

| Fact | Source |
|---|---|
| v4l2loopback has the event `V4L2_EVENT_PRI_CLIENT_USAGE` (`V4L2_EVENT_PRIVATE_START + 0x08E00000 + 1`) with the body `{ __u32 count }`: 1 while some reader has video running, 0 when none has | `v4l2loopback.c` 0.15.4, lines 815-824 and 2147-2160, 2026-10-06 |
| The event arrives on a reader's `STREAMON` and `STREAMOFF` and when the device is closed, because `close` does `REQBUFS(0)`, and that does `STREAMOFF`; this also holds when the reader is killed with `kill -9`. The current state arrives right at subscription | same, lines 2065-2130, 1703-1712, 2163-2170 |
| Merely opening the device, for example when an application lists cameras, sends no event | same: the event is sent only from `streamon` and `streamoff` |
| Since which version v4l2loopback has this event, and which version Debian 13 and Ubuntu 24.04 have, is not verified | to be verified |
| User systemd on the session bus offers `org.freedesktop.systemd1.Manager.RestartUnit` and `StopUnit`; the service has `INVOCATION_ID`, so it knows it runs under systemd | `busctl --user`, 2026-10-06 |
| From click to picture the camera starts in 5.4 to 5.8 s, and once it took about 20 s | `tray-and-recording.md` §9 |

## 3. Proposal

### 3.1 Modes

A new setting `camera`, in the menu and in `gpwebcam config`:

| Mode | Behavior |
|---|---|
| `demand` (on demand) | the camera starts when an application starts video, and stops when no application has used it for longer than the grace period (§3.2) |
| `always` (always on) | as now: the camera starts as soon as it is plugged in |
| `off` (pause) | the camera does not start; the placeholder says it is paused, the icon is faded |

Decided 2026-10-06 (Darko): `demand` by default. If the driver does not have the event (the subscription returns `EINVAL`), the service works as `always`, writes that to the log, and `doctor` warns.

While the camera is not running, the placeholder still goes to the device, so applications see the camera. When an application starts video, for about the first 5 s it sees "Starting GoPro HERO13 Black…", and then the camera's picture.

### 3.2 Grace period before turning off

The camera does not stop right away when the last application stops video, but after a grace period. The proposal was 30 s; Darko chose 15 s on 2026-10-06. Applications sometimes stop video and then start it again right away, for example when moving from the preview into the meeting (not verified for Zoom). Without a grace period, every such switch would cost 5 s of black picture.

### 3.3 The camera between uses

The service sends STOP and EXIT, and the USB network stays up. It needs to be measured whether the HERO13 turns itself off when there is no `keep_alive`. If it does, the service cannot wake it over USB, so `keep_alive` must then be sent between uses too: the camera is on, but the sensor and the encoder are not running.

### 3.4 Resolution without a restart

When no application has video running, the service can reopen the device at the new size. The note "Applies when gpwebcam restarts" then mostly goes away. The exception is an application that has set the format but has not started video: it still holds the format, so the fix from `8fb1ca7` applies and the old size stays.

### 3.5 Restart from the menu

The item "Restart gpwebcam" calls `RestartUnit("gpwebcam.service", "replace")` through `godbus`, which is already a dependency. It is shown only when the service runs under systemd. With §3.4 it serves mostly for recovery, when something gets stuck.

### 3.6 Menu

- Camera: On demand, Always on, Off
- Restart gpwebcam

Recording (`tray-and-recording.md` §5) turns the camera on regardless of the mode, for as long as it lasts.

## 4. Icon

Darko, 2026-10-06: white while everything works, orange when there is an error, faded white when the camera is not running. Done: white while video flows; orange for problems; white at 45 % for all other states (no camera, starting, and with this proposal also pause and waiting for an application). A thin dark outline keeps the white icon visible on a light panel.

## 5. Decisions

Darko, 2026-10-06:

1. The default mode is `demand`.
2. The grace period before turning off is 15 s.
3. Camera on demand comes before slice 2 of `tray-and-recording.md` (gpwebcam receives the UDP stream itself): it does not depend on it, and it solves the heating.

## 6. Where we are and what is next

- 2026-10-06: Darko's question, the checks in §2 and this proposal. Icon changed (§4). Darko's decisions in §5.
- 2026-10-06, test camera: setting 59 (auto power down) = 4, which per the Open GoPro specification is 5 minutes. So the service sends `keep_alive` between uses too (§3.3); whether the camera would really turn off on USB without it was not measured.
- 2026-10-06, written (tests pass with `-race`, and on Go 1.22), without §3.4:
  - `v4l2.WatchUsage`: a second file descriptor for the device, only for events, a subscription with `V4L2_EVENT_SUB_FL_SEND_INITIAL` (without that flag the initial state does not arrive), `select` on the exceptional condition, and `VIDIOC_DQEVENT`. Struct sizes and ioctl numbers verified with a C program against `linux/videodev2.h` (136 and 32 bytes, `0x80885659`, `0x4020565a`). Live on `/dev/video42`: the initial "not in use" right away, "in use" 1.05 s after an ffmpeg reader started, "not in use" when it finished after 2 s.
  - The setting and flag `camera` (`demand`, `always`, `off`); the Camera menu and "Restart gpwebcam" (`GetUnitByPID` then `Unit.Restart` through `godbus`, only with `INVOCATION_ID`).
  - Service: while the camera does not need to run, the placeholder "<model> ready" or "Camera off…", with `keep_alive`; the session ends 15 s after the last application (`errIdle`), and on a mode change right away. The "connected" notification now arrives on plug-in, with a sentence about what comes next, instead of at every start of video.
  - `doctor` checks whether the module reports usage.
- 2026-10-06, live test (a build from the working tree instead of the service, an ffmpeg reader instead of Zoom, 720p):

  | Step | Result |
  |---|---|
  | the service starts, nobody uses the device | the camera stays in status 0 (off) |
  | the reader starts video | webcam start after 2.4 s, video after 4.0 s; for the first 4 s the reader gets the placeholder (YAVG about 45), then the camera's frames (YAVG 8 to 10) |
  | the reader finishes | the camera stops after 16.3 s (15 s grace period plus a check every 0.5 s) |
  | `camera always` | video after 3.5 s |
  | `camera off` | the camera stops right away |
  | `camera demand` | the camera stays off |

- Fixes after the test: an unknown key in `settings.json` no longer fails the whole file, but is reported as a warning (`UnknownKeysError`), and `Save` keeps it; otherwise an older build would discard a file with the key `camera` and go back to the defaults (that is how build `b68f9e7`, installed 2026-10-06, would behave). Mode off has its own stop reason (`errOff`) and log message; "found camera" is written only on plug-in and on an interface rename.
- Along the way, 2026-10-06: the local checks "on Go 1.22" through `mise exec go@1.22` in this session were not on 1.22, because the shell exported `GOROOT` for 1.27.1, so Go switched to 1.27.1. The real check: `mise exec go@1.22 -- env -u GOROOT -u GOBIN GOTOOLCHAIN=local GOWORK=off go test ./...`. CI uses real Go 1.22 and was green.
- 2026-10-06: commit `2a23480`, CI green. Darko installed the package built from `2a23480` (worktree without `go.work`, with checkboxes) and tried it with Zoom, the Camera menu and the restart from the menu: "everything works nicely".
- 2026-10-06, §3.4 written (tests pass with `-race`, and on real Go 1.22, against `tray.go` and `go.mod` from the commit):
  - `feed.Swap` replaces the device under the feed's lock, so no frame is written between closing the old one and opening the new one. The old output must be closed first, because v4l2loopback has one output format token.
  - `maybeResize` runs only between sessions: while waiting for the camera, while waiting for an application, and before a new session. The conditions are that the configured resolution differs from the one in use, that the module reports usage, and that no application has video running. If the device keeps the old size (an application set the format without video), that resolution is not tried again until the usage changes or until the resolution is chosen again. If the device does not open at any size, the write to the closed output fails and the service exits, so systemd starts it again.
  - In mode always the session does not end on its own, so `watchDemand` ends it when a change is pending and there is no application.
  - The note in the menu and the texts in the README, the man page and `gpwebcam config -h`: the new resolution applies as soon as no application uses the camera.
- 2026-10-06, live test of §3.4 (a build with checkboxes, without the fork; an ffmpeg reader instead of an application): `res 1080` while nobody uses the device, and the device switches to 1920x1080 in 0.1 s; the reader turns video on, the camera starts in 1080p, video after 4.2 s; `res 720` while the reader runs does not change the device; the reader finishes, the camera stops after 17 s (grace period), and the device switches to 1280x720 right away.
- Next: commit, then slice 2 of `tray-and-recording.md`.
