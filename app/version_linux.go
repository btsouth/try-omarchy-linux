//go:build linux

package main

// The Linux app is released separately from the Windows launcher, whose
// version is currentVersion, and from the guest image it pins. App releases are
// tagged linux-app-vX.Y.Z (preview builds add -preview.N); guest images are
// tagged linux-vX.Y.Z.
const linuxAppVersion = "v0.1.0-preview.5"
