//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// connectReclaimAgent starts a real guest agent with a fake guest connected,
// so the reclaim request travels the same socket protocol as in a VM.
func connectReclaimAgent(t *testing.T) (*guestAgent, net.Conn, *bufio.Reader) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	a := newGuestAgent()
	a.batteryLine = nil
	go a.accept(l)
	guest, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { guest.Close() })
	guest.Write([]byte("hello 2\n"))
	r := bufio.NewReader(guest)
	guest.SetReadDeadline(time.Now().Add(2 * time.Second))
	if line, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(line, "time ") {
		t.Fatalf("guest did not get the time on connect: %q %v", line, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for a.reclaimStatus() == "The guest agent is not connected. Wait for startup or update the guest." && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return a, guest, r
}

// useReclaimAgent installs a reclaim agent and state for one test and restores
// the previous ones afterwards. The host drive reports plenty of free space.
func useReclaimAgent(t *testing.T, a *guestAgent, dir string, supported bool) {
	t.Helper()
	oldAgent, oldDir, oldSupported := theAgent.Load(), reclaimDir.Load(), reclaimSupported.Load()
	t.Cleanup(func() {
		theAgent.Store(oldAgent)
		reclaimDir.Store(oldDir)
		reclaimSupported.Store(oldSupported)
	})
	theAgent.Store(a)
	reclaimDir.Store(&dir)
	reclaimSupported.Store(supported)
	oldFree := reclaimFreeBytes
	reclaimFreeBytes = func(string) (int64, error) { return 40 << 30, nil }
	t.Cleanup(func() { reclaimFreeBytes = oldFree })
}

// TestLinuxReclaimMessagesNameThisComputer keeps Windows wording out of Linux.
func TestLinuxReclaimMessagesNameThisComputer(t *testing.T) {
	for _, text := range []string{reclaimReadyStatus(), reclaimNeedsSpaceMessage(), reclaimUnsupportedMessage(), reclaimStartedMessage(), linuxReclaimPrompt(8192)} {
		if strings.Contains(text, "Windows") || strings.Contains(text, "tray") {
			t.Errorf("Linux reclaim text names another platform or a missing control: %q", text)
		}
	}
	if !strings.Contains(linuxReclaimPrompt(8192), "up to 8 GB") || !strings.Contains(linuxReclaimPrompt(0), "some of its free space") {
		t.Errorf("prompt does not state the pass size: %q", linuxReclaimPrompt(8192))
	}
}

// TestLinuxReclaimInfoBeforeAndDuringAPass walks the Settings card through
// startup, an unsupported folder, an idle agent, a running pass and its end.
func TestLinuxReclaimInfoBeforeAndDuringAPass(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vm"), 0o755); err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(dir, "vm", "disk.raw")
	if err := os.WriteFile(disk, bytes.Repeat([]byte{1}, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	useReclaimAgent(t, nil, dir, true)
	if info := linuxReclaimInfoFor(dir); info.CanStart || !strings.Contains(info.Status, "starting") {
		t.Fatalf("reclaim offered before the guest agent exists: %+v", info)
	}

	useReclaimAgent(t, newGuestAgent(), dir, false)
	if info := linuxReclaimInfoFor(dir); info.CanStart || info.Status != reclaimUnsupportedMessage() {
		t.Fatalf("reclaim offered on a folder that cannot release blocks: %+v", info)
	}

	useReclaimAgent(t, newGuestAgent(), dir, true)
	if info := linuxReclaimInfoFor(dir); info.CanStart || !strings.Contains(info.Status, "not connected") {
		t.Fatalf("reclaim offered with no guest helper connected: %+v", info)
	}

	a, guest, r := connectReclaimAgent(t)
	useReclaimAgent(t, a, dir, true)
	info := linuxReclaimInfoFor(dir)
	if !info.CanStart || !strings.Contains(info.Status, "Omarchy's disk uses") {
		t.Fatalf("idle reclaim info: %+v", info)
	}
	if !a.requestZeroFill(1024) {
		t.Fatal("zero-fill request was not sent")
	}
	if line, _ := r.ReadString('\n'); line != "zero-fill 1024\n" {
		t.Fatalf("guest got %q", line)
	}
	if info := linuxReclaimInfoFor(dir); info.CanStart || !strings.Contains(info.Status, "Keep Omarchy running") {
		t.Fatalf("a second pass was offered while one runs: %+v", info)
	}
	finished := make(chan bool, 1)
	a.reclaimFinished = func(ok bool) { finished <- ok }
	guest.Write([]byte("zero-fill done\n"))
	select {
	case ok := <-finished:
		if !ok {
			t.Fatal("finished pass reported as failed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("finished pass was not reported")
	}
	if info := linuxReclaimInfoFor(dir); info.CanStart || !strings.Contains(info.Status, reclaimReadyStatus()) {
		t.Fatalf("finished pass did not ask for shutdown: %+v", info)
	}
}

// TestLinuxReclaimFailureIsReportedOnceAndCanBeRetried ignores a repeated
// failure line and offers the pass again with the reason shown.
func TestLinuxReclaimFailureIsReportedOnceAndCanBeRetried(t *testing.T) {
	dir := t.TempDir()
	a, guest, r := connectReclaimAgent(t)
	useReclaimAgent(t, a, dir, true)
	results := make(chan bool, 4)
	a.reclaimFinished = func(ok bool) { results <- ok }
	if !a.requestZeroFill(512) {
		t.Fatal("zero-fill request was not sent")
	}
	r.ReadString('\n')
	guest.Write([]byte("zero-fill failed writing zeros\nzero-fill failed again\n"))
	select {
	case ok := <-results:
		if ok {
			t.Fatal("failure reported as success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("failure was not reported")
	}
	time.Sleep(50 * time.Millisecond)
	if len(results) != 0 {
		t.Fatal("an unrequested failure line was reported again")
	}
	info := linuxReclaimInfoFor(dir)
	if !info.CanStart || !strings.Contains(info.Status, "Preparation failed") {
		t.Fatalf("failed pass is not retryable with its reason shown: %+v", info)
	}
}

// TestLinuxReclaimFromSettingsStartsAPass sends the capped budget to the guest
// and refuses before contacting it when the drive lacks the reserve.
func TestLinuxReclaimFromSettingsStartsAPass(t *testing.T) {
	dir := t.TempDir()
	a, _, r := connectReclaimAgent(t)
	useReclaimAgent(t, a, dir, true)
	info := startLinuxReclaimFromSettings(dir)
	if info.CanStart || info.Status != reclaimStartedMessage() {
		t.Fatalf("Settings did not start a pass: %+v", info)
	}
	if line, _ := r.ReadString('\n'); line != fmt.Sprintf("zero-fill %d\n", reclaimPassCapMiB) {
		t.Fatalf("guest got %q", line)
	}
	if !a.reclaimInProgress() {
		t.Fatal("no pass in progress after Settings started one")
	}
	// A drive without the reserve refuses before anything reaches the guest.
	a2, _, _ := connectReclaimAgent(t)
	useReclaimAgent(t, a2, dir, true)
	reclaimFreeBytes = func(string) (int64, error) { return 3 << 30, nil }
	if info := startLinuxReclaimFromSettings(dir); info.Status != reclaimNeedsSpaceMessage() || a2.reclaimInProgress() {
		t.Fatalf("low space: %+v", info)
	}
}

// TestLinuxCanPunchHolesUsesItsOwnFile leaves the folder as it found it.
func TestLinuxCanPunchHolesUsesItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, ".reclaim-check-old")
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := linuxCanPunchHoles(dir)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("check left files behind: %v", entries)
	}
	// tmpfs and ext4 both support hole punching; a failure here means the
	// test machine's TMPDIR does not, which the launcher reports, not hides.
	if err != nil {
		t.Logf("hole punching unsupported in %s: %v", dir, err)
	}
	if err := linuxCanPunchHoles(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("an unusable folder was accepted")
	}
}

// TestLinuxReclaimCommandTalksToTheLifecyclePort covers -reclaim's answers:
// started, refused and no launcher running.
func TestLinuxReclaimCommandTalksToTheLifecyclePort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	replies := make(chan string, 2)
	replies <- "ok: " + reclaimStartedMessage() + "\n"
	replies <- "error: " + reclaimNeedsSpaceMessage() + "\n"
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			line, _ := bufio.NewReader(c).ReadString('\n')
			if line != "reclaim\n" {
				c.Write([]byte("error: unexpected\n"))
			} else {
				c.Write([]byte(<-replies))
			}
			c.Close()
		}
	}()
	var out, errOut bytes.Buffer
	if code := sendLinuxReclaim(l.Addr().String(), &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != reclaimStartedMessage() {
		t.Fatalf("started pass: code %d out %q err %q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := sendLinuxReclaim(l.Addr().String(), &out, &errOut); code != 1 || strings.TrimSpace(errOut.String()) != reclaimNeedsSpaceMessage() {
		t.Fatalf("refused pass: code %d out %q err %q", code, out.String(), errOut.String())
	}
	l.Close()
	errOut.Reset()
	if code := sendLinuxReclaim(l.Addr().String(), &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "not running") {
		t.Fatalf("no launcher: code %d err %q", code, errOut.String())
	}
}

// TestLinuxTrayReclaimOpensItsWindowOnce queues one window for repeated clicks.
func TestLinuxTrayReclaimOpensItsWindowOnce(t *testing.T) {
	before := linuxGUIEnabled
	linuxGUIEnabled = true
	defer func() { linuxGUIEnabled = before }()
	for len(linuxReclaimRequests) > 0 {
		<-linuxReclaimRequests
	}
	menu := &linuxTrayMenu{}
	menu.Event(3, "hovered", dbus.MakeVariant(""), 0)
	if len(linuxReclaimRequests) != 0 {
		t.Fatal("hover started reclaim")
	}
	menu.Event(3, "clicked", dbus.MakeVariant(""), 0)
	menu.Event(3, "clicked", dbus.MakeVariant(""), 0)
	if len(linuxReclaimRequests) != 1 {
		t.Fatalf("tray queued %d reclaim windows", len(linuxReclaimRequests))
	}
	<-linuxReclaimRequests
	if label := linuxTrayLayout(3).Properties["label"].Value(); label != "Reclaim disk space..." {
		t.Fatalf("tray label %v", label)
	}
}

// The reclaim helper answers the confirmation the way a person would. It
// speaks the setup pipe protocol without GTK.
func TestLinuxReclaimWindowHelper(t *testing.T) {
	answer := os.Getenv("TRY_OMARCHY_RECLAIM_ANSWER")
	if answer == "" {
		return
	}
	fmt.Println(`{"event":"ready"}`)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var state linuxSetupState
		if json.Unmarshal(scanner.Bytes(), &state) != nil || state.Request == 0 {
			continue
		}
		if state.Prompt == "choice" && !strings.Contains(state.Status, "at least 4 GB stays free") {
			panic("confirmation does not state the reserve: " + state.Status)
		}
		json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: answer})
	}
}

// TestLinuxReclaimWindowAsksBeforeStarting starts a pass only after Prepare.
func TestLinuxReclaimWindowAsksBeforeStarting(t *testing.T) {
	for _, tc := range []struct {
		answer  string
		started bool
	}{{"secondary", false}, {"primary", true}} {
		dir := t.TempDir()
		a, _, _ := connectReclaimAgent(t)
		useReclaimAgent(t, a, dir, true)
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxReclaimWindowHelper$")
		cmd.Env = append(os.Environ(), "TRY_OMARCHY_RECLAIM_ANSWER="+tc.answer)
		w := launchLinuxWindow(cmd, func() {})
		if w == nil {
			t.Fatal("helper did not start")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result := runLinuxReclaim(ctx, w, dir)
		cancel()
		w.stop()
		if a.reclaimInProgress() != tc.started {
			t.Fatalf("answer %s: started=%v", tc.answer, a.reclaimInProgress())
		}
		if tc.started && result != reclaimStartedMessage() || !tc.started && result != "" {
			t.Fatalf("answer %s: result %q", tc.answer, result)
		}
	}
}

// TestLinuxCompactionAfterAPassPunchesZeroBlocks frees zero blocks without
// changing what the disk reads, and reports the result.
func TestLinuxCompactionAfterAPassPunchesZeroBlocks(t *testing.T) {
	dir := t.TempDir()
	if err := linuxCanPunchHoles(dir); err != nil {
		t.Skipf("TMPDIR cannot punch holes: %v", err)
	}
	disk := filepath.Join(dir, "disk.raw")
	data := append(bytes.Repeat([]byte{7}, compactBlock), make([]byte, 4*compactBlock)...)
	if err := os.WriteFile(disk, data, 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := platformAllocatedFileBytes(disk)
	a, guest, r := connectReclaimAgent(t)
	useReclaimAgent(t, a, dir, true)
	a.requestZeroFill(512)
	r.ReadString('\n')
	guest.Write([]byte("zero-fill done\n"))
	deadline := time.Now().Add(2 * time.Second)
	for !a.compactPending() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	notices := make(chan string, 2)
	oldNotify, oldGUI := linuxNotify, linuxGUIEnabled
	linuxNotify = func(id, title, body string) error { notices <- title + ": " + body; return nil }
	linuxGUIEnabled = true
	defer func() { linuxNotify, linuxGUIEnabled = oldNotify, oldGUI }()
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	compactLinuxDisk(&config{disk: disk})
	after, _ := platformAllocatedFileBytes(disk)
	if after >= before {
		t.Fatalf("allocated bytes %d -> %d", before, after)
	}
	got, _ := os.ReadFile(disk)
	if !bytes.Equal(got, data) {
		t.Fatal("compaction changed disk contents")
	}
	select {
	case notice := <-notices:
		if !strings.HasPrefix(notice, "Space given back") || !strings.Contains(notice, "less than before") {
			t.Fatalf("notice %q", notice)
		}
	default:
		t.Fatal("no result notice")
	}
}
