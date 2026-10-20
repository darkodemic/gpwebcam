#!/bin/bash
# Builds packaging/aur/gpwebcam-git/PKGBUILD from a commit of this
# repository, as CI does, in an archlinux container:
#   docker run --rm -v "$PWD:/repo:ro" archlinux:latest \
#     /repo/packaging/aur/check-gpwebcam-git.sh "$(git rev-parse HEAD)"
# The checkout needs its tags (pkgver uses git describe). makepkg clones
# /repo instead of GitHub, so this checks the recipe against the code of
# that commit; it pushes nothing.
set -euo pipefail
commit=$1
pkg=/home/builder/pkg

pacman -Syu --noconfirm --needed base-devel git go namcap >/dev/null
useradd -m builder
install -d -o builder "$pkg"
cp /repo/packaging/aur/gpwebcam-git/PKGBUILD /repo/packaging/aur/gpwebcam.install "$pkg"/
sed -i "s|'git+https://github.com/darkodemic/gpwebcam.git'|'gpwebcam::git+file:///repo#commit=$commit'|" "$pkg/PKGBUILD"
grep -q "git+file:///repo" "$pkg/PKGBUILD"
chown -R builder "$pkg"
# The mounted checkout belongs to another user.
su builder -c "git config --global --add safe.directory '*'"
su builder -c "cd $pkg && makepkg -d --noconfirm"
namcap "$pkg/PKGBUILD"
# The .SRCINFO in the repository must describe the PKGBUILD; only the
# version and the source differ here.
diff <(su builder -c "cd $pkg && makepkg --printsrcinfo" | grep -v -e 'pkgver = ' -e 'source = ') \
  <(grep -v -e 'pkgver = ' -e 'source = ' /repo/packaging/aur/gpwebcam-git/.SRCINFO)
echo "gpwebcam-git builds; .SRCINFO matches"
