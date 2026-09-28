#!/usr/bin/env bash
# Local source checks. Windows tests are compiled here and run only in CI.
set -euo pipefail
repo=$(cd "$(dirname "$0")/../.." && pwd)
out=${1:?usage: check.sh EVIDENCE_DIRECTORY}
mkdir -p "$out"
out=$(realpath "$out")
cd "$repo/app"
go test -race -count=1 ./... >"$out/linux-tests.log" 2>&1
go vet ./... >"$out/linux-vet.log" 2>&1
GOOS=windows GOARCH=amd64 go vet -unsafeptr=false ./... >"$out/windows-vet.log" 2>&1
GOOS=windows GOARCH=amd64 go build -o /dev/null .
GOOS=windows GOARCH=amd64 go test -c -o /dev/null .
CGO_ENABLED=0 go build -trimpath -o "$out/try-omarchy" .
sha256sum "$out/try-omarchy" >"$out/launcher.sha256"
git -C "$repo" diff --check
printf 'Source checks passed. Windows tests were compiled, not run.\n'
