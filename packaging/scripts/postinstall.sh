#!/bin/sh
# Runs as root after the package is installed or upgraded. It only prints
# what to do next: packages must not load kernel modules behind the
# admin's back or start services for users.
set -e
cat <<'MSG'
gpwebcam: to use a GoPro as a webcam:
  1. Make sure the v4l2loopback module is installed (v4l2loopback-dkms or
     your distribution's module package). /usr/lib/modprobe.d/99-gpwebcam.conf
     sets it up with /dev/video42 "GoPro". If the module was already loaded
     before this installation, reboot, or reload it while no program uses it.
  2. As your user: systemctl --user enable --now gpwebcam.service
  3. On the camera, set Preferences > Connections > USB Connection to
     GoPro Connect, then plug it in.
  4. Check the setup: gpwebcam doctor
After an upgrade, as your user: systemctl --user daemon-reload
and then: systemctl --user restart gpwebcam
MSG
exit 0
