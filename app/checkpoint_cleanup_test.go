package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpointRecoversInterruptedStagesOnly(t *testing.T) {
	s := checkpointFixture(t)
	completed, err := s.Create("Keep", nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, file string
		remove     bool
	}{
		{".pending-" + strings.Repeat("a", 32), ".try-omarchy-backup-123456", true},
		{".pending-" + strings.Repeat("e", 32), ".try-omarchy-backup-0f3a9c21d4e5b687", true},
		{".pending-" + strings.Repeat("f", 32), ".try-omarchy-backup-notours", false},
		{".pending-" + strings.Repeat("b", 32), "vm.zip", true},
		{".deleting-" + strings.Repeat("c", 32), "snapshot.json", true},
		{".pending-" + strings.Repeat("d", 32), "personal.txt", false},
		{".pending-unknown", "vm.zip", false},
	}
	for _, c := range cases {
		path := filepath.Join(s.path(), c.name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, c.file), []byte("preserve unexpected data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		_, err := os.Stat(filepath.Join(s.path(), c.name, c.file))
		if c.remove != os.IsNotExist(err) {
			t.Fatalf("cleanup %s: %v", c.name, err)
		}
	}
	entries, err := s.List()
	if err != nil || len(entries) != 1 || entries[0].ID != completed.ID {
		t.Fatal("changed completed snapshot", err)
	}
	if err := s.Recover(); err != nil {
		t.Fatal("second recovery", err)
	}
}

func TestCheckpointRecoveryHonorsActiveStoreLock(t *testing.T) {
	s := checkpointFixture(t)
	if err := os.MkdirAll(s.path(), 0700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(s.path(), ".pending-"+strings.Repeat("a", 32))
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	guard, err := lockMoveStore(moveStore{dir: s.path()})
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if err := s.Recover(); err == nil {
		t.Fatal("ignored active operation")
	}
	if _, err := os.Stat(stage); err != nil {
		t.Fatal("removed active stage", err)
	}
}
