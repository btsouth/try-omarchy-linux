//go:build linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// What the home and recovery screens say about storage: where the VM lives in
// words a person recognizes, what it really occupies next to the capacity the
// guest sees, and what stays when the app is removed.

// A row is a title with a short explanation under it. The window lays rows out
// in headed groups and never interprets their text.
type linuxRow struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

type linuxSection struct {
	Heading string     `json:"heading,omitempty"`
	Rows    []linuxRow `json:"rows"`
}

// linuxGB formats sizes the way the disk capacity is written in Settings, so
// "5.1 GB used of 24 GB" reads in one unit.
func linuxGB(bytes int64) string {
	const mb, gb = int64(1) << 20, int64(1) << 30
	switch {
	case bytes <= 0:
		return "0 MB"
	case bytes < gb/2:
		return strconv.FormatInt(max(bytes/mb, 1), 10) + " MB"
	case bytes >= 100*gb:
		return strconv.FormatInt((bytes+gb/2)/gb, 10) + " GB"
	}
	return strings.TrimSuffix(strconv.FormatFloat(float64(bytes)/float64(gb), 'f', 1, 64), ".0") + " GB"
}

// Sandboxed apps see a folder they were granted at /run/user/UID/doc/ID/NAME,
// which means nothing to the person who picked it. The Documents portal's Info
// call gives the entry's real path.
var linuxDocumentHostPath = documentPortalHostPath

func documentPortalHostPath(id string) (string, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var path []byte
	var apps map[string][]string
	call := conn.Object("org.freedesktop.portal.Documents", "/org/freedesktop/portal/documents").CallWithContext(ctx, "org.freedesktop.portal.Documents.Info", 0, id)
	if err := call.Store(&path, &apps); err != nil {
		return "", err
	}
	host := string(bytes.TrimRight(path, "\x00"))
	if !filepath.IsAbs(host) {
		return "", fmt.Errorf("the portal returned no usable path")
	}
	return host, nil
}

// linuxDocumentPath splits /run/user/UID/doc/ID/NAME/rest into the document ID
// and rest. The mount can sit below another runtime directory in a private
// session, so it is found by its doc/ID/NAME shape inside the user's runtime tree.
func linuxDocumentPath(path string, uid int) (id, rest string, ok bool) {
	runtimeRoot := fmt.Sprintf("/run/user/%d", uid)
	rel, err := filepath.Rel(runtimeRoot, filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	for i, part := range parts {
		if part == "doc" && i+2 < len(parts) && parts[i+1] != "" && parts[i+1] != "by-app" {
			return parts[i+1], filepath.Join(parts[i+3:]...), true
		}
	}
	return "", "", false
}

// linuxHostPath returns where path really is on the host. Anything that is not
// a document portal path is already a host path inside the sandbox.
func linuxHostPath(path string) string {
	id, rest, ok := linuxDocumentPath(path, os.Getuid())
	if !ok {
		return path
	}
	host, err := linuxDocumentHostPath(id)
	if err != nil {
		logf("storage: no host path for a portal folder: %v", err)
		return path
	}
	return filepath.Join(host, rest)
}

// linuxDisplayPath is the host path with the home folder shortened to ~, the
// way a file manager shows it.
func linuxDisplayPath(path string) string {
	return linuxShortenHome(linuxHostPath(path))
}

func linuxShortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(home) {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

func linuxAllocatedBytes(path string) int64 {
	var stat syscall.Stat_t
	if err := syscall.Lstat(path, &stat); err != nil || stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Blocks < 0 {
		return 0
	}
	return stat.Blocks * 512
}

// linuxTreeBytes adds up the space regular files under dir occupy. It never
// follows links and stops after a bounded number of entries.
func linuxTreeBytes(dir string) int64 {
	var total int64
	seen := 0
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if seen++; seen > 10000 {
			return filepath.SkipAll
		}
		if d.Type().IsRegular() {
			total += linuxAllocatedBytes(path)
		}
		return nil
	})
	return total
}

type linuxKeptCopy struct {
	Title string
	Path  string
	Bytes int64
}

// linuxKeptCopies lists what recovery keeps beside the current VM: disks left
// by resets and the original left by a move. Only the app's own records name
// these places; nothing is searched for.
func linuxKeptCopies(dir, defaultDir string) []linuxKeptCopy {
	var kept []linuxKeptCopy
	for _, disk := range linuxRetainedResetDisks(dir) {
		kept = append(kept, linuxKeptCopy{Title: "Disk kept from a reset", Path: filepath.Dir(disk), Bytes: linuxAllocatedBytes(disk)})
	}
	if retained, _ := linuxRetainedMove(defaultDir, dir); retained != nil {
		var size int64
		for _, part := range []string{"vm", "guest"} {
			size += linuxTreeBytes(filepath.Join(retained.Source, part))
		}
		kept = append(kept, linuxKeptCopy{Title: "Previous copy from the last move", Path: retained.Source, Bytes: size})
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Path < kept[j].Path })
	return kept
}

func linuxFreeBytes(dir string) int64 {
	free, err := diskFreeBytes(dir)
	if err != nil {
		return -1
	}
	return free
}

const (
	linuxAppStorageNote    = "Inside the app's own storage. Uninstalling keeps it unless you also delete the app's data."
	linuxFolderStorageNote = "A folder you chose. Uninstalling the app never touches it."
)

// linuxStorageRows is the storage summary. The compact form is for the home
// screen; the full form adds the guest system files, free space and every kept
// copy with its size and place.
func linuxStorageRows(dir, defaultDir string, full bool) []linuxRow {
	note := linuxFolderStorageNote
	if pathsEqual(dir, defaultDir) {
		note = linuxAppStorageNote
	}
	rows := []linuxRow{{Title: "Location", Detail: linuxDisplayPath(dir) + "\n" + note}}
	disk, err := os.Lstat(filepath.Join(dir, "vm", "disk.raw"))
	if err == nil && disk.Mode().IsRegular() {
		used := linuxAllocatedBytes(filepath.Join(dir, "vm", "disk.raw"))
		rows = append(rows, linuxRow{Title: "Omarchy disk",
			Detail: linuxGB(used) + " used of " + linuxGB(disk.Size()) + "\nOmarchy sees the full " + linuxGB(disk.Size()) + ". Only what it uses takes space here."})
	}
	kept := linuxKeptCopies(dir, defaultDir)
	if !full {
		if len(kept) > 0 {
			var total int64
			for _, copy := range kept {
				total += copy.Bytes
			}
			word := "copy uses"
			if len(kept) != 1 {
				word = "copies use"
			}
			rows = append(rows, linuxRow{Title: "Kept copies", Detail: fmt.Sprintf("%d %s %s. Manage them in Backup and recovery.", len(kept), word, linuxGB(total))})
		}
		return rows
	}
	if system := linuxTreeBytes(filepath.Join(dir, "guest")); system > 0 {
		rows = append(rows, linuxRow{Title: "System files", Detail: linuxGB(system) + "\nKept so Omarchy can start, repair itself and reset."})
	}
	if free := linuxFreeBytes(dir); free >= 0 {
		rows = append(rows, linuxRow{Title: "Free on this drive", Detail: linuxGB(free)})
	}
	for _, copy := range kept {
		rows = append(rows, linuxRow{Title: copy.Title, Detail: linuxGB(copy.Bytes) + "\n" + linuxDisplayPath(copy.Path)})
	}
	return rows
}
