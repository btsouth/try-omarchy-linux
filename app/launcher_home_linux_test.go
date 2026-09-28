//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLinuxHomeIsReadOnlyAndDetectsExistingDisk(t *testing.T) {
	root := filepath.Join(t.TempDir(), "try-omarchy")
	state, dir := linuxHomeState(root)
	if dir != root || state.Prompt != "home" || state.Path != root {
		t.Fatalf("new home: %+v %q", state, dir)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("idle home created storage: %v", err)
	}
	custom := filepath.Join(t.TempDir(), "custom")
	if customState, customDir := linuxHomeStateForDir(custom); customDir != custom || customState.Path != custom {
		t.Fatalf("explicit location: %+v %q", customState, customDir)
	}
	if err := os.MkdirAll(filepath.Join(root, "vm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vm", "disk.raw"), []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, _ = linuxHomeState(root)
	if state.Status == "" || state.Path != root {
		t.Fatalf("existing home: %+v", state)
	}
}

func TestLinuxFreshSettingsShareBeforeStorageChoice(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "custom"}[custom], func(t *testing.T) {
			root := t.TempDir()
			defaultDir := filepath.Join(root, "default", "try-omarchy")
			share := filepath.Join(root, "exchange")
			if err := os.Mkdir(share, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(defaultDir); !os.IsNotExist(err) {
				t.Fatalf("home created data: %v", err)
			}
			configureSetupCancellation(false)
			cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxSetupHelperProcess$")
			cmd.Env = append(os.Environ(), "TRY_OMARCHY_SETUP_TEST=settings-share-save", "TRY_OMARCHY_SETUP_SHARE="+share)
			w := launchLinuxSetupWindow(cmd)
			if w == nil {
				t.Fatal("settings fixture did not start")
			}
			feedback := showLinuxSettingsInWindow(context.Background(), w, defaultDir, false)
			w.stop()
			if feedback == "" {
				t.Fatal("first settings save failed")
			}
			prefs, err := loadSettings(settingsPath(defaultDir))
			if err != nil || prefs.Share != share || prefs.ShareDisabled {
				t.Fatalf("saved share: %+v %v", prefs, err)
			}
			selected := defaultDir
			if custom {
				selected = filepath.Join(root, "custom", "try-omarchy")
			}
			got, proceed, err := resolveLinuxDataDirectory(defaultDir, defaultDir, false, func(string) (string, bool, error) {
				return selected, true, nil
			})
			if err != nil || !proceed || got != selected {
				t.Fatalf("storage choice: %q %t %v", got, proceed, err)
			}
			prefs, err = loadSettings(settingsPath(selected))
			if err != nil || prefs.Share != share || prefs.ShareDisabled {
				t.Fatalf("share lost after location: %+v %v", prefs, err)
			}
			if _, err := validateLinuxSharedFolder(prefs.Share, selected); err != nil {
				t.Fatalf("share invalid after location: %v", err)
			}
		})
	}
}

func TestLinuxFreshSettingsCancelKeepsSharingDisabled(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "new-install", "try-omarchy")
	w := setupWindowFixture(t, "settings-cancel")
	if feedback := showLinuxSettingsInWindow(context.Background(), w, dataDir, false); feedback != "" {
		t.Fatalf("cancel returned %q", feedback)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("cancel created data or enabled sharing: %v", err)
	}
}

func TestLinuxDisplayAndMicrophoneSettingsSurviveRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "try-omarchy")
	before := desktopPreferences{CameraDisabled: true, AutomaticUpdatesDisabled: true}
	if err := saveDesktopPreferences(dir, before); err != nil {
		t.Fatal(err)
	}
	w := setupWindowFixture(t, "settings-phase9-save")
	if feedback := showLinuxSettingsInWindow(context.Background(), w, dir, false); feedback == "" {
		t.Fatal("display and microphone save failed")
	}
	w.stop()
	saved, err := loadSettings(settingsPath(dir))
	if err != nil || !saved.Fullscreen {
		t.Fatalf("fullscreen setting: %+v %v", saved, err)
	}
	desktop, err := loadDesktopPreferences(dir)
	if err != nil || !desktop.MicrophoneDisabled || !desktop.CameraDisabled || !desktop.AutomaticUpdatesDisabled {
		t.Fatalf("desktop preferences: %+v %v", desktop, err)
	}
	reloaded := setupWindowFixture(t, "settings-phase9-reload")
	if feedback := showLinuxSettingsInWindow(context.Background(), reloaded, dir, false); feedback != "" {
		t.Fatalf("cancel after restart returned %q", feedback)
	}
	selected := filepath.Join(filepath.Dir(dir), "custom", "try-omarchy")
	got, proceed, err := resolveLinuxDataDirectory(dir, dir, false, func(string) (string, bool, error) {
		return selected, true, nil
	})
	if err != nil || !proceed || got != selected {
		t.Fatalf("custom storage choice: %q %t %v", got, proceed, err)
	}
	selectedSettings, err := loadSettings(settingsPath(selected))
	if err != nil || !selectedSettings.Fullscreen {
		t.Fatalf("fullscreen after storage choice: %+v %v", selectedSettings, err)
	}
	selectedDesktop, err := loadDesktopPreferences(selected)
	if err != nil || !selectedDesktop.MicrophoneDisabled || !selectedDesktop.CameraDisabled || !selectedDesktop.AutomaticUpdatesDisabled {
		t.Fatalf("desktop preferences after storage choice: %+v %v", selectedDesktop, err)
	}
}

func TestLinuxFreshSettingsRejectStorageInsideSavedShare(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default", "try-omarchy")
	share := filepath.Join(root, "exchange")
	if err := os.Mkdir(share, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := saveSettings(settingsPath(defaultDir), settings{Share: share, SharedFolderPrompted: true}); err != nil {
		t.Fatal(err)
	}
	unsafe := filepath.Join(share, "try-omarchy")
	if _, proceed, err := resolveLinuxDataDirectory(defaultDir, defaultDir, false, func(string) (string, bool, error) {
		return unsafe, true, nil
	}); err == nil || proceed {
		t.Fatalf("storage inside saved share accepted: proceed=%t err=%v", proceed, err)
	}
	if _, err := os.Stat(unsafe); !os.IsNotExist(err) {
		t.Fatalf("unsafe storage was created: %v", err)
	}
}

func TestLinuxDirectStartFlags(t *testing.T) {
	for _, tc := range []struct {
		flags map[string]bool
		want  bool
	}{
		{map[string]bool{}, false},
		{map[string]bool{"launcher": true}, false},
		{map[string]bool{"dir": true}, true},
		{map[string]bool{"memory": true}, true},
		{map[string]bool{"release": true}, true},
	} {
		if got := linuxDirectStart(tc.flags); got != tc.want {
			t.Fatalf("flags %v: got %t, want %t", tc.flags, got, tc.want)
		}
	}
}

func TestLinuxPrebootSettingsSaveAndCancel(t *testing.T) {
	for _, mode := range []string{"settings-cancel", "settings-save", "settings-running"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			original := settings{MemoryMiB: 2048, CPUs: 0, Render: "gpu", Share: "/saved/folder", ShareDisabled: true, SharedFolderPrompted: true, Fullscreen: true, Displays: 2, Forwards: []string{"tcp:2222:22"}, SSHKey: "/saved/key"}
			if err := saveSettings(settingsPath(dir), original); err != nil {
				t.Fatal(err)
			}
			original, _ = loadSettings(settingsPath(dir))
			w := setupWindowFixture(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			feedback := showLinuxSettingsInWindow(ctx, w, dir, mode == "settings-running")
			if mode == "settings-running" && feedback == "" {
				t.Fatal("running VM did not receive restart feedback")
			}
			got, err := loadSettings(settingsPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			want := original
			if mode != "settings-cancel" {
				want.MemoryMiB, want.CPUs, want.Render = 4096, 1, "cpu"
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("settings changed unexpectedly: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestLinuxPrebootSettingsKeepLocationChoice(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default", "try-omarchy")
	selectedDir := filepath.Join(root, "chosen", "try-omarchy")
	if err := saveSettings(settingsPath(defaultDir), settings{MemoryMiB: 4096, Render: "cpu"}); err != nil {
		t.Fatal(err)
	}
	selected, proceed, err := resolveLinuxDataDirectory(defaultDir, defaultDir, false, func(string) (string, bool, error) {
		return selectedDir, true, nil
	})
	if err != nil || !proceed || selected != selectedDir {
		t.Fatalf("location: %q %t %v", selected, proceed, err)
	}
	prefs, err := loadSettings(settingsPath(selectedDir))
	if err != nil || prefs.MemoryMiB != 4096 || prefs.Render != "cpu" {
		t.Fatalf("preferences lost: %+v %v", prefs, err)
	}
	if pointer, found, err := loadDataLocationPointer(defaultDir); err != nil || !found || pointer != selectedDir {
		t.Fatalf("pointer: %q %t %v", pointer, found, err)
	}
}
