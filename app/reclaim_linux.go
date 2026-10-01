//go:build linux

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Giving space back to this computer. Deleting files inside Omarchy does not
// shrink disk.raw: the guest only marks the blocks free, so the sparse file
// keeps them allocated. A reclaim pass asks the guest agent to write zeros over
// its free space, then deletes them. After the guest powers off, the launcher
// punches holes through every all-zero block of disk.raw.
//
// A pass is started from running Settings, the tray, or `try-omarchy -reclaim`
// while Omarchy runs. Preparation temporarily uses host space, bounded by
// reclaimBudgetMiB, so the drive keeps a reserve.

// linuxReclaimInfo is what running Settings shows in its Storage card.
type linuxReclaimInfo struct {
	Status   string `json:"status"`
	CanStart bool   `json:"canStart"`
}

var linuxReclaimRequests = make(chan struct{}, 1)
var linuxReclaimOpen atomic.Bool

// requestTrayReclaim asks the supervisor to open the reclaim window.
func requestTrayReclaim() {
	if !linuxGUIEnabled {
		return
	}
	select {
	case linuxReclaimRequests <- struct{}{}:
	default:
	}
}

// linuxReclaimInfoFor describes the current pass for the running VM. It reads
// state only; it never starts a pass.
func linuxReclaimInfoFor(dir string) *linuxReclaimInfo {
	if !reclaimSupported.Load() {
		return &linuxReclaimInfo{Status: reclaimUnsupportedMessage}
	}
	a := theAgent.Load()
	if a == nil {
		return &linuxReclaimInfo{Status: "Omarchy is still starting. Reclaim becomes available once its desktop is up."}
	}
	usage := ""
	if used, err := platformAllocatedFileBytes(filepath.Join(dir, "vm", "disk.raw")); err == nil {
		usage = "Omarchy's disk uses " + linuxGB(used) + " on this drive. "
	}
	if a.reclaimInProgress() {
		return &linuxReclaimInfo{Status: usage + a.reclaimStatus()}
	}
	status := usage + "Reclaim gives back space from files you deleted inside Omarchy."
	if previous := a.reclaimStatus(); strings.HasPrefix(previous, "Preparation") || strings.HasPrefix(previous, "Could not") {
		status += " " + previous
	}
	return &linuxReclaimInfo{Status: status, CanStart: true}
}

// linuxReclaimPrompt says what a pass costs before it starts.
func linuxReclaimPrompt(budgetMiB int64) string {
	amount := "some of its free space"
	if budgetMiB > 0 {
		amount = "up to " + linuxGB(budgetMiB<<20) + " of its free space"
	}
	return "Omarchy prepares " + amount + " in this pass. While it does, that space is in use on this drive, and at least 4 GB stays free. " +
		"Keep Omarchy running until Try Omarchy says it is ready, then shut Omarchy down. Shutting down takes a little longer while the space is given back. " +
		"Your files inside Omarchy are not changed. Run it again for another pass if you deleted a lot."
}

// runLinuxReclaim confirms and starts a pass in an open window. It returns
// what to tell the person, or "" when they chose not to start.
func runLinuxReclaim(ctx context.Context, w *linuxSetupWindow, dir string) string {
	info := linuxReclaimInfoFor(dir)
	if !info.CanStart {
		return info.Status
	}
	var budget int64
	if free, err := reclaimFreeBytes(dir); err == nil {
		budget = reclaimBudgetMiB(free)
	}
	answer, err := w.ask(ctx, linuxSetupState{Prompt: "choice", Title: "Give unused space back?",
		Primary: "Prepare free space", Secondary: "Not now", Status: linuxReclaimPrompt(budget)})
	if err != nil || answer != "primary" {
		return ""
	}
	if err := requestReclaimError(); err != nil {
		return err.Error()
	}
	return reclaimStartedMessage
}

// showLinuxReclaim is the tray's reclaim window.
func showLinuxReclaim(parent context.Context, dir string) {
	if !linuxReclaimOpen.CompareAndSwap(false, true) {
		return
	}
	defer linuxReclaimOpen.Store(false)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	w := startLinuxWindow(cancel)
	if w == nil {
		return
	}
	defer w.stop()
	if result := runLinuxReclaim(ctx, w, dir); result != "" {
		w.ask(ctx, linuxSetupState{Prompt: "message", Title: "Reclaim disk space", Status: result})
	}
}

// startLinuxReclaimFromSettings starts a pass from running Settings. The
// Storage card's help states the cost next to the button, so Settings starts
// it directly and keeps the form, its edits and focus in place.
func startLinuxReclaimFromSettings(dir string) *linuxReclaimInfo {
	info := linuxReclaimInfoFor(dir)
	if !info.CanStart {
		return info
	}
	if err := requestReclaimError(); err != nil {
		info.Status = err.Error()
		return info
	}
	info = linuxReclaimInfoFor(dir)
	info.Status = reclaimStartedMessage
	return info
}

// linuxReclaimFinished tells the person when a pass needs their next step.
// Preparation can take minutes, so they should not have to keep checking.
func linuxReclaimFinished(ok bool) {
	if ok {
		tellLinuxUser("reclaim", "Ready to give space back", "Shut down Omarchy to give its unused space back to this computer. Shutting down takes a little longer while that happens.")
		return
	}
	tellLinuxUser("reclaim", "Space was not prepared", "Omarchy could not prepare its free space. Nothing changed. If it keeps failing, create diagnostics from Backup and recovery.")
}

// linuxCanPunchHoles checks that the folder holding disk.raw can give blocks
// back. Without that, preparing free space would only grow the disk file.
// The check uses its own small file, never the VM's disk.
func linuxCanPunchHoles(dir string) error {
	if stale, _ := filepath.Glob(filepath.Join(dir, ".reclaim-check-*")); len(stale) > 0 {
		for _, path := range stale {
			os.Remove(path)
		}
	}
	f, err := os.CreateTemp(dir, ".reclaim-check-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(make([]byte, 64<<10)); err != nil {
		return err
	}
	return punchHole(f, 0, 64<<10)
}

// configureLinuxReclaim enables reclaim when the data folder supports it.
func configureLinuxReclaim(cfg *config) {
	reclaimDir.Store(&cfg.dir)
	if err := linuxCanPunchHoles(cfg.dir); err != nil {
		logf("reclaim: unavailable, the data folder cannot release disk blocks: %v", err)
		reclaimSupported.Store(false)
		return
	}
	reclaimSupported.Store(true)
}

// compactLinuxDisk returns zero-filled blocks to the host after a reclaim.
// It shows progress, since reading a large disk takes a while, and says how
// much space came back.
func compactLinuxDisk(cfg *config) {
	a := theAgent.Load()
	if a == nil || !a.compactPending() {
		return
	}
	logf("reclaim: compacting %s", cfg.disk)
	var w *linuxSetupWindow
	if linuxGUIEnabled {
		w = startLinuxWindow(func() {})
		defer w.stop()
	}
	const status = "Giving unused space back..."
	detail := "Omarchy has shut down. Try Omarchy is returning the space it prepared. This can take a minute."
	var last time.Time
	report := func(done, total int64) {
		if w == nil || (time.Since(last) < 100*time.Millisecond && done < total) {
			return
		}
		last = time.Now()
		w.update(linuxSetupState{Status: status, Detail: detail, Current: done, Total: total, NonCancellable: true})
	}
	if w != nil {
		w.update(linuxSetupState{Status: status, Detail: detail, NonCancellable: true})
	}
	before, beforeErr := platformAllocatedFileBytes(cfg.disk)
	reclaimed, err := compactDisk(cfg.disk, report)
	if err != nil {
		logf("reclaim: compaction failed after %s: %v", formatGiB(reclaimed), err)
		tellLinuxUser("reclaim", "Space was not given back", "Omarchy shut down normally and your files are intact, but Try Omarchy could not return the prepared space: "+err.Error())
		return
	}
	after, afterErr := platformAllocatedFileBytes(cfg.disk)
	logf("reclaim: %s of zero blocks turned back into holes", formatGiB(reclaimed))
	if beforeErr != nil || afterErr != nil {
		tellLinuxUser("reclaim", "Space given back", "Omarchy shut down and its unused space was returned.")
		return
	}
	tellLinuxUser("reclaim", "Space given back", "Omarchy's disk now uses "+linuxGB(after)+" on this drive, "+linuxGB(max(before-after, 0))+" less than before.")
}

// sendLinuxReclaim asks a running launcher to start a pass. It is the
// `-reclaim` command, for scripts and terminals.
func sendLinuxReclaim(address string, out, errOut io.Writer) int {
	c, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		fmt.Fprintln(errOut, "Try Omarchy is not running. Start Omarchy, then try again.")
		return 1
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, "reclaim\n"); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	reply, err := bufio.NewReader(io.LimitReader(c, 4096)).ReadString('\n')
	if err != nil {
		fmt.Fprintln(errOut, "The running launcher did not confirm reclaim. It may need an update.")
		return 1
	}
	reply = strings.TrimSpace(reply)
	if text, ok := strings.CutPrefix(reply, "ok: "); ok {
		fmt.Fprintln(out, text)
		return 0
	}
	fmt.Fprintln(errOut, strings.TrimPrefix(reply, "error: "))
	return 1
}
