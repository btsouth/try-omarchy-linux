//go:build linux

package main

// progressUI is the Linux stand-in for the Windows setup window until the
// Linux front end has its own. Status lines go to the launcher log.
type progressUI struct{}

func getUI() *progressUI { return &progressUI{} }

func (*progressUI) setStatus(format string, a ...any) { logf(format, a...) }

func (*progressUI) setProgress(current, total int64) {}
