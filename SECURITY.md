# Security

## Reporting a problem

Report security problems privately, through [Security > Report a vulnerability](https://github.com/darkodemic/gpwebcam/security/advisories/new) on GitHub, not in a public issue. One person maintains gpwebcam in spare time; expect an answer within a week.

## Supported versions

Security fixes go into the latest release only.

## What gpwebcam exposes

- It receives the camera's video over UDP, port 8554 by default, only on the computer's address on the GoPro's USB link and only from the camera's address. ffmpeg decodes it from a pipe and opens no socket.
- It controls the camera through the camera's HTTP API on that link, with a timeout on every request.
- It runs as the logged-in user, without root, in the systemd sandbox of `gpwebcam.service` (`ProtectSystem=strict`, `ProtectHome=read-only` and more). It writes only to the video device, `~/.config/gpwebcam`, `~/Videos` and its runtime folder.
- `gpwebcam record` talks to the service through `$XDG_RUNTIME_DIR/gpwebcam/control.sock`, which only the user can open.
