//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLinuxGBUsesOneReadableUnit(t *testing.T) {
	const gb = int64(1) << 30
	for _, tc := range []struct {
		bytes int64
		want  string
	}{
		{0, "0 MB"},
		{-5, "0 MB"},
		{1, "1 MB"},
		{300 << 20, "300 MB"},
		{gb / 2, "0.5 GB"},
		{gb, "1 GB"},
		{24 * gb, "24 GB"},
		{5*gb + gb/10, "5.1 GB"},
		{gb*2 + gb*97/100, "3 GB"},
		{250*gb + 300<<20, "250 GB"},
	} {
		if got := linuxGB(tc.bytes); got != tc.want {
			t.Errorf("linuxGB(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestLinuxDocumentPathSplitsGrantAndRemainder(t *testing.T) {
	for _, tc := range []struct {
		path, id, rest string
		ok             bool
	}{
		{"/run/user/1000/doc/4f1a2b9c/MovedVM/try-omarchy", "4f1a2b9c", "try-omarchy", true},
		{"/run/user/1000/doc/4f1a2b9c/MovedVM", "4f1a2b9c", "", true},
		{"/run/user/1000/doc/4f1a2b9c/MovedVM/try-omarchy/vm/disk.raw", "4f1a2b9c", "try-omarchy/vm/disk.raw", true},
		{"/run/user/1000/omabox/box/private-runtime/doc/aa11/Fixtures/x", "aa11", "x", true},
		{"/run/flatpak/doc/stable/Omarchy/try-omarchy", "stable", "try-omarchy", true},
		{"/run/flatpak/doc/by-app/com.tryomarchy.TryOmarchy", "", "", false},
		{"/run/user/1000/doc/by-app/com.tryomarchy.TryOmarchy", "", "", false},
		{"/run/user/1000/doc/4f1a2b9c", "", "", false},
		{"/run/user/1001/doc/4f1a2b9c/MovedVM", "", "", false},
		{"/home/ana/.var/app/com.tryomarchy.TryOmarchy/data/try-omarchy", "", "", false},
	} {
		id, rest, ok := linuxDocumentPath(tc.path, 1000)
		if id != tc.id || rest != tc.rest || ok != tc.ok {
			t.Errorf("linuxDocumentPath(%q) = %q %q %t, want %q %q %t", tc.path, id, rest, ok, tc.id, tc.rest, tc.ok)
		}
	}
}

func TestLinuxHostPathAsksThePortalAndKeepsTheRemainder(t *testing.T) {
	uid := os.Getuid()
	docRoot := filepath.Join("/run/user", strconv.Itoa(uid), "doc")
	old := linuxDocumentHostPath
	defer func() { linuxDocumentHostPath = old }()
	var asked []string
	linuxDocumentHostPath = func(id string) (string, error) {
		asked = append(asked, id)
		if id == "gone" {
			return "", errors.New("no such document")
		}
		return "/home/ana/Drives/Omarchy", nil
	}
	if got := linuxHostPath(filepath.Join(docRoot, "abc123", "Omarchy", "try-omarchy")); got != "/home/ana/Drives/Omarchy/try-omarchy" {
		t.Fatalf("resolved portal path: %q", got)
	}
	if got := linuxHostPath("/run/flatpak/doc/stable/Omarchy/try-omarchy"); got != "/home/ana/Drives/Omarchy/try-omarchy" {
		t.Fatalf("resolved stable portal path: %q", got)
	}
	plain := "/home/ana/.var/app/com.tryomarchy.TryOmarchy/data/try-omarchy"
	if got := linuxHostPath(plain); got != plain {
		t.Fatalf("ordinary path changed: %q", got)
	}
	unresolved := filepath.Join(docRoot, "gone", "Omarchy")
	if got := linuxHostPath(unresolved); got != unresolved {
		t.Fatalf("an unknown grant must keep the path it was given: %q", got)
	}
	if len(asked) != 3 || asked[0] != "abc123" || asked[1] != "stable" {
		t.Fatalf("portal queries: %v", asked)
	}
}

func TestLinuxDisplayPathShortensOnlyTheHomeFolder(t *testing.T) {
	t.Setenv("HOME", "/home/ana")
	for path, want := range map[string]string{
		"/home/ana":                      "~",
		"/home/ana/Omarchy/try-omarchy":  "~/Omarchy/try-omarchy",
		"/home/anabel/Omarchy":           "/home/anabel/Omarchy",
		"/run/media/ana/USB/try-omarchy": "/run/media/ana/USB/try-omarchy",
		"/var/home/ana/elsewhere":        "/var/home/ana/elsewhere",
	} {
		if got := linuxDisplayPath(path); got != want {
			t.Errorf("linuxDisplayPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func sparseFile(t *testing.T, path string, apparent int64, written int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if written > 0 {
		if _, err := f.Write(make([]byte, written)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Truncate(apparent); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxStorageRowsSeparateUsageFromCapacityAndExplainRemoval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default", "try-omarchy")
	folder := filepath.Join(root, "chosen", "try-omarchy")
	for _, dir := range []string{defaultDir, folder} {
		sparseFile(t, filepath.Join(dir, "vm", "disk.raw"), 24<<20, 3<<20)
	}
	rows := linuxStorageRows(defaultDir, defaultDir, false)
	if len(rows) != 2 || rows[0].Title != "Location" || !strings.Contains(rows[0].Detail, "Uninstalling keeps it unless you also delete the app's data") {
		t.Fatalf("app storage rows: %+v", rows)
	}
	if !strings.Contains(rows[1].Detail, "3 MB used of 24 MB") || !strings.Contains(rows[1].Detail, "Only what it uses takes space here") {
		t.Fatalf("disk row must separate real use from capacity: %q", rows[1].Detail)
	}
	rows = linuxStorageRows(folder, defaultDir, false)
	if !strings.Contains(rows[0].Detail, "Uninstalling the app never touches it") || !strings.Contains(rows[0].Detail, folder) {
		t.Fatalf("chosen folder row: %q", rows[0].Detail)
	}
}

func TestLinuxStorageRowsListKeptCopiesWithSizesAndPlaces(t *testing.T) {
	t.Setenv("HOME", "/nonexistent-home")
	root := t.TempDir()
	dir := filepath.Join(root, "try-omarchy")
	sparseFile(t, filepath.Join(dir, "vm", "disk.raw"), 24<<20, 1<<20)
	sparseFile(t, filepath.Join(dir, "guest", "rootfs.ext4"), 8<<20, 2<<20)
	kept := filepath.Join(dir, "vm", linuxResetRetainedPrefix+"111")
	sparseFile(t, filepath.Join(kept, "disk.raw"), 24<<20, 4<<20)
	if got := linuxKeptCopies(dir, dir); len(got) != 1 || got[0].Path != kept || got[0].Bytes < 4<<20 {
		t.Fatalf("kept copies: %+v", got)
	}
	compact := linuxStorageRows(dir, dir, false)
	if last := compact[len(compact)-1]; last.Title != "Kept copies" || !strings.Contains(last.Detail, "1 copy uses") || !strings.Contains(last.Detail, "Backup and recovery") {
		t.Fatalf("compact kept summary: %+v", compact)
	}
	full := linuxStorageRows(dir, dir, true)
	titles := map[string]string{}
	for _, row := range full {
		titles[row.Title] = row.Detail
	}
	for _, want := range []string{"Location", "Omarchy disk", "System files", "Free on this drive", "Disk kept from a reset"} {
		if _, ok := titles[want]; !ok {
			t.Fatalf("full view lacks %q: %+v", want, full)
		}
	}
	if !strings.Contains(titles["Disk kept from a reset"], kept) {
		t.Fatalf("a kept copy must say where it is: %q", titles["Disk kept from a reset"])
	}
}

func TestLinuxStorageRowsForAnEmptyInstallShowOnlyTheLocation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "try-omarchy")
	rows := linuxStorageRows(dir, dir, false)
	if len(rows) != 1 || rows[0].Title != "Location" {
		t.Fatalf("rows for a VM that is not installed: %+v", rows)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("reading storage created %s: %v", dir, err)
	}
}
