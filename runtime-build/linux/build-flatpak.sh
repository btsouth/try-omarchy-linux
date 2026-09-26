#!/usr/bin/env bash
# Build the Linux Flatpak in the Flathub build image and export a bundle.
#   build-flatpak.sh OUTPUT_DIR
# OUTPUT_DIR keeps the builder state between runs, so rebuilds only redo the
# modules whose sources changed.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
mkdir -p "${1:?usage: build-flatpak.sh OUTPUT_DIR}"
out=$(cd "$1" && pwd)
image=ghcr.io/flathub-infra/flatpak-github-actions:freedesktop-25.08
docker run --rm --privileged -v "$repo":/src:ro -v "$out":/out "$image" bash -c "
  set -e
  flatpak-builder --disable-rofiles-fuse --force-clean --state-dir /out/state --repo /out/repo \
    /out/build /src/runtime-build/linux/com.tryomarchy.TryOmarchy.yml
  flatpak build-bundle /out/repo /out/com.tryomarchy.TryOmarchy.flatpak com.tryomarchy.TryOmarchy
  chown -R $(id -u):$(id -g) /out"
echo "bundle: $out/com.tryomarchy.TryOmarchy.flatpak"
