# 0003 — gw owns the loopback device and runs as a user service

- **Status:** Accepted 2026-10-05. `gw` is the only writer to `/dev/video42`, keeps it open the whole time it runs, and writes a placeholder image while there is no camera; `gw run` runs as a systemd user service, without a udev rule. Code written, partly tried on the camera (`second-slice-gw-run.md`).
- **Date:** 2026-10-05
- **Supersedes:** — / **Superseded by:** —
- **Owner:** Darko
- **Related:** `first-slice-gw-start.md` §7.2 (Zoom does not see the camera); `second-slice-gw-run.md`; `gopro-fork-review-and-handover.md` §5 (the original flow through udev and a system service), §10.1 (notifications)

## Context

Zoom does not see the GoPro if it is started before `gw`. The cause was measured on 2026-10-05 (`first-slice-gw-start.md` §7.2):

- The module is loaded with `exclusive_caps=1`, because Chrome and similar applications otherwise reject the device. With that option, `/dev/video42` reports "Video Capture" only while some writer holds it open, and "Video Output" otherwise.
- When a writer opens or closes the device, the kernel sends no udev event. An application that is already running therefore cannot find out that the camera appeared.

In the first slice the writer was ffmpeg, started only when the camera is plugged in, so the device was a "camera" only while the stream ran.

The original plan (handover note §5) was a udev rule that starts a system `gw@<interface>.service` on plug-in. Desktop notifications (§10.1) need the session D-Bus, which only the user's service has.

## Decision

1. `gw` opens `/dev/videoN` once, at startup, and keeps it open until it exits. It sets the format itself (`VIDIOC_S_FMT`, YU12, size from `-res`) and writes the frames itself.
2. ffmpeg no longer writes to the device. It decodes the stream, scales it to the device size, and sends raw `yuv420p` frames to `gw` through a pipe (`-f rawvideo -flush_packets 1 pipe:1`).
3. While the camera sends no video, `gw` repeats a placeholder image 10 times per second: "gw - GoPro webcam for Linux" with the state below it ("Camera not connected", "Connecting to camera", "No video from camera, retrying"). ffmpeg draws the image once (`drawtext`), and if that fails, a solid dark image is used.
4. A new command, `gw run`, runs all the time: every 0.5 s it looks for the GoPro interface in sysfs, starts a session when it finds it, and when the interface disappears, it brings back the placeholder image within about 0.5 s. `gw start` remains for a single session.
5. `gw run` runs as a systemd **user** service (`contrib/systemd/gw.service`, since ADR 0005 `packaging/systemd/gpwebcam.service`), as the logged-in user. Access to the device comes from the uaccess ACL, root is not needed, and a udev rule is not needed, because the service waits for the camera itself.

## Consequences

**Positive**

- The device is "Video Capture" from the moment the service starts, so Zoom and other applications see it regardless of the order in which they start.
- An application gets an image even when the camera is not plugged in, and knows right away what state it is in.
- After the cable is pulled, the placeholder image appears within about 0.5 s, instead of the last frame staying until ffmpeg's 5 s timeout expires.
- `gw` counts and writes the frames itself, so a watchdog for packets that do not arrive gets a natural place.
- The user service has the session D-Bus for notifications and needs neither root nor udev rules.

**Negative**

- `gw` contains two ioctls with structures translated from `videodev2.h`. The layout is verified by a test against the kernel headers on x86_64.
- Every frame goes through the pipe and one more copy (about 93 MB/s for 1080p30). In a measurement on 2026-10-05, the old and the new path gave the same latency (1.13 s and 1.12 s).
- The placeholder image uses a little CPU even while nobody is watching: about 31 MB/s of copying into v4l2loopback.
- While `gw run` runs, `/dev/video42` is taken and no other program can write to it.

**Risks**

- If the module is loaded without `exclusive_caps=1` or with a different device number, `gw` cannot open the device, and the service restarts every 10 s until the module is configured.
- Changing `-res` changes the device format; an application that opened the device with the old format has to open it again.

## Alternatives considered

- **`exclusive_caps=0`:** the device always reports both capture and output. Chrome and WebRTC applications reject such a device, and that is the reason `exclusive_caps=1` exists.
- **Creating the device only when the camera is plugged in (`v4l2loopback-ctl add`):** a new device produces a udev event, but the control device needs root, and `gw` must not have root at runtime.
- **v4l2loopback's built-in timeout image:** the device still has to have a writer from the start, and it is configured with an external tool.
- **Two ffmpeg processes, one for the placeholder image and one for the camera:** the device is closed between them, so capture is lost for a moment, and a reading application may drop it.
- **udev rule and a system service per interface:** no session D-Bus, and it does not solve the Zoom problem.

## Out of scope

- Desktop notifications (handover note §10.1).
- Several cameras at the same time.
