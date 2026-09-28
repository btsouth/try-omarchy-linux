#!/usr/bin/env bash
set -euo pipefail
root=/workspace/try-omarchy-linux/phase10l-graphics-build
source=/workspace/try-omarchy-linux/phase10l-graphics-src
image=ghcr.io/flathub-infra/flatpak-github-actions@sha256:1de59efef01a946d9f28f1ec3458bf6f61dcee3507d7fecdd473f8134da4ec77
docker run --rm --network host --privileged --device /dev/fuse \
  -e HOME=/tmp -v /etc/machine-id:/etc/machine-id:ro \
  -v "$root":/out -v "$source":/src:ro "$image" sh -c '
    set -eu
    mkdir -p /tmp/runtime-user
    chmod 700 /tmp/runtime-user
    export XDG_RUNTIME_DIR=/tmp/runtime-user
    dbus-run-session -- sh -c '\''
      set -eu
      flatpak install --user --noninteractive /out/com.tryomarchy.TryOmarchy.flatpak
      flatpak run --user --devel --filesystem=/src:ro --filesystem=/out \
        --env=PATH=/out/go/bin:/app/bin:/usr/bin \
        --env=GOCACHE=/out/compatible-gocache \
        --env=GOMODCACHE=/out/compatible-gomodcache \
        --env=GOFLAGS=-mod=readonly \
        --env=DISPLAY= --env=WAYLAND_DISPLAY= \
        --command=sh com.tryomarchy.TryOmarchy -c \
        "cd /src/app && qemu-system-x86_64 --version && go version && go test -race -count=1 ./..."
    '\''
  '
