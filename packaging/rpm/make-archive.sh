#!/bin/sh
# Writes gpwebcam-<version>.tar.gz for the RPM spec: the source of the
# current commit with the Go modules vendored, so the RPM builds offline,
# as Copr and Fedora's mock do. Packit runs it as its create-archive
# action (.packit.yaml); it prints the archive's path, which Packit reads.
#   packaging/rpm/make-archive.sh <version> [<output directory>]
set -eu
version=$1
out=${2:-.}
root=$(git rev-parse --show-toplevel)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

git -C "$root" archive --prefix="gpwebcam-$version/" HEAD | tar -x -C "$work"
(cd "$work/gpwebcam-$version" && GOFLAGS=-mod=mod go mod vendor)
tar -C "$work" --sort=name --owner=0 --group=0 --numeric-owner \
  --mtime="@$(git -C "$root" log -1 --format=%ct)" \
  -czf "$out/gpwebcam-$version.tar.gz" "gpwebcam-$version"
echo "$out/gpwebcam-$version.tar.gz"
