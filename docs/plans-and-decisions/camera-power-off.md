# Power off the camera from the menu

- **Status:** Accepted 2026-10-27: the old gpControl sleep command over USB, as **Power off camera** in the tray menu; written and tested live the same day (§5).
- **Date:** 2026-10-27
- **Owner:** Darko
- **Related:** `camera-on-demand.md` (modes, keep-alive between uses); `tray-and-recording.md` (the tray); `open-gopro-webcam-api.md` (the HTTP API); handover note §2 (camera, battery)

## 1. Problem

Darko, 2026-10-27: when he does not need the camera for a longer time, after the last meeting of the day or for the night, he turns it off and on with its button, because otherwise it stays warm for nothing. He asks whether gpwebcam can turn it off from the tray menu, and takes it that turning it on from the computer is hard. A plain power off, with the button to turn it on, is enough; it need not be part of the on-demand flow.

## 2. Verified

All on the test camera, HERO13 Black with firmware 02.10, without its battery (status 1 = 0), on 2026-10-27.

| Fact | Source |
|---|---|
| The Open GoPro HTTP API has no sleep or power off command. Its only power command is a reboot, through the older `gp/gpControl` path | the SDK's `http_commands.py`, read 2026-10-27 |
| The BLE API has Sleep (0x05) and Power Down (0x04); a sleeping camera keeps advertising, and a BLE connection wakes it | the SDK's `constants.py`; Open GoPro docs; 2026-10-27 |
| `GET /gp/gpControl/command/system/sleep` on port 8080 over USB, with the webcam idle: HTTP 200 and `{}`; 1.7 s later the camera leaves the USB bus (kernel: USB disconnect) | `curl`, 16:49 |
| After that, the button turns the camera on as usual. The interface comes back, the service finds it without a replug, and `/gopro/webcam/status` answers idle | 17:01 |
| With the service stopped, so without keep-alive, the camera on USB power stayed on for 7.5 min, past its auto power down of 5 min (setting 59 = 4), and still answered afterwards | 17:02 to 17:10 |
| While the camera is off, its USB device is gone (no vendor `2672` in sysfs), so nothing over USB can reach it | 17:00 |

## 3. Options

| Option | Turns off | Turns on | Cost |
|---|---|---|---|
| A. The old sleep command over USB | yes, measured | no; the button | one HTTP call. The command is undocumented, so a firmware update may drop it; the service would then say so |
| B. Stop keep-alive and let auto power down act | no, measured | no | ruled out by §2 |
| C. BLE Sleep, and wake over BLE | yes | yes | BlueZ over D-Bus (godbus is already a dependency and pure Go), pairing the camera once, keep-alive over BLE as well, a reported firmware bug on wake (OpenGoPro #476); a plan of its own |

Decided 2026-10-27 (Darko): A now, as a plain power off. C is a separate question.

## 4. Design

- The tray menu gets **Power off camera**, in the last block next to Restart and Quit, enabled while a camera is connected. The label avoids "off" on its own, which the Camera submenu uses for the mode.
- `camera.Sleep` sends the old command. HTTP 404 becomes `camera.ErrNoSleep`, for a firmware without it.
- The click sets `offWanted`, drops a recording (the session saves the file), ends the session with `errPowerOff` and wakes the idle wait. The main loop, which owns the camera, then does the work in `turnOff`. The session's end has sent STOP and EXIT, so the webcam is stopped first; `sleepCamera` reads the webcam status anyway and stops it again if it still streams, since sleep was only ever tried with the webcam stopped. The placeholder shows "Turning off <model>…", the service sends sleep and waits up to 10 s for the interface to go. Then the placeholder says "<model> is off. Turn it on with its button.", the tray shows no dot and the same line, and a notification says so. If the camera stays, the log and a notification say why, and the service goes on as before.
- While the power off is pending, Record and Power off camera are disabled and `gpwebcam record start` is refused, so nothing starts the camera by itself when it comes back. A camera that leaves the bus on its own (cable, button) clears the pending power off, as it clears a recording (`forget`).
- The button turns the camera on; the service finds it as after a plug-in and continues in the current mode.
- No command line counterpart yet. `gpwebcam config` and `gpwebcam record` cover the other menu items, so `gpwebcam camera off` through the control socket is the natural next step for desktops without a tray.

## 5. Where we are and what is next

- 2026-10-27: Darko's question, the measurements in §2, the decision in §3.
- 2026-10-27: written in the worktree `camera-power-off`; tests pass with `-race` and on Go 1.22.
- 2026-10-27, live test with a build from the worktree instead of the service, the click sent over D-Bus (`com.canonical.dbusmenu Event` on the item): while an ffmpeg reader used the device and the camera streamed, click at 17:28:11.57, webcam stopped (STOP, EXIT) at :12.35, sleep sent, interface gone at :13.19; the reader kept running on the placeholder, and the "is off" notification arrived.
- 2026-10-27, review (`reviewer` agent) found two races the live test could not reach, both fixed with tests: a click in the moment between the camera leaving the bus and the service noticing left the flag set, so the camera would have been turned off right after the next plug-in; and Record during the turn off survived it and would have started the camera on its return. Also from the review: a click right after a session failed no longer waits for the retry, `sleepCamera` stops a webcam that still streams, and a camera that leaves the bus before answering counts as off.
- 2026-10-27, live test of the build with the fixes: the button turned the camera on and the build found it 1 s later, idle in mode demand; the click from the idle state at 17:39:16.70, sleep sent at once (no webcam to stop), interface gone at :17.28; the tooltip read "GoPro HERO13 Black is off. Turn it on with its button.", Record and Power off camera were disabled, `gpwebcam record` answered "Not recording."
- Next: commit; `gpwebcam camera off` for desktops without a tray; BLE (§3 C) as its own plan if turning the camera on from the computer is wanted.
