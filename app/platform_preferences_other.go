//go:build !linux

package main

// platformPreferenceFiles are preference files only this platform writes.
// Windows has none beyond the shared set, so its archives and rollback
// journals keep their existing inventory.
var platformPreferenceFiles []string
