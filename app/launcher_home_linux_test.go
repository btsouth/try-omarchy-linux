//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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
	if customState, customDir := linuxHomeStateForDir(custom, root); customDir != custom || customState.Path != custom {
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

func stubHomeChecks(t *testing.T, kvm error, free int64) {
	t.Helper()
	oldKVM, oldFree := linuxKVMCheck, diskFreeBytes
	t.Cleanup(func() { linuxKVMCheck, diskFreeBytes = oldKVM, oldFree })
	linuxKVMCheck = func() error { return kvm }
	diskFreeBytes = func(string) (int64, error) { return free, nil }
}

func TestLinuxHomeExplainsAFirstSetupBeforeAnythingIsDownloaded(t *testing.T) {
	stubHomeChecks(t, nil, 200<<30)
	root := filepath.Join(t.TempDir(), "try-omarchy")
	state, _ := linuxHomeState(root)
	if state.Headline != "Omarchy is not set up yet." || state.Notice != "" || state.CheckAgain || state.Installed {
		t.Fatalf("first-run home: %+v", state)
	}
	if len(state.Sections) != 2 || state.Sections[0].Heading != "What setup does" {
		t.Fatalf("setup section: %+v", state.Sections)
	}
	rows := map[string]string{}
	for _, row := range state.Sections[0].Rows {
		rows[row.Title] = row.Detail
	}
	for title, want := range map[string]string{
		"Download": "About 2 GB, once. If it is interrupted, it continues where it stopped.",
		"Space":    "About 13 GB.",
		"Location": linuxDisplayPath(root),
		"Account":  "Your own username and password, or a quick-start account",
	} {
		if !strings.Contains(rows[title], want) {
			t.Fatalf("%s row lacks %q: %q", title, want, rows[title])
		}
	}
	if !strings.Contains(rows["Space"], "200 GB is free here.") || !strings.Contains(rows["Space"], "Omarchy sees a 24 GB disk") {
		t.Fatalf("space row: %q", rows["Space"])
	}
}

func TestLinuxHomeActionsFollowTheStateOfThings(t *testing.T) {
	labels := func(actions []linuxAction) []string {
		var out []string
		for _, a := range actions {
			out = append(out, a.Label+"="+a.Reply)
		}
		return out
	}
	suggested := func(actions []linuxAction) string {
		for _, a := range actions {
			if a.Suggested {
				return a.Reply
			}
		}
		return ""
	}
	for _, tc := range []struct {
		name      string
		state     linuxSetupState
		want      []string
		menu      []string
		suggested string
	}{
		{"first setup", linuxSetupState{CanAttach: true},
			[]string{"Set up Omarchy=setup", "Customize...=customize", "Settings=settings", "Close=close"},
			[]string{"About and help=about", "Use existing data folder=attach"}, "setup"},
		{"first setup without room", linuxSetupState{Setup: "customize"},
			[]string{"Choose another folder...=customize", "Settings=settings", "Close=close"},
			[]string{"About and help=about"}, "customize"},
		{"first setup without KVM", linuxSetupState{CheckAgain: true},
			[]string{"Check again=check", "Settings=settings", "Close=close"},
			[]string{"About and help=about"}, "check"},
		{"installed default VM", linuxSetupState{Installed: true, CanAttach: true, CanDelete: true},
			[]string{"Launch Omarchy=launch", "Settings=settings", "Backup and recovery=recovery", "Close=close"},
			[]string{"About and help=about", "Use existing data folder=attach", "Delete this VM...=delete-default"}, "launch"},
		{"installed external VM", linuxSetupState{Installed: true, CanAttach: true},
			[]string{"Launch Omarchy=launch", "Settings=settings", "Backup and recovery=recovery", "Close=close"},
			[]string{"About and help=about", "Use existing data folder=attach"}, "launch"},
		{"installed, KVM missing", linuxSetupState{Installed: true, CheckAgain: true},
			[]string{"Check again=check", "Settings=settings", "Backup and recovery=recovery", "Close=close"},
			[]string{"About and help=about"}, "check"},
		{"saved storage unavailable", linuxSetupState{CanForget: true, CanAttach: true},
			[]string{"Use existing data folder=attach", "Forget saved location=forget", "About and help=about", "Close=close"},
			nil, "attach"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actions, menu := linuxHomeActions(tc.state)
			if !reflect.DeepEqual(labels(actions), tc.want) || !reflect.DeepEqual(labels(menu), tc.menu) || suggested(actions) != tc.suggested {
				t.Fatalf("actions %v menu %v suggested %q", labels(actions), labels(menu), suggested(actions))
			}
			for _, a := range menu {
				if a.Reply == "delete-default" && !a.Destructive {
					t.Fatal("Delete must be marked destructive")
				}
			}
		})
	}
}

func TestLinuxHomeWarnsWhenTheDriveCannotHoldOmarchy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "try-omarchy")
	stubHomeChecks(t, nil, 5<<30)
	state, _ := linuxHomeState(root)
	if state.Notice != "Only 5 GB is free here." || !strings.Contains(state.Status, "Setup needs about 13 GB") || state.HelpURL != linuxHelpURL("space") || state.Setup != "customize" {
		t.Fatalf("setup space notice: %+v", state)
	}
	if err := os.MkdirAll(filepath.Join(root, "vm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vm", "disk.raw"), []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	stubHomeChecks(t, nil, 1<<30)
	state, _ = linuxHomeState(root)
	if state.Notice != "Only 1 GB is free on this drive." || !strings.Contains(state.Status, "stop working if the drive fills up") {
		t.Fatalf("installed low-space notice: %+v", state)
	}
	stubHomeChecks(t, nil, 50<<30)
	if state, _ = linuxHomeState(root); state.Notice != "" || state.Headline != "Omarchy is ready." {
		t.Fatalf("plenty of space must not warn: %+v", state)
	}
}

func TestLinuxHomeOffersToCheckAgainWhileKVMIsUnavailable(t *testing.T) {
	stubHomeChecks(t, &kvmError{Short: "This account cannot use KVM.", msg: "this account is not allowed to use KVM. Add it to the kvm group with: sudo usermod -aG kvm $USER. Then sign out and back in"}, 200<<30)
	state, _ := linuxHomeState(filepath.Join(t.TempDir(), "try-omarchy"))
	if !state.CheckAgain || state.Notice != "This account cannot use KVM." || state.HelpURL != linuxHelpURL("kvm") || state.Headline != "Omarchy cannot start yet." {
		t.Fatalf("KVM notice: %+v", state)
	}
	if !strings.HasPrefix(state.Status, "This account is not allowed to use KVM.") || !strings.Contains(state.Status, "sudo usermod -aG kvm $USER") || !strings.HasSuffix(state.Status, "sign out and back in.") {
		t.Fatalf("KVM guidance must say what to do: %q", state.Status)
	}
	// An unexpected failure still gets a banner, not a blank state.
	stubHomeChecks(t, errors.New("odd"), 200<<30)
	if state, _ = linuxHomeState(filepath.Join(t.TempDir(), "try-omarchy")); state.Notice != "KVM is not available." || !state.CheckAgain {
		t.Fatalf("unknown KVM failure: %+v", state)
	}
}

func TestLinuxHomeSaysWhereAnUnavailableVMWasAndThatNothingWasDeleted(t *testing.T) {
	stubHomeChecks(t, nil, 200<<30)
	t.Setenv("HOME", "/home/ana")
	root := t.TempDir()
	defaultDir := filepath.Join(root, "data", "try-omarchy")
	missing := filepath.Join(root, "gone", "try-omarchy")
	if err := saveDataLocationPointer(defaultDir, missing); err != nil {
		t.Fatal(err)
	}
	state, dir := linuxHomeState(defaultDir)
	if dir != "" || !state.CanForget || state.Headline != "Your Omarchy folder is not available." || !strings.Contains(state.Status, "Nothing has been deleted") {
		t.Fatalf("unavailable home: %+v %q", state, dir)
	}
	if strings.Contains(state.Status+state.Detail, "stat ") || state.Detail != "" {
		t.Fatalf("the home must not show raw filesystem errors: %+v", state)
	}
	// After the grant is gone, the recorded real path still names the folder.
	hint, err := json.Marshal(linuxLocationHint{Path: missing, Host: "/home/ana/Drives/Omarchy/try-omarchy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(linuxLocationHintPath(defaultDir)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linuxLocationHintPath(defaultDir), hint, 0o600); err != nil {
		t.Fatal(err)
	}
	if state, _ = linuxHomeState(defaultDir); !strings.Contains(state.Status, "~/Drives/Omarchy/try-omarchy") {
		t.Fatalf("home must name the recorded folder: %q", state.Status)
	}
}

func TestLinuxLocationHintRecordsOnlyPortalFolders(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "data", "try-omarchy")
	old := linuxDocumentHostPath
	defer func() { linuxDocumentHostPath = old }()
	linuxDocumentHostPath = func(id string) (string, error) { return "/mnt/omarchy", nil }
	doc := filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "doc", "abc", "omarchy", "try-omarchy")
	noteLinuxLocationHint(defaultDir, doc)
	if got := linuxLocationHostPath(defaultDir, doc); got != "/mnt/omarchy/try-omarchy" {
		t.Fatalf("recorded host path: %q", got)
	}
	// A pointer that no longer matches the record is not described by it.
	linuxDocumentHostPath = func(id string) (string, error) { return "", errors.New("gone") }
	other := filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "doc", "zzz", "elsewhere")
	if got := linuxLocationHostPath(defaultDir, other); got != other {
		t.Fatalf("hint leaked onto another folder: %q", got)
	}
	// A folder that is already a host path drops the stale record.
	noteLinuxLocationHint(defaultDir, "/mnt/plain/try-omarchy")
	if _, err := os.Stat(linuxLocationHintPath(defaultDir)); !os.IsNotExist(err) {
		t.Fatalf("stale hint kept: %v", err)
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

// The home loop itself must send the buttons: the window draws only what it is
// given, so a state without them is a home nobody can leave except by closing.
func TestTheHomeLoopSendsTheButtonsItDecided(t *testing.T) {
	record := filepath.Join(t.TempDir(), "states.jsonl")
	t.Setenv("TRY_OMARCHY_STATE_RECORD", record)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	defaultDir := filepath.Join(t.TempDir(), "try-omarchy")
	if runLinuxHome(linuxScriptedWindow(t, "close"), defaultDir, "", false) {
		t.Fatal("closing the home must not launch")
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var sent linuxSetupState
	if err := json.Unmarshal(bytes.SplitN(data, []byte("\n"), 2)[0], &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Prompt != "home" || len(sent.Actions) == 0 || len(sent.Menu) == 0 {
		t.Fatalf("home sent without its buttons: %+v", sent)
	}
	bare := sent
	bare.Actions, bare.Menu, bare.Request = nil, nil, 0
	actions, menu := linuxHomeActions(bare)
	if !reflect.DeepEqual(sent.Actions, actions) || !reflect.DeepEqual(sent.Menu, menu) {
		t.Fatalf("sent %+v / %+v, decided %+v / %+v", sent.Actions, sent.Menu, actions, menu)
	}
}

func TestLinuxResourceSummaryIsAnEstimateFromHostSizing(t *testing.T) {
	text := linuxAutomaticResourcesSummary(hostResources{LogicalCPUs: 12, TotalMiB: 32768, AvailableMiB: 16384})
	for _, want := range []string{"8 processors", "6.0–8.0 GiB", "estimate", "Checked again at launch"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestLinuxProfilesRetainManualValues(t *testing.T) {
	dir := t.TempDir()
	if err := saveSettings(settingsPath(dir), settings{MemoryMiB: 2048, CPUs: 1}); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{resourceBalanced, resourceManual} {
		t.Setenv("TRY_OMARCHY_PROFILE", profile)
		w := setupWindowFixture(t, "settings-profile")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result := showLinuxSettingsInWindow(ctx, w, dir, false)
		cancel()
		if !strings.Contains(result, "Settings saved") {
			t.Fatalf("save: %q", result)
		}
		prefs, err := loadSettings(settingsPath(dir))
		if err != nil || prefs.MemoryMiB != 2048 || prefs.CPUs != 1 {
			t.Fatalf("manual values changed: %+v %v", prefs, err)
		}
		resources, err := loadResourcePreferences(dir)
		if err != nil || resources.Profile != profile {
			t.Fatalf("profile: %+v %v", resources, err)
		}
	}
}

func TestLinuxMaximumResourceFailureNamesLinux(t *testing.T) {
	_, err := planGuestResources(resourceMaximum, hostResources{LogicalCPUs: 4, TotalMiB: 8192, AvailableMiB: 1024}, false, 0, 0, false, false)
	if err == nil {
		t.Fatal("expected insufficient memory")
	}
	text := linuxResourceErrorText(err)
	if strings.Contains(text, "Windows") || !strings.Contains(text, "close some Linux apps or choose Balanced") {
		t.Fatalf("wrong host guidance: %s", text)
	}
}
