# gpwebcam - GoPro webcam support for linux

`gpwebcam` turns a GoPro connected over USB into a regular Linux webcam. Browsers, Zoom, OBS and every other V4L2 application see it as a camera named **GoPro**.

It runs as a systemd user service without root. The service waits for the camera, starts its webcam mode when you plug it in, and shows a "Camera not connected" picture while it is unplugged, so applications keep listing the camera. In our measurements the delay from scene to the Zoom preview is about 0.2 seconds.

**Status:** early, no release yet. It is tested with a HERO13 Black, firmware 02.10 (`H24.01.02.10.00`), on Arch Linux, at 1080p and 30 fps. The `.deb` and `.rpm` packages install, run and uninstall cleanly on Debian 13, Ubuntu 24.04 and Fedora 44, but have not been tried there with a camera yet. Dedicated recording mode is not implemented yet.

Want to build or change gpwebcam? See [CONTRIBUTING.md](CONTRIBUTING.md).

## What you need

- Linux with systemd.
- The **v4l2loopback** kernel module, which provides the virtual camera device.
- **ffmpeg** with an H.264 decoder.
- A GoPro with webcam support over USB. Only the HERO13 Black is tested so far.
- A USB cable that carries data, not only power.

## Install

### 1. Install the package

Packages will be on the [releases page](https://github.com/darkodemic/gpwebcam/releases) from the first release on. Until then, build them as described in [CONTRIBUTING.md](CONTRIBUTING.md), then install them as below.

**Arch Linux**

```sh
sudo pacman -S --needed v4l2loopback-dkms linux-headers
sudo pacman -U gpwebcam-<version>-x86_64.pkg.tar.zst
```

- `v4l2loopback-dkms` builds the module for your kernel; `linux-headers` must match the kernel you run (use `linux-lts-headers` for `linux-lts`).

**Debian and Ubuntu** (not yet tried with a camera)

```sh
sudo apt install ./gpwebcam_<version>_amd64.deb
```

- apt also installs ffmpeg, and v4l2loopback-dkms as a recommended package.
- Ubuntu 24.04 already ships the module with its kernel.

**Fedora** (not yet tried with a camera)

v4l2loopback is not in Fedora, and Fedora's own ffmpeg cannot decode H.264 without extra steps. Both come from [RPM Fusion](https://rpmfusion.org/Configuration):

```sh
sudo dnf install akmod-v4l2loopback v4l2loopback
sudo dnf swap ffmpeg-free ffmpeg --allowerasing
sudo dnf install ./gpwebcam-<version>.x86_64.rpm
```

### 2. Load the module

The package configures v4l2loopback in `/usr/lib/modprobe.d/99-gpwebcam.conf` and loads it at every boot. That configuration creates two devices:

- `/dev/video42`, labelled **GoPro**, for gpwebcam;
- a second device with a free number, labelled **OBS Virtual Camera**, so OBS keeps working.

Reboot once after the installation. If the module was not loaded before, `sudo modprobe v4l2loopback` works as well.

To check, run this; it should print `GoPro` and `OBS Virtual Camera` among the names:

```sh
cat /sys/class/video4linux/*/name
```

### 3. Start the service

As your own user, not root:

```sh
systemctl --user enable --now gpwebcam.service
```

- `enable` starts it at every login, and `--now` starts it right away.
- From then on, the **GoPro** camera is always available to applications. Without a camera it shows the "Camera not connected" picture.

### 4. Prepare the camera

1. On the camera, set **Preferences → Connections → USB Connection** to **GoPro Connect**, not MTP.
2. Connect the camera with the USB cable and turn it on.

A microSD card is not needed.

### 5. Check the setup

```sh
gpwebcam doctor
```

It checks ffmpeg, the module and the device, the service, the settings, the camera's connection, the firewall and the system tray, and says what to fix. It only reads the camera's state, so it is safe to run while the service streams.

## Use

- In your video application, choose the camera named **GoPro**.
- By default the GoPro works like any webcam: it starts streaming when an application turns its video on, and stops 15 seconds after the last application turned it off, so it does not heat up while nobody uses it. Starting takes about 5 seconds; the application shows "Starting GoPro HERO13 Black" meanwhile. To stream whenever the camera is connected, or never, change the [camera mode](#settings).
- When you unplug it, the "Camera not connected" picture comes back. Plug it in again at any time; the application does not need a restart.
- While there is no video, the picture says why, and names the camera model: it is not connected, was found and waits for its network, is starting, does not answer, or sends no video. Dots after the text keep moving while gpwebcam waits for something, so you can tell it has not frozen.
- A desktop notification tells you when the camera connects, disconnects or has a problem, for example "GoPro HERO13 Black connected. It starts when an application uses the camera." It needs `notify-send` (package `libnotify`, or `libnotify-bin` on Debian and Ubuntu); turn it off in the tray menu or with `gpwebcam config notify off`.
- Decoding runs on the GPU through VAAPI (AMD and Intel graphics) when ffmpeg can open a VAAPI device, and on the CPU otherwise. On an AMD GPU this took a third less CPU time with no noticeable added delay. If GPU decoding gives no picture twice in a row, gpwebcam switches to the CPU by itself. On a laptop where it would wake the discrete GPU, turn off **Hardware decoding** in the tray menu or run `gpwebcam config hwdec none`.

### Tray icon

While the service runs, a camera icon in the system tray shows the state: white while video flows, orange when the camera has a problem, and faded while there is no video for another reason, such as no camera connected or the camera starting. Its menu has:

- the camera's state, for example "GoPro HERO13 Black: 1080p, linear";
- **Camera**: On demand (the default; streams while an application uses the camera), Always on (streams while it is connected) or Off (never streams; the picture says so).
- **Field of view**: Wide, Narrow, SuperView or Linear. The camera restarts with the new one, which takes about 4 seconds; applications keep the camera open meanwhile.
- **Resolution**: 1080p or 720p. Applications keep the frame size they started the video with, so a new resolution applies once no application uses the camera; turn the video off and on again in the application.
- **Hardware decoding** and **Notifications**, on or off.
- **Restart gpwebcam**, when it runs as the systemd service.
- **Hide icon**. Bring it back with `gpwebcam config tray on`.

The icon needs a desktop with a system tray that speaks StatusNotifierItem: KDE Plasma, Waybar, Quickshell and similar bars show it, and GNOME shows it only with the AppIndicator extension (Ubuntu has it on by default). Without a tray, use `gpwebcam config`.

### Settings

The tray menu and `gpwebcam config` change the same settings, saved in `~/.config/gpwebcam/settings.json`:

```sh
gpwebcam config            # show all settings
gpwebcam config fov wide   # change one
```

| Setting | Default | Values |
|---|---|---|
| `camera` | `demand` | `demand` (stream while an application uses the camera), `always` (while it is connected), `off` |
| `res` | `1080` | `1080`, `720` |
| `fov` | `linear` | `wide`, `narrow`, `superview`, `linear` |
| `hwdec` | `auto` | `auto`, `none` |
| `notify` | `on` | `on`, `off` |
| `tray` | `on` | `on`, `off` |

The running service applies a change within about 2 seconds; a new resolution applies once no application uses the camera. A flag of the same name given to `gpwebcam run` overrides the file (see [Options](#options)), and the tray menu then marks that item as set by a flag.

### Camera models

gpwebcam is tested with the **HERO13 Black**. Other models that support webcam mode over USB may work, since the Open GoPro API also lists HERO9 to HERO12 in earlier versions. With an untested model, gpwebcam tries anyway, logs a warning and shows a notification once; please [report](https://github.com/darkodemic/gpwebcam/issues) whether it works.

Applications always see the camera as **GoPro**: the name is fixed when the module is loaded, and the device stays the same while you swap cameras. The model appears on the placeholder, in notifications and in the log. To use another name, set it in your own module configuration and tell gpwebcam:

1. Copy `/usr/lib/modprobe.d/99-gpwebcam.conf` to `/etc/modprobe.d/99-gpwebcam.conf` and change `card_label="GoPro,OBS Virtual Camera"` to, for example, `card_label="GoPro HERO13 Black,OBS Virtual Camera"`.
2. Add `-device-label "GoPro HERO13 Black"` to the service's command (see [Change the service command](#change-the-service-command)).
3. Reboot.

Logs and control:

```sh
journalctl --user -u gpwebcam -f       # follow the log
systemctl --user restart gpwebcam      # restart, e.g. after changing options
systemctl --user disable --now gpwebcam  # stop it and do not start it at login
```

### Change the service command

The service runs `gpwebcam run` without options. Options that are not settings, such as `-device-label` or `-port`, go into its command through a drop-in:

```sh
systemctl --user edit gpwebcam.service
```

In the editor, write:

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/gpwebcam run -port 8555
```

The empty `ExecStart=` clears the packaged command before setting the new one. Save, then run `systemctl --user restart gpwebcam`.

### Options

`gpwebcam run` and `gpwebcam start` take the same options, except `-camera` and `-tray`, which only `run` has; `start` always streams. `-camera`, `-res`, `-fov`, `-hwdec`, `-notify` and `-tray` override the [settings](#settings) file; without them the file applies.

| Option | Default | Meaning |
|---|---|---|
| `-res` | `1080` | Resolution: `1080` or `720`. |
| `-fov` | `linear` | Field of view: `wide`, `narrow`, `superview` or `linear`. |
| `-device-label` | `GoPro` | Label of the v4l2loopback device to write to. |
| `-video-nr` | `-1` | Use `/dev/videoN` instead of finding the device by label. |
| `-iface` | the only GoPro interface | GoPro network interface; needed only with several cameras. |
| `-port` | `8554` | UDP port the camera streams to, from 1024 to 65535. |
| `-hwdec` | `auto` | Hardware decoding: `auto` uses VAAPI when it works, `none` always decodes on the CPU. |
| `-notify` | `true` | Desktop notifications; `-notify=false` turns them off. |
| `-tray` | `true` | Tray icon; `-tray=false` turns it off. |
| `-camera` | `demand` | When the camera streams: `demand`, `always` or `off`. |
| `-ffmpeg` | `ffmpeg` | ffmpeg executable. |
| `-dhcp-wait` | `30s` | How long to wait for the camera to give the computer an address. |
| `-connect-wait` | `20s` | How long to wait for the camera to answer. |
| `-http-timeout` | `5s` | Timeout of each request to the camera. |

Commands:

- `gpwebcam run` keeps running and handles plugging and unplugging; it is what the service runs.
- `gpwebcam start` streams one camera session and exits when it ends.
- `gpwebcam config [<setting> [<value>]]` shows or changes the [settings](#settings).
- `gpwebcam doctor` checks the setup and says what to fix; it takes `-ffmpeg`, `-iface`, `-video-nr`, `-device-label`, `-port` and `-http-timeout`.
- `gpwebcam list` lists connected GoPro network interfaces.
- `gpwebcam version` prints the version.

Run only one gpwebcam at a time: stop the service before you start `gpwebcam run` or `gpwebcam start` by hand.

## Module configuration

`/usr/lib/modprobe.d/99-gpwebcam.conf` contains:

```
options v4l2loopback devices=2 video_nr=42,-1 card_label="GoPro,OBS Virtual Camera" exclusive_caps=1,1
```

- To change it, copy the file to `/etc/modprobe.d/99-gpwebcam.conf` and edit the copy; a file in `/etc` with the same name replaces the packaged one.
- To turn it off, link that name to `/dev/null`.
- Changes take effect at the next boot.

gpwebcam finds its device by the **GoPro** label, so a different device number works without other changes.

## Troubleshooting

Run `gpwebcam doctor` first; it finds most problems on its own. The log is in `journalctl --user -u gpwebcam -e`.

| Message or symptom | Likely cause and fix |
|---|---|
| `no video device named "GoPro"` | The module is not loaded, or it was loaded with other options (for example by OBS). Reboot, or check `cat /sys/class/video4linux/*/name`. |
| `VIDIOC_S_FMT ... device or resource busy` | Another program writes to the device, such as a second gpwebcam started by hand. |
| The log stays at `waiting for a camera` | The camera is off, its USB mode is MTP instead of GoPro Connect, or the cable carries only power. On some models, the Media Mod hides the connection. |
| `wait for IPv4 address on ...` | Nothing gives the GoPro connection an address. NetworkManager and systemd-networkd do it automatically; make sure the connection is not set to "ignore" or "disabled". |
| `host address 10.42.0.1/24 is not in a GoPro network` | NetworkManager set up the connection in "shared" mode. Fix it with `nmcli connection modify "<connection>" ipv4.method auto`, then `nmcli connection up "<connection>"`; `nmcli device` shows the connection name. |
| `the camera does not answer within 20s` | Unplug the cable and plug it back in. Turning the camera off and on with the cable attached can leave it unresponsive. |
| "Camera cannot start. Is its battery in and charged?", or `the camera cannot start capturing` in the log | The camera refused to start with error 4. Once in a while, right after a restart, this passes by itself: gpwebcam tries three times a second apart, then again every few seconds. If it repeats, the camera may be running without its battery: a HERO13 Black kept an existing stream going without one, but refused every new start. Put a charged battery in, turn the camera off, then turn it on and plug the cable back in. |
| `the camera reports streaming, but no video arrived` | gpwebcam retries by itself. If it keeps happening, a firewall or VPN is dropping the video: allow incoming UDP port 8554 on the GoPro connection. |
| An application does not list the camera | The service was not running when the application started. Start the service, then restart the application once. |
| `an application keeps the device at its size` | An application had the camera open when gpwebcam started or changed the resolution, so the device kept its old size. gpwebcam goes on at that size and tries again when applications start or stop using the camera; closing the camera in every application lets the new resolution apply. |
| The camera does not start when an application opens it | With camera mode `demand`, gpwebcam starts the camera when the application turns its video on, not when it lists cameras; give it about 5 seconds. `gpwebcam doctor` says whether your v4l2loopback reports applications at all; if not, the camera streams whenever it is connected. Mode `off` never starts it. |
| No tray icon | The desktop has no system tray (GNOME needs the AppIndicator extension), or the icon was hidden: `gpwebcam config tray on`. `gpwebcam doctor` checks both. |
| `settings file not used` or `settings file changed but cannot be used` | The settings file has an unknown setting or value; the log says which. gpwebcam keeps the defaults or the last good settings. Fix it with `gpwebcam config`, or delete the file. |
| An application reports the camera as busy | Another application has the camera open. Like any V4L2 camera, it can be used by one application at a time; close it in the other application first. |

## Limitations

- The camera's webcam mode provides at most 1080p at 30 fps, with no audio and no stabilization.
- One camera at a time.
- One application at a time can use the camera. This is a V4L2 rule that v4l2loopback enforces since version 0.14, the same as for a USB webcam.

## License

gpwebcam is licensed under the [Apache License 2.0](LICENSE).

GoPro and HERO are trademarks of GoPro, Inc. gpwebcam is an independent project, not affiliated with or endorsed by GoPro. It was inspired by [jschmid1/gopro_as_webcam_on_linux](https://github.com/jschmid1/gopro_as_webcam_on_linux) and shares no code with it. Camera behavior comes from GoPro's [Open GoPro](https://gopro.github.io/OpenGoPro/) specification and from tests on real hardware.
