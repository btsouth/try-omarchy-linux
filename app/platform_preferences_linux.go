//go:build linux

package main

// platformPreferenceFiles are preference files only this platform writes.
// Backups, snapshots and rollbacks carry them with the shared ones, so a
// restored VM keeps its display scale and keyboard layout.
var platformPreferenceFiles = []string{linuxExperiencePreferencesFilename, linuxDisplaysFilename}
