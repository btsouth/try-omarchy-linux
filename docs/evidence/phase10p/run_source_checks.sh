#!/usr/bin/env bash
set -euo pipefail
root=/workspace/try-omarchy-linux/phase10l-graphics-src/app
out=/workspace/try-omarchy-linux/phase10l-graphics-build
export PATH="$out/go/bin:$PATH"
export DISPLAY=
export WAYLAND_DISPLAY=
export DBUS_SESSION_BUS_ADDRESS=
cd "$root"
echo 'Linux vet'
go vet ./...
echo 'Windows vet'
GOOS=windows GOARCH=amd64 go vet -unsafeptr=false ./...
echo 'Windows cross-build'
GOOS=windows GOARCH=amd64 go build ./...
echo 'Windows test compilation'
GOOS=windows GOARCH=amd64 go test -c -o "$out/app-phase10n.test.exe" .
GOOS=windows GOARCH=amd64 go test -c -o "$out/sign-update-phase10n.test.exe" ./cmd/sign-update
echo 'All cross-checks passed'
