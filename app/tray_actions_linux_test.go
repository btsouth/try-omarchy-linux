//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestLinuxTrayOffersSessionActions(t *testing.T) {
	drain := func(ch chan struct{}) {
		for len(ch) > 0 {
			<-ch
		}
	}
	for _, ch := range []chan struct{}{linuxShareRequests, linuxDiagnosticsRequests, linuxHelpRequests, linuxResumeRequests} {
		drain(ch)
		defer drain(ch)
	}
	menu := &linuxTrayMenu{}
	for _, id := range []int32{linuxTrayShare, linuxTrayDiagnostics, linuxTrayHelp, linuxTrayResume} {
		menu.Event(id, "hovered", dbus.MakeVariant(""), 0)
		menu.Event(id, "clicked", dbus.MakeVariant(""), 0)
		menu.Event(id, "clicked", dbus.MakeVariant(""), 0)
	}
	if len(linuxShareRequests) != 1 || len(linuxDiagnosticsRequests) != 1 || len(linuxHelpRequests) != 1 || len(linuxResumeRequests) != 1 {
		t.Fatalf("queued share=%d diagnostics=%d help=%d", len(linuxShareRequests), len(linuxDiagnosticsRequests), len(linuxHelpRequests))
	}
	var labels []string
	separators := 0
	for _, child := range linuxTrayLayout(0).Children {
		node := child.Value().(linuxMenuNode)
		if label, ok := node.Properties["label"]; ok {
			labels = append(labels, label.Value().(string))
		} else if node.Properties["type"].Value() == "separator" {
			separators++
		}
	}
	want := []string{"Open shared folder", "Settings...", "USB devices", "Reclaim disk space...", "Create diagnostics", "Help and shortcuts...", "Resume Omarchy", "Shut down Omarchy..."}
	if len(labels) != len(want) || separators != 2 {
		t.Fatalf("labels %q, %d separators", labels, separators)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("labels %q", labels)
		}
	}
	props, err := menu.GetGroupProperties(nil, nil)
	if err != nil || len(props) != len(linuxTrayOrder)+1 {
		t.Fatalf("group properties %d %v", len(props), err)
	}
}

func TestLinuxTrayOpensTheSessionSharedFolder(t *testing.T) {
	before := linuxOpenFolder
	defer func() { linuxOpenFolder = before }()
	var opened []string
	linuxOpenFolder = func(path string) error { opened = append(opened, path); return nil }
	openLinuxSharedFolder("")
	if len(opened) != 0 {
		t.Fatal("opened a folder without a share")
	}
	share := t.TempDir()
	openLinuxSharedFolder(share)
	if len(opened) != 1 || opened[0] != share {
		t.Fatalf("opened %q", opened)
	}
	linuxOpenFolder = func(string) error { return errors.New("no file manager") }
	openLinuxSharedFolder(share)
}

func TestLinuxTrayDiagnosticsWritesABundle(t *testing.T) {
	dir := t.TempDir()
	createLinuxDiagnostics(dir)
	matches, _ := filepath.Glob(dir + "/diagnostics/try-omarchy-diagnostics-*.zip")
	if len(matches) != 1 {
		t.Fatalf("bundles: %v", matches)
	}
}

// The portal helper opens a real folder when a test desktop asks it to.
func TestLinuxOpenFolderThroughPortalHelper(t *testing.T) {
	dir := os.Getenv("TRYOMARCHY_PORTAL_OPEN_DIR")
	if dir == "" {
		return
	}
	if err := openLinuxFolderThroughPortal(dir); err != nil {
		t.Fatal(err)
	}
	if err := openLinuxFolderThroughPortal(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("opened a missing folder")
	}
}
