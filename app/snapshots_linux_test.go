//go:build linux

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// linuxSnapshotFixture is a complete Linux VM folder, including the Linux-only
// display and keyboard preferences and a guest install receipt.
func linuxSnapshotFixture(t *testing.T) string {
	t.Helper()
	s := checkpointFixture(t)
	if err := saveLinuxExperiencePreferences(s.installation, linuxExperiencePreferences{Scale: "1.5", Keyboard: "de"}); err != nil {
		t.Fatal(err)
	}
	guest := filepath.Join(s.installation, "guest")
	names := []string{"build-spec.json", "rootfs.ext4", "vmlinuz-linux", "initramfs-linux.img"}
	sums := map[string]string{}
	for _, name := range names {
		sums[name] = strings.Repeat("a", 64)
	}
	if err := writeInstallReceipt(guest, "https://example.invalid/releases/download/linux-v0.1.1/", strings.Repeat("b", 64), names, sums); err != nil {
		t.Fatal(err)
	}
	return s.installation
}

func TestLinuxSnapshotsCarryLinuxPreferences(t *testing.T) {
	dir := linuxSnapshotFixture(t)
	if !backupNameAllowed(linuxExperiencePreferencesFilename) {
		t.Fatal("backups refuse the Linux display and keyboard preferences")
	}
	found := false
	for _, name := range checkpointRollbackNames() {
		found = found || name == linuxExperiencePreferencesFilename
	}
	if !found {
		t.Fatal("roll back would leave the Linux display and keyboard preferences behind")
	}
	store := checkpointStore{installation: dir}
	entry, err := store.Create("With German keyboard", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveLinuxExperiencePreferences(dir, linuxExperiencePreferences{Scale: "2", Keyboard: "fr"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(entry.ID, nil); err != nil {
		t.Fatal(err)
	}
	got, err := loadLinuxExperiencePreferences(dir)
	if err != nil || got.Scale != "1.5" || got.Keyboard != "de" {
		t.Fatalf("roll back restored %+v, %v", got, err)
	}
	copyDir := filepath.Join(filepath.Dir(dir), "copy")
	if err := store.Restore(entry.ID, copyDir, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := loadLinuxExperiencePreferences(copyDir); err != nil || got.Keyboard != "de" {
		t.Fatalf("restored copy has %+v, %v", got, err)
	}
}

func TestLinuxRollBackKeepsThePreviousStateUntilRemoved(t *testing.T) {
	dir := linuxSnapshotFixture(t)
	disk := filepath.Join(dir, "vm", "disk.raw")
	original, _ := os.ReadFile(disk)
	store := checkpointStore{installation: dir}
	entry, err := store.Create("Clean", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disk, []byte("work since the snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(entry.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(disk); string(got) != string(original) {
		t.Fatal("roll back did not restore the snapshot's disk")
	}
	kept := linuxRollbackKept(dir)
	if len(kept) != 1 {
		t.Fatalf("kept states: %v", kept)
	}
	if got, _ := os.ReadFile(filepath.Join(kept[0], "vm", "disk.raw")); string(got) != "work since the snapshot" {
		t.Fatal("the previous state was not kept")
	}
	rows := linuxStorageRows(dir, filepath.Join(t.TempDir(), "default"), true)
	if !strings.Contains(fmt.Sprint(rows), "State kept from a roll back") || !strings.Contains(fmt.Sprint(rows), "1 snapshot,") {
		t.Fatalf("storage rows do not show the snapshot and kept state: %+v", rows)
	}
	pinned, sums := "https://example.invalid/releases/download/linux-v0.2.0/", strings.Repeat("c", 64)
	runtimeRelease, runtimeSums := "", ""
	if ok, err := pinCheckpointBoot(dir, map[string]bool{}, &pinned, &sums, &runtimeRelease, &runtimeSums); err != nil || !ok {
		t.Fatalf("first boot after roll back is not pinned: %v %v", ok, err)
	}
	if !strings.Contains(pinned, "linux-v0.1.1") || sums != strings.Repeat("b", 64) {
		t.Fatalf("first boot would fetch newer system files instead of the snapshot's: %s %s", pinned, sums)
	}
	explicitRelease := "https://example.invalid/custom/"
	if ok, _ := pinCheckpointBoot(dir, map[string]bool{"release": true}, &explicitRelease, &sums, &runtimeRelease, &runtimeSums); ok || explicitRelease != "https://example.invalid/custom/" {
		t.Fatal("an explicit -release was overridden by the snapshot")
	}
	commitCheckpointBoot(dir)
	if _, err := os.Lstat(filepath.Join(dir, "vm", checkpointBootFilename)); !os.IsNotExist(err) {
		t.Fatal("first boot marker remained after a successful boot")
	}
	if err := removeLinuxRollbackKept(dir); err != nil {
		t.Fatal(err)
	}
	if len(linuxRollbackKept(dir)) != 0 {
		t.Fatal("kept state was not removed")
	}
	if got, _ := os.ReadFile(disk); string(got) != string(original) {
		t.Fatal("removing the kept state changed the current VM")
	}
	if entries, _ := store.List(); len(entries) != 1 {
		t.Fatal("removing the kept state changed the snapshots")
	}
}

func TestLinuxStartupFinishesAnInterruptedRollBack(t *testing.T) {
	dir := linuxSnapshotFixture(t)
	disk := filepath.Join(dir, "vm", "disk.raw")
	original, _ := os.ReadFile(disk)
	// A roll back that stopped after moving the current vm aside, before its
	// journal was committed, must put the original back.
	state := checkpointRollbackState{Version: 1, ID: randomCheckpointRollbackID()}
	stage := filepath.Join(dir, ".snapshot-rollback-"+state.ID)
	for _, name := range checkpointRollbackNames() {
		present, _ := rollbackPathExists(filepath.Join(dir, name))
		state.Items = append(state.Items, checkpointRollbackItem{Name: name, Previous: present, Next: present})
	}
	if err := os.MkdirAll(filepath.Join(stage, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stage, "next"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, item := range state.Items {
		if item.Previous {
			copyTree(t, filepath.Join(dir, item.Name), filepath.Join(stage, "next", item.Name))
		}
	}
	if err := saveCheckpointRollback(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "vm"), filepath.Join(stage, "data", "vm")); err != nil {
		t.Fatal(err)
	}
	preparing := filepath.Join(dir, ".snapshot-preparing-123")
	if err := os.MkdirAll(filepath.Join(preparing, "next"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A roll back that published its stage but crashed before the journal.
	unjournaled := filepath.Join(dir, ".snapshot-rollback-"+randomCheckpointRollbackID())
	if err := os.MkdirAll(filepath.Join(unjournaled, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join(stage, "next", "guest"), filepath.Join(unjournaled, "next", "guest"))
	if !linuxSnapshotRecoveryPending(dir) {
		t.Fatal("interrupted roll back not detected")
	}
	if err := recoverLinuxSnapshots(dir); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(disk); err != nil || string(got) != string(original) {
		t.Fatalf("the original disk was not put back: %v", err)
	}
	if linuxSnapshotRecoveryPending(dir) {
		t.Fatal("recovery left its journal or preparation folder")
	}
	if _, err := os.Stat(preparing); !os.IsNotExist(err) {
		t.Fatal("abandoned preparation folder was kept")
	}
	if _, err := os.Stat(unjournaled); !os.IsNotExist(err) {
		t.Fatal("an unjournaled stage holding only a snapshot copy was kept")
	}
	if kept := linuxRollbackKept(dir); len(kept) != 0 {
		t.Fatalf("an undone roll back is listed as kept state: %v", kept)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLinuxDeletingTheDefaultVMRemovesItsSnapshots(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "try-omarchy")
	for _, name := range []string{"vm/disk.raw", "guest/build-spec.json", "guest/rootfs.ext4", "guest/vmlinuz-linux", "guest/initramfs-linux.img"} {
		path := filepath.Join(defaultDir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o700)
		data := []byte("fixture")
		if name == "guest/build-spec.json" {
			data = []byte(`{"image":{"architecture":"x86_64"}}`)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := saveSettings(settingsPath(defaultDir), settings{SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	store := checkpointStore{installation: defaultDir}
	if _, err := store.Create("Before delete", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(linuxDeletePrompt(defaultDir), "frees about") {
		t.Fatal(linuxDeletePrompt(defaultDir))
	}
	if err := deleteLinuxDefaultVM(defaultDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(defaultDir, "checkpoints")); !os.IsNotExist(err) {
		t.Fatalf("snapshots outlived their VM: %v", err)
	}
	if _, err := os.Stat(settingsPath(defaultDir)); err != nil {
		t.Fatal("deleting the VM removed Try Omarchy's settings")
	}
}

func TestLinuxSnapshotRowsOpenEachSnapshot(t *testing.T) {
	when := time.Date(2026, 10, 1, 9, 30, 0, 0, time.Local)
	rows := linuxSnapshotRows([]vmCheckpoint{{ID: strings.Repeat("a", 32), Name: "Before update", Created: when, ArchiveBytes: 3 << 30}, {ID: strings.Repeat("b", 32), Name: "Damaged snapshot", Problem: "snapshot archive size changed"}})
	if rows[0].Reply != "snapshot:"+strings.Repeat("a", 32) || !strings.Contains(rows[0].Detail, "Oct 1, 2026 at 09:30") || !strings.Contains(rows[0].Detail, "3 GB") {
		t.Fatalf("row %+v", rows[0])
	}
	if rows[1].State != "unavailable" || !strings.Contains(rows[1].Detail, "cannot be used") {
		t.Fatalf("damaged row %+v", rows[1])
	}
	if empty := linuxSnapshotRows(nil); len(empty) != 1 || empty[0].Reply != "" {
		t.Fatalf("empty list %+v", empty)
	}
	if name := linuxDefaultSnapshotName(when); !validCheckpointName(name) {
		t.Fatalf("default name %q is not accepted by the store", name)
	}
}

// The snapshot helper plays a person through the pages over the real pipe
// protocol: create a named snapshot, open it, roll back, then leave.
func TestLinuxSnapshotPagesHelper(t *testing.T) {
	script := os.Getenv("TRY_OMARCHY_SNAPSHOT_SCRIPT")
	if script == "" {
		return
	}
	steps := strings.Split(script, ",")
	fmt.Println(`{"event":"ready"}`)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	log, _ := os.OpenFile(os.Getenv("TRY_OMARCHY_SNAPSHOT_LOG"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	defer log.Close()
	for scanner.Scan() {
		var state linuxSetupState
		if json.Unmarshal(scanner.Bytes(), &state) != nil || state.Request == 0 {
			continue
		}
		fmt.Fprintf(log, "%s|%s|%s|%s\n", state.Prompt, state.Title, state.Notice, strings.ReplaceAll(state.Status, "\n", " "))
		if len(steps) == 0 {
			panic("unexpected prompt " + state.Prompt)
		}
		answer := steps[0]
		steps = steps[1:]
		if answer == "first-snapshot" {
			answer = ""
			for _, section := range state.Sections {
				for _, row := range section.Rows {
					if row.Reply != "" && answer == "" {
						answer = row.Reply
					}
				}
			}
		}
		json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: answer})
	}
}

func TestLinuxSnapshotPagesCreateAndRollBack(t *testing.T) {
	dir := linuxSnapshotFixture(t)
	disk := filepath.Join(dir, "vm", "disk.raw")
	original, _ := os.ReadFile(disk)
	logPath := filepath.Join(t.TempDir(), "pages.log")
	run := func(script string) string {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxSnapshotPagesHelper$")
		cmd.Env = append(os.Environ(), "TRY_OMARCHY_SNAPSHOT_SCRIPT="+script, "TRY_OMARCHY_SNAPSHOT_LOG="+logPath)
		w := launchLinuxWindow(cmd, func() {})
		if w == nil {
			t.Fatal("helper did not start")
		}
		defer w.stop()
		return showLinuxSnapshots(w, dir)
	}
	result := run("create,Before trying Hyprland plugins,close")
	if !strings.Contains(result, `Snapshot "Before trying Hyprland plugins" saved`) {
		t.Fatalf("create result %q", result)
	}
	if err := os.WriteFile(disk, []byte("broken after plugins"), 0o600); err != nil {
		t.Fatal(err)
	}
	result = run("first-snapshot,rollback,primary,close")
	if result != "" {
		t.Fatalf("declining the roll back changed something: %q", result)
	}
	if got, _ := os.ReadFile(disk); string(got) != "broken after plugins" {
		t.Fatal("declined roll back changed the disk")
	}
	result = run("first-snapshot,rollback,secondary,close")
	if !strings.Contains(result, "Rolled back to") {
		t.Fatalf("roll back result %q", result)
	}
	if got, _ := os.ReadFile(disk); string(got) != string(original) {
		t.Fatal("roll back did not restore the disk")
	}
	pages, _ := os.ReadFile(logPath)
	for _, want := range []string{"snapshot-name|Create a snapshot||", "snapshot|Before trying Hyprland plugins||", "choice|Roll back to \"Before trying Hyprland plugins\"?||", "is kept in this VM's folder"} {
		if !strings.Contains(string(pages), want) {
			t.Errorf("pages did not show %q:\n%s", want, pages)
		}
	}
	// An invalid name is refused with a reason, and the form stays.
	result = run("create,   ,cancel,close")
	if result != "" {
		t.Fatalf("cancelled create: %q", result)
	}
	pages, _ = os.ReadFile(logPath)
	if !strings.Contains(string(pages), "1 to 80 characters") {
		t.Error("an empty snapshot name was not explained")
	}
}

func TestLinuxStoreLockFallsBackOnlyOnTheDocumentPortal(t *testing.T) {
	portal := fmt.Sprintf("/run/user/%d/doc/abc123/try-omarchy/checkpoints", os.Getuid())
	for _, tc := range []struct {
		dir  string
		err  error
		want bool
	}{
		{portal, syscall.ENOSYS, true},
		{portal, syscall.EOPNOTSUPP, true},
		{portal, syscall.EWOULDBLOCK, false},
		{"/home/someone/try-omarchy/checkpoints", syscall.ENOSYS, false},
		{fmt.Sprintf("/run/user/%d/doc", os.Getuid()), syscall.ENOSYS, false},
	} {
		if got := portalStoreLockFallback(tc.dir, tc.err); got != tc.want {
			t.Errorf("%s with %v: fallback %v, want %v", tc.dir, tc.err, got, tc.want)
		}
	}
	// A real held lock is still refused outside the portal.
	dir := t.TempDir()
	first, err := lockMoveStore(moveStore{dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := lockMoveStore(moveStore{dir: dir}); err == nil {
		second.Close()
		t.Fatal("a second store operation ran while the first held the lock")
	}
}
