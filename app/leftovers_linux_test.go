//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Not a test: a child process that starts a restore and is killed, the way a
// crash or a forced quit ends one, so its cleanup never runs.
func TestLinuxLeftoverKillHelper(t *testing.T) {
	if os.Getenv("TRY_OMARCHY_LEFTOVER_KILL") == "" {
		return
	}
	configureSetupCancellation(false)
	journalLinuxStaging(os.Getenv("TRY_OMARCHY_LEFTOVER_DEFAULT"))
	steps := 0
	restoreVMBackupProgress(os.Getenv("TRY_OMARCHY_LEFTOVER_ARCHIVE"), os.Getenv("TRY_OMARCHY_LEFTOVER_DEST"), func(done, _ int64, _ string) {
		// Let a few files finish, so the folder holds a real partial copy.
		if steps++; done > 0 && steps >= 4 {
			os.Exit(3)
		}
	})
	os.Exit(4)
}

func killedRestore(t *testing.T) (defaultDir, parent, staging string) {
	t.Helper()
	dir, archive := backupFixture(t)
	if err := writeVMBackup(dir, archive); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	defaultDir = filepath.Join(root, "data", "try-omarchy")
	parent = filepath.Join(root, "restore-here")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxLeftoverKillHelper$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_LEFTOVER_KILL=1", "TRY_OMARCHY_LEFTOVER_DEFAULT="+defaultDir,
		"TRY_OMARCHY_LEFTOVER_ARCHIVE="+archive, "TRY_OMARCHY_LEFTOVER_DEST="+filepath.Join(parent, "try-omarchy-restored-x"))
	if err := cmd.Run(); err == nil || cmd.ProcessState.ExitCode() != 3 {
		t.Fatalf("the restore must be cut off mid-way: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(parent, ".try-omarchy-restore-*"))
	if len(matches) != 1 {
		t.Fatalf("expected one abandoned staging folder, found %v", matches)
	}
	return defaultDir, parent, matches[0]
}

func TestAnInterruptedRestoreIsFoundByItsRecordAndRemovedSafely(t *testing.T) {
	t.Setenv("HOME", "/nonexistent-home")
	defaultDir, parent, staging := killedRestore(t)
	if !regexp.MustCompile(`^\.try-omarchy-restore-[0-9a-f]{16}$`).MatchString(filepath.Base(staging)) {
		t.Fatalf("staging name: %s", staging)
	}
	found := linuxFindLeftovers(defaultDir)
	if len(found) != 1 || found[0].Path != staging || found[0].Status != leftoverPresent || found[0].Bytes <= 0 {
		t.Fatalf("found: %+v", found)
	}
	rows := linuxLeftoverRows(found)
	if len(rows) != 1 || rows[0].Title != "Unfinished restore" || !strings.Contains(rows[0].Detail, "in "+parent) || !strings.Contains(rows[0].Detail, "only a partial copy of a VM") {
		t.Fatalf("rows: %+v", rows)
	}
	if notice := linuxLeftoverNotice(defaultDir); !strings.Contains(notice, "interrupted") || !strings.Contains(notice, "Backup and recovery can remove it") {
		t.Fatalf("home notice: %q", notice)
	}
	// Unrelated files beside it, and the backup being restored, are not touched.
	bystander := filepath.Join(parent, "my-notes.txt")
	if err := os.WriteFile(bystander, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeLinuxLeftover(defaultDir, found[0].linuxLeftover); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging folder still there: %v", err)
	}
	if data, err := os.ReadFile(bystander); err != nil || string(data) != "keep" {
		t.Fatalf("a neighbouring file was touched: %v", err)
	}
	if got := loadLinuxLeftovers(defaultDir); len(got) != 0 {
		t.Fatalf("the record must be cleared once removed: %+v", got)
	}
	if _, err := os.Stat(linuxLeftoverJournal(defaultDir)); !os.IsNotExist(err) {
		t.Fatalf("an empty journal should not linger: %v", err)
	}
}

func TestNormalRestoresAndBackupsLeaveNoRecord(t *testing.T) {
	dir, archive := backupFixture(t)
	defaultDir := filepath.Join(t.TempDir(), "try-omarchy")
	stop := journalLinuxStaging(defaultDir)
	defer stop()
	if err := writeVMBackup(dir, archive); err != nil {
		t.Fatal(err)
	}
	if err := restoreVMBackup(archive, filepath.Join(filepath.Dir(dir), "restored")); err != nil {
		t.Fatal(err)
	}
	if got := loadLinuxLeftovers(defaultDir); len(got) != 0 {
		t.Fatalf("finished work must not be recorded as a leftover: %+v", got)
	}
	// A failed one cleans up after itself and clears its record too.
	if err := restoreVMBackup(archive, filepath.Join(filepath.Dir(dir), "restored")); err == nil {
		t.Fatal("restoring over an existing folder must fail")
	}
	if got := loadLinuxLeftovers(defaultDir); len(got) != 0 {
		t.Fatalf("a refused restore left a record: %+v", got)
	}
}

func TestAnUnwritableJournalStopsTheOperationBeforeAnythingIsStaged(t *testing.T) {
	dir, archive := backupFixture(t)
	root := t.TempDir()
	blocker := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocker, []byte("a file, not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := journalLinuxStaging(filepath.Join(blocker, "try-omarchy"))
	defer stop()
	err := writeVMBackup(dir, archive)
	if err == nil || !strings.Contains(err.Error(), "could not record the temporary files") {
		t.Fatalf("with no record possible, the backup must not start writing: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(archive), ".try-omarchy-*")); len(matches) != 0 {
		t.Fatalf("staging was created without a record: %v", matches)
	}
}

func TestOnlyWhatMatchesTheShapeOfAStagingFolderIsEverOfferedForRemoval(t *testing.T) {
	t.Setenv("HOME", "/nonexistent-home")
	root := t.TempDir()
	defaultDir := filepath.Join(root, "data", "try-omarchy")
	parent := filepath.Join(root, "external")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	name := func(kind string) string { return filepath.Join(parent, ".try-omarchy-"+kind+"-0123456789abcdef") }

	// A real folder that happens to hold the person's own files.
	userFolder := name("restore")
	os.MkdirAll(filepath.Join(userFolder, "guest"), 0o700)
	os.WriteFile(filepath.Join(userFolder, "holiday-photos.zip"), []byte("precious"), 0o600)
	// A link, even to a harmless place.
	linkTarget := filepath.Join(root, "elsewhere")
	os.MkdirAll(linkTarget, 0o700)
	os.WriteFile(filepath.Join(linkTarget, "keep.txt"), []byte("keep"), 0o600)

	cases := []struct {
		name  string
		entry linuxLeftover
		want  linuxLeftoverStatus
	}{
		{"folder with foreign files", linuxLeftover{Kind: "restore", Path: userFolder}, leftoverUnrecognized},
		{"wrong kind for the name", linuxLeftover{Kind: "backup", Path: userFolder}, leftoverUnrecognized},
		{"a name Try Omarchy never makes", linuxLeftover{Kind: "restore", Path: filepath.Join(parent, "Documents")}, leftoverUnrecognized},
		{"a numeric legacy name", linuxLeftover{Kind: "restore", Path: filepath.Join(parent, ".try-omarchy-restore-3651067293")}, leftoverUnrecognized},
		{"a relative path", linuxLeftover{Kind: "restore", Path: ".try-omarchy-restore-0123456789abcdef"}, leftoverUnrecognized},
		{"an unclean path", linuxLeftover{Kind: "restore", Path: parent + "/../external/.try-omarchy-restore-0123456789abcdef"}, leftoverUnrecognized},
		{"a name of the wrong shape", linuxLeftover{Kind: "restore", Path: name("backup")[:len(name("backup"))-1] + "0"}, leftoverUnrecognized},
		{"a folder on an unplugged drive", linuxLeftover{Kind: "restore", Path: filepath.Join(root, "no-such-drive", ".try-omarchy-restore-0123456789abcdef")}, leftoverUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := inspectLinuxLeftover(tc.entry); got != tc.want {
				t.Fatalf("status %d, want %d", got, tc.want)
			}
		})
	}
	// A recorded name whose folder is gone is simply forgotten.
	gone := linuxLeftover{Kind: "restore", Path: filepath.Join(parent, ".try-omarchy-restore-aaaaaaaaaaaaaaaa")}
	if got, _ := inspectLinuxLeftover(gone); got != leftoverGone {
		t.Fatalf("missing folder: %d", got)
	}
	// A symlink at the recorded path is never followed or removed.
	linked := filepath.Join(parent, ".try-omarchy-restore-bbbbbbbbbbbbbbbb")
	if err := os.Symlink(linkTarget, linked); err != nil {
		t.Fatal(err)
	}
	if got, _ := inspectLinuxLeftover(linuxLeftover{Kind: "restore", Path: linked}); got != leftoverUnrecognized {
		t.Fatalf("symlink: %d", got)
	}
	for _, entry := range []linuxLeftover{{Kind: "restore", Path: userFolder}, {Kind: "restore", Path: linked}} {
		if err := saveLinuxLeftovers(defaultDir, []linuxLeftover{entry}); err != nil {
			t.Fatal(err)
		}
		if err := removeLinuxLeftover(defaultDir, entry); err == nil || !strings.Contains(err.Error(), "left alone") {
			t.Fatalf("removal of %s must refuse: %v", entry.Path, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(userFolder, "holiday-photos.zip")); err != nil || string(data) != "precious" {
		t.Fatalf("a user's own files were deleted: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(linkTarget, "keep.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("a symlink target was deleted: %v", err)
	}
}

func TestAnInterruptedBackupFileIsRemovedButAnUnexpectedFileIsNot(t *testing.T) {
	t.Setenv("HOME", "/nonexistent-home")
	root := t.TempDir()
	defaultDir := filepath.Join(root, "try-omarchy")
	partial := filepath.Join(root, ".try-omarchy-backup-0123456789abcdef")
	if err := os.WriteFile(partial, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := linuxLeftover{Kind: "backup", Path: partial}
	if err := saveLinuxLeftovers(defaultDir, []linuxLeftover{entry}); err != nil {
		t.Fatal(err)
	}
	rows := linuxLeftoverRows(linuxFindLeftovers(defaultDir))
	if len(rows) != 1 || rows[0].Title != "Unfinished backup" || !strings.Contains(rows[0].Detail, "not a usable backup") {
		t.Fatalf("rows: %+v", rows)
	}
	prompt := linuxLeftoverPrompt(linuxRemovableLeftovers(linuxFindLeftovers(defaultDir)))
	if !strings.Contains(prompt, partial) || !strings.Contains(prompt, "Only these files are removed") {
		t.Fatalf("confirmation must name the exact file: %q", prompt)
	}
	if err := removeLinuxLeftover(defaultDir, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(partial); !os.IsNotExist(err) {
		t.Fatal("the partial backup was not removed")
	}
	// A directory where a backup file should be is not what a backup writes.
	notAFile := filepath.Join(root, ".try-omarchy-backup-fedcba9876543210")
	os.Mkdir(notAFile, 0o700)
	if got, _ := inspectLinuxLeftover(linuxLeftover{Kind: "backup", Path: notAFile}); got != leftoverUnrecognized {
		t.Fatalf("directory as backup file: %d", got)
	}
}
