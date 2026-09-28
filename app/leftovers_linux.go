//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A restore stages a whole VM, and a backup a large temporary file, in a folder
// the person picked. If the launcher is killed, that stays behind as a hidden
// file nobody can tell from anything else. Each path is written to the app's own
// journal before it is created, so a leftover is known by its exact name and
// place. Nothing is ever searched for, and only a path that still has the
// shape of what the launcher makes is offered for removal.

const linuxLeftoverJournalName = "leftovers.json"

type linuxLeftover struct {
	Kind    string `json:"kind"` // "restore" or "backup"
	Path    string `json:"path"`
	Started string `json:"started"`
}

var (
	linuxLeftoverMu   sync.Mutex
	linuxLeftoverName = regexp.MustCompile(`^\.try-omarchy-(restore|backup)-[0-9a-f]{16}$`)
)

func linuxLeftoverJournal(defaultDir string) string {
	return filepath.Join(linuxMoveStore(defaultDir).dir, linuxLeftoverJournalName)
}

func loadLinuxLeftovers(defaultDir string) []linuxLeftover {
	data, err := os.ReadFile(linuxLeftoverJournal(defaultDir))
	if err != nil || len(data) > 1<<20 {
		return nil
	}
	var entries []linuxLeftover
	if err := json.Unmarshal(data, &entries); err != nil {
		logf("leftovers: unreadable journal ignored: %v", err)
		return nil
	}
	return entries
}

// saveLinuxLeftovers writes durably: a record that a crash could lose is no record.
func saveLinuxLeftovers(defaultDir string, entries []linuxLeftover) error {
	path := linuxLeftoverJournal(defaultDir)
	if len(entries) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	staged := path + ".part"
	f, err := os.OpenFile(staged, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(staged, path)
}

// journalLinuxStaging records staging paths for one recovery operation. If the
// journal cannot be written the operation stops before creating anything.
func journalLinuxStaging(defaultDir string) (stop func()) {
	stagingBegin = func(kind, path string) error {
		linuxLeftoverMu.Lock()
		defer linuxLeftoverMu.Unlock()
		entries := append(loadLinuxLeftovers(defaultDir), linuxLeftover{Kind: kind, Path: path, Started: time.Now().Format(time.RFC3339)})
		if err := saveLinuxLeftovers(defaultDir, entries); err != nil {
			return fmt.Errorf("could not record the temporary files this needs: %w", err)
		}
		return nil
	}
	stagingEnd = func(path string) { forgetLinuxLeftover(defaultDir, path) }
	return func() { stagingBegin, stagingEnd = nil, nil }
}

func forgetLinuxLeftover(defaultDir, path string) {
	linuxLeftoverMu.Lock()
	defer linuxLeftoverMu.Unlock()
	entries := loadLinuxLeftovers(defaultDir)
	kept := entries[:0]
	for _, e := range entries {
		if e.Path != path {
			kept = append(kept, e)
		}
	}
	if len(kept) != len(entries) {
		if err := saveLinuxLeftovers(defaultDir, kept); err != nil {
			logf("leftovers: %v", err)
		}
	}
}

type linuxLeftoverStatus int

const (
	leftoverPresent      linuxLeftoverStatus = iota // safe to offer for removal
	leftoverGone                                    // nothing there any more
	leftoverUnreachable                             // its folder cannot be reached, for example an unplugged drive
	leftoverUnrecognized                            // something is there, but it is not what the launcher made
)

// The top-level names a restore writes. Anything else in a staging folder means
// it is not one, whatever it is called.
func linuxRestoreStagingNames() map[string]bool {
	names := map[string]bool{"guest": true, "runtime": true, "vm": true, "settings.json": true,
		storageSettingsFilename: true, desktopPreferencesFilename: true, launchPreferencesFilename: true,
		audioPreferencesFilename: true, audioEndpointsFilename: true, resourcePreferencesFilename: true}
	for index := 0; index < maximumGuestDisplays; index++ {
		names[displayPlacementFilename(index)] = true
	}
	return names
}

// inspectLinuxLeftover checks a recorded path against the shape of what the
// launcher creates: its own name pattern, a real folder or file owned by this
// user, and for a restore, nothing but the files a restore writes.
func inspectLinuxLeftover(l linuxLeftover) (linuxLeftoverStatus, int64) {
	path := filepath.Clean(l.Path)
	base := filepath.Base(path)
	match := linuxLeftoverName.FindStringSubmatch(base)
	if !filepath.IsAbs(path) || path != l.Path || match == nil || match[1] != l.Kind {
		return leftoverUnrecognized, 0
	}
	if _, err := os.Lstat(filepath.Dir(path)); err != nil {
		return leftoverUnreachable, 0
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return leftoverGone, 0
	}
	if err != nil {
		return leftoverUnreachable, 0
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Getuid() {
		return leftoverUnrecognized, 0
	}
	switch l.Kind {
	case "backup":
		if !info.Mode().IsRegular() {
			return leftoverUnrecognized, 0
		}
		return leftoverPresent, linuxAllocatedBytes(path)
	case "restore":
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return leftoverUnrecognized, 0
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return leftoverUnreachable, 0
		}
		allowed := linuxRestoreStagingNames()
		for _, entry := range entries {
			if !allowed[entry.Name()] {
				return leftoverUnrecognized, 0
			}
		}
		return leftoverPresent, linuxTreeBytes(path)
	}
	return leftoverUnrecognized, 0
}

type linuxLeftoverFound struct {
	linuxLeftover
	Status linuxLeftoverStatus
	Bytes  int64
}

// linuxFindLeftovers reports the journal's entries, and drops the ones whose
// files are gone so the list only holds what still matters.
func linuxFindLeftovers(defaultDir string) []linuxLeftoverFound {
	var found []linuxLeftoverFound
	for _, entry := range loadLinuxLeftovers(defaultDir) {
		status, size := inspectLinuxLeftover(entry)
		if status == leftoverGone {
			forgetLinuxLeftover(defaultDir, entry.Path)
			continue
		}
		found = append(found, linuxLeftoverFound{entry, status, size})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Started < found[j].Started })
	return found
}

func linuxLeftoverTitle(l linuxLeftover) string {
	if l.Kind == "backup" {
		return "Unfinished backup"
	}
	return "Unfinished restore"
}

// linuxLeftoverRows describe what an interrupted run left behind and where, in
// words that say it is safe to remove.
func linuxLeftoverRows(found []linuxLeftoverFound) []linuxRow {
	var rows []linuxRow
	for _, f := range found {
		where := linuxDisplayPath(filepath.Dir(f.Path))
		switch f.Status {
		case leftoverPresent:
			what := "An earlier restore was interrupted. Its files are only a partial copy of a VM."
			if f.Kind == "backup" {
				what = "An earlier backup was interrupted. This is only a partial file, not a usable backup."
			}
			rows = append(rows, linuxRow{Title: linuxLeftoverTitle(f.linuxLeftover), Detail: linuxGB(f.Bytes) + " in " + where + "\n" + what})
		case leftoverUnreachable:
			rows = append(rows, linuxRow{Title: linuxLeftoverTitle(f.linuxLeftover) + " (folder not reachable)", Detail: "Reconnect " + where + " to remove it."})
		case leftoverUnrecognized:
			rows = append(rows, linuxRow{Title: "Something else is at " + linuxDisplayPath(f.Path), Detail: "Try Omarchy did not create it in this form, so it will not touch it."})
		}
	}
	return rows
}

// linuxRemovableLeftovers are the entries that pass inspection right now.
func linuxRemovableLeftovers(found []linuxLeftoverFound) []linuxLeftoverFound {
	var out []linuxLeftoverFound
	for _, f := range found {
		if f.Status == leftoverPresent {
			out = append(out, f)
		}
	}
	return out
}

// removeLinuxLeftover deletes one recorded leftover after checking it again.
// A restore folder goes with RemoveAll, which never follows links; a backup
// file with Remove. Anything that no longer passes is left alone.
func removeLinuxLeftover(defaultDir string, l linuxLeftover) error {
	status, _ := inspectLinuxLeftover(l)
	switch status {
	case leftoverGone:
		forgetLinuxLeftover(defaultDir, l.Path)
		return nil
	case leftoverUnreachable:
		return fmt.Errorf("%s cannot be reached; reconnect it and try again", linuxDisplayPath(filepath.Dir(l.Path)))
	case leftoverUnrecognized:
		forgetLinuxLeftover(defaultDir, l.Path)
		return fmt.Errorf("%s is not something Try Omarchy made in this form, so it was left alone", linuxDisplayPath(l.Path))
	}
	var err error
	if l.Kind == "backup" {
		err = os.Remove(l.Path)
	} else {
		err = os.RemoveAll(l.Path)
	}
	if err != nil {
		return err
	}
	forgetLinuxLeftover(defaultDir, l.Path)
	return nil
}

func linuxLeftoverNotice(defaultDir string) string {
	items := linuxRemovableLeftovers(linuxFindLeftovers(defaultDir))
	switch len(items) {
	case 0:
		return ""
	case 1:
		what := "An interrupted restore"
		if items[0].Kind == "backup" {
			what = "An interrupted backup"
		}
		return what + " left " + linuxGB(items[0].Bytes) + " in " + linuxDisplayPath(filepath.Dir(items[0].Path)) + ". Backup and recovery can remove it."
	}
	var total int64
	for _, f := range items {
		total += f.Bytes
	}
	return fmt.Sprintf("%d interrupted backups or restores left %s behind. Backup and recovery can remove them.", len(items), linuxGB(total))
}

func linuxLeftoverPrompt(items []linuxLeftoverFound) string {
	var b strings.Builder
	var total int64
	for _, f := range items {
		fmt.Fprintf(&b, "%s, %s: %s\n", linuxLeftoverTitle(f.linuxLeftover), linuxGB(f.Bytes), linuxDisplayPath(f.Path))
		total += f.Bytes
	}
	fmt.Fprintf(&b, "\nThis frees about %s. Only these files are removed. Your VM, your backups and everything else in those folders stay.", linuxGB(total))
	return b.String()
}
