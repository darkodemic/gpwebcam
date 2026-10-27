# RPM spec for Copr (docs/plans-and-decisions/fedora-copr.md). It builds
# from the archive that packaging/rpm/make-archive.sh writes: the source
# with the Go modules vendored, so the build needs no network. Packit sets
# Version from the git tag and, for pull request builds only, adds a
# snapshot suffix to Release; release builds use the Release below. Keep
# the installed files in step with the nFPM packages in .goreleaser.yaml.

# The gobuild macro uses GOPATH mode unless told otherwise; gpwebcam is a
# Go module.
%global gomodulesmode GO111MODULE=on

Name:           gpwebcam
Version:        0.3.2
Release:        1%{?dist}
Summary:        Use a GoPro connected over USB as a Linux webcam

# gpwebcam and github.com/darkodemic/systray: Apache-2.0
# github.com/godbus/dbus/v5: BSD-2-Clause
# golang.org/x/sys: BSD-3-Clause
License:        Apache-2.0 AND BSD-2-Clause AND BSD-3-Clause
URL:            https://github.com/darkodemic/gpwebcam
Source0:        gpwebcam-%{version}.tar.gz

ExclusiveArch:  %{golang_arches}
BuildRequires:  golang >= 1.22
BuildRequires:  go-rpm-macros
BuildRequires:  systemd-rpm-macros
BuildRequires:  desktop-file-utils
# The tests run the real ffmpeg.
BuildRequires:  /usr/bin/ffmpeg

# Fedora's ffmpeg-free provides /usr/bin/ffmpeg but not the name ffmpeg.
Requires:       /usr/bin/ffmpeg
Requires:       hicolor-icon-theme
# The loopback module comes from RPM Fusion (akmod-v4l2loopback).
Recommends:     v4l2loopback
# notify-send, for desktop notifications.
Recommends:     libnotify

Provides:       bundled(golang(github.com/darkodemic/systray)) = 1.13.0
Provides:       bundled(golang(github.com/godbus/dbus/v5)) = 5.2.2
Provides:       bundled(golang(golang.org/x/sys)) = 0.27.0

%description
gpwebcam starts the webcam mode of a GoPro connected over USB through the
Open GoPro HTTP API and feeds its video into a v4l2loopback device that
browsers, Zoom and other video applications use like any other camera.
It runs as a systemd user service without root.

%prep
%autosetup

%build
# No -trimpath: debugedit needs the source paths for the debugsource package.
export GOFLAGS="-mod=vendor"
export LDFLAGS="-X main.version=%{version}"
%gobuild -o gpwebcam ./cmd/gpwebcam

%install
install -Dpm 0755 gpwebcam %{buildroot}%{_bindir}/gpwebcam
install -Dpm 0644 packaging/systemd/gpwebcam.service %{buildroot}%{_userunitdir}/gpwebcam.service
install -Dpm 0644 packaging/modules-load.d/gpwebcam.conf %{buildroot}%{_modulesloaddir}/gpwebcam.conf
install -Dpm 0644 packaging/modprobe.d/99-gpwebcam.conf %{buildroot}%{_modprobedir}/99-gpwebcam.conf
install -Dpm 0644 packaging/desktop/gpwebcam.desktop %{buildroot}%{_datadir}/applications/gpwebcam.desktop
install -Dpm 0644 packaging/icons/gpwebcam.svg %{buildroot}%{_datadir}/icons/hicolor/scalable/apps/gpwebcam.svg
install -Dpm 0644 packaging/man/gpwebcam.1 %{buildroot}%{_mandir}/man1/gpwebcam.1
mkdir -p licenses
for f in vendor/github.com/darkodemic/systray/LICENSE vendor/github.com/godbus/dbus/v5/LICENSE vendor/golang.org/x/sys/LICENSE; do
  install -Dpm 0644 "$f" "licenses/${f#vendor/}"
done

%check
desktop-file-validate %{buildroot}%{_datadir}/applications/gpwebcam.desktop
export GOFLAGS="-mod=vendor"
go test ./...

%files
%license LICENSE licenses/*
%doc README.md
%{_bindir}/gpwebcam
%{_userunitdir}/gpwebcam.service
%{_modulesloaddir}/gpwebcam.conf
%{_modprobedir}/99-gpwebcam.conf
%{_datadir}/applications/gpwebcam.desktop
%{_datadir}/icons/hicolor/scalable/apps/gpwebcam.svg
%{_mandir}/man1/gpwebcam.1*

%changelog
* Tue Oct 27 2026 Darko Demić <mail@darkodemic.com> - 0.3.2-1
- First Copr build.
