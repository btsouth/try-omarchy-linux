//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func desktopFile(id, body string) hostDesktopFile {
	return hostDesktopFile{id: id, path: "/usr/share/applications/" + id, data: []byte("[Desktop Entry]\nType=Application\n" + body)}
}

func TestDesktopExecArguments(t *testing.T) {
	for _, tc := range []struct {
		exec string
		want []string
	}{
		{"firefox %u", []string{"firefox"}},
		{"/usr/bin/flatpak run --branch=stable --command=calc org.gnome.Calculator @@u %U @@", []string{"/usr/bin/flatpak", "run", "--branch=stable", "--command=calc", "org.gnome.Calculator", "@@u", "@@"}},
		{`"/opt/My App/run" --name "two words" %F`, []string{"/opt/My App/run", "--name", "two words"}},
		{`sh -c "echo \\"$HOME\\" 100%%"`, []string{"sh", "-c", `echo "$HOME" 100%`}},
		{"viewer --title %c --entry %k %i", []string{"viewer", "--title", "Viewer App", "--entry", "/usr/share/applications/viewer.desktop"}},
		{`tool ""`, []string{"tool", ""}},
		{`path\\sname`, []string{`path\sname`}},
	} {
		got, err := desktopExecArguments(tc.exec, "Viewer App", "/usr/share/applications/viewer.desktop")
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %q, %v; want %q", tc.exec, got, err, tc.want)
		}
	}
	for _, exec := range []string{"", "%f", `"unfinished`, "app %x", "app %", `"app\`} {
		if got, err := desktopExecArguments(exec, "App", "/a.desktop"); err == nil {
			t.Errorf("%q: accepted as %q", exec, got)
		}
	}
}

func TestLinuxHostAppsFollowsTheDesktopLauncher(t *testing.T) {
	files := []hostDesktopFile{
		desktopFile("zed.desktop", "Name=Zed\nName[de]=Zett\nExec=zed %F\n\n[Desktop Action new]\nName=New Window\nExec=zed --new\n"),
		desktopFile("hidden.desktop", "Name=Hidden\nExec=hidden\nNoDisplay=true\n"),
		desktopFile("removed.desktop", "Name=Removed\nExec=removed\nHidden=true\n"),
		// The same ID later in the search path stays hidden behind the first one.
		desktopFile("removed.desktop", "Name=Removed\nExec=removed\n"),
		desktopFile("terminal.desktop", "Name=Terminal Tool\nExec=tool\nTerminal = true \n"),
		desktopFile("link.desktop", "Name=Link\nExec=link\n"),
		desktopFile("kde-apps.desktop", "Name=apps\nExec=apps\n"),
		desktopFile("noexec.desktop", "Name=Bus Only\nDBusActivatable=true\n"),
		desktopFile(linuxAppID+".desktop", "Name=Try Omarchy\nExec=try-omarchy\n"),
		desktopFile("control.desktop", "Name=Bad\\nName\nExec=bad\n"),
		desktopFile("../escape.desktop", "Name=Escape\nExec=escape\n"),
		{id: "service.desktop", path: "/usr/share/applications/service.desktop", data: []byte("[Desktop Entry]\nType=Link\nName=Service\nExec=service\n")},
	}
	var got []string
	for _, app := range linuxHostApps(files) {
		got = append(got, app.ID+"="+app.Name+"="+strings.Join(app.exec, " "))
	}
	want := []string{"kde-apps.desktop=apps=apps", "link.desktop=Link=link", "zed.desktop=Zed=zed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHostDesktopDirectoriesAndIDs(t *testing.T) {
	env := map[string]string{"XDG_DATA_HOME": "/home/u/data", "XDG_DATA_DIRS": "/opt/share:relative:/usr/share/:/opt/share"}
	got := hostDesktopDirectories(func(key string) string { return env[key] }, "/home/u")
	want := []string{"/home/u/data/applications", "/opt/share/applications", "/usr/share/applications", "/home/u/data/flatpak/exports/share/applications", "/var/lib/flatpak/exports/share/applications"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = hostDesktopDirectories(func(string) string { return "" }, "/home/u")
	want = []string{"/home/u/.local/share/applications", "/usr/local/share/applications", "/usr/share/applications", "/home/u/.local/share/flatpak/exports/share/applications", "/var/lib/flatpak/exports/share/applications"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults: got %q, want %q", got, want)
	}
	if id := hostDesktopID("/usr/share/applications", "/usr/share/applications/kde/okular.desktop"); id != "kde-okular.desktop" {
		t.Fatalf("subdirectory ID %q", id)
	}
	for id, valid := range map[string]bool{"a.desktop": true, ".desktop": false, "a/b.desktop": false, "a b.desktop": false, "a.txt": false, ".hidden.desktop": false, strings.Repeat("a", 250) + ".desktop": false} {
		if validHostDesktopID(id) != valid {
			t.Errorf("%q valid=%v", id, !valid)
		}
	}
}

func TestReadNativeHostDesktopFilesHonorsPrecedenceAndLimits(t *testing.T) {
	if inFlatpak() {
		t.Skip("reads this sandbox's own directories")
	}
	home := t.TempDir()
	system := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_DATA_DIRS", system)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".local/share/applications/editor.desktop"), "[Desktop Entry]\nType=Application\nName=My Editor\nExec=my-editor\n")
	write(filepath.Join(system, "applications/editor.desktop"), "[Desktop Entry]\nType=Application\nName=Editor\nExec=editor\n")
	write(filepath.Join(system, "applications/kde/viewer.desktop"), "[Desktop Entry]\nType=Application\nName=Viewer\nExec=viewer %f\n")
	write(filepath.Join(system, "applications/notes.txt"), "not a desktop file")
	write(filepath.Join(system, "applications/huge.desktop"), "[Desktop Entry]\nType=Application\nName=Huge\nExec=huge\n"+strings.Repeat("#", maximumHostDesktopFileBytes))
	if err := os.Symlink(filepath.Join(system, "applications/editor.desktop"), filepath.Join(system, "applications/linked.desktop")); err != nil {
		t.Fatal(err)
	}
	files, err := readLinuxHostDesktopFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, app := range linuxHostApps(files) {
		got = append(got, app.ID+"="+strings.Join(app.exec, " "))
	}
	want := []string{"linked.desktop=editor", "editor.desktop=my-editor", "kde-viewer.desktop=viewer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// exec copies a command's output with io.Copy, which uses ReadFrom when the
// writer has one. The limit must hold on that path too.
func TestLimitedBufferStopsACopyAtItsLimit(t *testing.T) {
	out := &limitedBuffer{limit: 1000}
	if _, ok := any(out).(io.ReaderFrom); ok {
		t.Fatal("a ReadFrom method would let a copy skip the limit")
	}
	if n, err := io.Copy(out, strings.NewReader(strings.Repeat("a", 5000))); err == nil || out.data.Len() > 1000 {
		t.Fatalf("copied %d bytes past the limit, kept %d, %v", n, out.data.Len(), err)
	}
	cmd := exec.Command("sh", "-c", "head -c 5000 /dev/zero")
	cmd.Stdout = &limitedBuffer{limit: 1000}
	if err := cmd.Run(); err == nil {
		t.Fatal("a command with too much output succeeded")
	}
}

func TestParseHostDesktopStream(t *testing.T) {
	entry := func(path, body string) string { return fmt.Sprintf("%s\x00%d\x00%s", path, len(body), body) }
	body := "[Desktop Entry]\nType=Application\nName=Calc\nExec=calc\n"
	stream := entry("/usr/share/applications/calc.desktop", body) +
		entry("/var/lib/flatpak/exports/share/applications/org.example.App.desktop", "[Desktop Entry]\nType=Application\nName=App\nExec=flatpak run org.example.App\x00binary\n") +
		entry("relative/applications/bad.desktop", body) +
		entry("/usr/share/other/bad.desktop", body)
	files := parseHostDesktopStream([]byte(stream))
	if len(files) != 2 || files[0].id != "calc.desktop" || files[1].id != "org.example.App.desktop" || string(files[0].data) != body {
		t.Fatalf("files %+v", files)
	}
	// A truncated or oversized record ends the list without inventing a file.
	for _, bad := range []string{"/usr/share/applications/a.desktop", "/usr/share/applications/a.desktop\x00100\x00short", fmt.Sprintf("/usr/share/applications/a.desktop\x00%d\x00", maximumHostDesktopFileBytes+1), "/usr/share/applications/a.desktop\x00-1\x00"} {
		if files := parseHostDesktopStream([]byte(bad)); len(files) != 0 {
			t.Errorf("%q: %+v", bad, files)
		}
	}
}

func TestApproveLinuxHostApps(t *testing.T) {
	installed := []linuxHostApp{{ID: "calc.desktop", Name: "Calculator"}, {ID: "editor.desktop", Name: "Editor"}}
	prefs := approvedAppPreferences{SchemaVersion: 1}
	next, err := approveLinuxHostApps(prefs, []string{"editor.desktop", "calc.desktop"}, installed)
	if err != nil || len(next.Apps) != 2 || next.Apps[0].Path != "editor.desktop" || next.Apps[0].Name != "Editor" || !validApprovedAppID(next.Apps[0].ID) || next.Apps[0].ID == next.Apps[1].ID {
		t.Fatalf("approved %+v, %v", next, err)
	}
	// Keeping an approval keeps its ID, follows a rename, and survives the app being uninstalled.
	kept, err := approveLinuxHostApps(next, []string{"calc.desktop", "editor.desktop"}, []linuxHostApp{{ID: "calc.desktop", Name: "Calc"}})
	if err != nil || len(kept.Apps) != 2 || kept.Apps[0].ID != next.Apps[1].ID || kept.Apps[0].Name != "Calc" || kept.Apps[1] != next.Apps[0] {
		t.Fatalf("kept %+v, %v", kept, err)
	}
	if sameApprovedApps(next, kept) || !sameApprovedApps(next, next) {
		t.Fatal("approved list comparison")
	}
	for name, chosen := range map[string][]string{
		"not installed": {"missing.desktop"},
		"repeated":      {"calc.desktop", "calc.desktop"},
		"a path":        {"/usr/share/applications/calc.desktop"},
		"too many":      strings.Split(strings.Repeat("calc.desktop,", maximumApprovedApps+1), ","),
	} {
		if got, err := approveLinuxHostApps(prefs, chosen, installed); err == nil {
			t.Errorf("%s: accepted %+v", name, got)
		}
	}
	shown := approvedLinuxHostApps(next)
	if len(shown) != 2 || shown[0].ID != "editor.desktop" || shown[0].Name != "Editor" {
		t.Fatalf("shown %+v", shown)
	}
}

func TestLaunchApprovedLinuxAppRunsOnlyTheApprovedEntry(t *testing.T) {
	dir := t.TempDir()
	files := []hostDesktopFile{desktopFile("calc.desktop", "Name=Calculator\nExec=calc --mode \"basic view\" %U\n"), desktopFile("other.desktop", "Name=Other\nExec=other\n")}
	oldRead, oldStart := readHostDesktopFiles, startHostCommand
	t.Cleanup(func() { readHostDesktopFiles, startHostCommand = oldRead, oldStart })
	readHostDesktopFiles = func(context.Context) ([]hostDesktopFile, error) { return files, nil }
	var started [][]string
	startHostCommand = func(args []string) error { started = append(started, args); return nil }

	prefs, err := approveLinuxHostApps(approvedAppPreferences{SchemaVersion: 1}, []string{"calc.desktop"}, linuxHostApps(files))
	if err != nil {
		t.Fatal(err)
	}
	if err := saveApprovedWindowsApps(dir, prefs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "approved-host-apps.json")); err != nil {
		t.Fatal(err)
	}
	if err := launchApprovedLinuxApp(dir, prefs.Apps[0].ID); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"calc", "--mode", "basic view"}}; !reflect.DeepEqual(started, want) {
		t.Fatalf("started %q, want %q", started, want)
	}
	for name, id := range map[string]string{"unknown": strings.Repeat("0", 32), "desktop ID": "other.desktop", "path": "../calc"} {
		if err := launchApprovedLinuxApp(dir, id); err == nil {
			t.Errorf("%s: launched", name)
		}
	}
	// An approved app that was uninstalled, or now needs a terminal, does not start.
	files = []hostDesktopFile{desktopFile("calc.desktop", "Name=Calculator\nExec=calc\nTerminal=true\n")}
	if err := launchApprovedLinuxApp(dir, prefs.Apps[0].ID); err == nil {
		t.Error("launched an entry the launcher no longer offers")
	}
	if len(started) != 1 {
		t.Fatalf("started %q", started)
	}
}

func TestStartLinuxHostCommandDetachesTheApp(t *testing.T) {
	if inFlatpak() {
		t.Skip("would start the command outside this sandbox")
	}
	marker := filepath.Join(t.TempDir(), "started")
	// Field 6 of /proc/PID/stat is the session; a session leader's is its own PID.
	script := `arg=$1; set -- $(cat /proc/$$/stat); test "$6" = "$1" && leads=leader; printf '%s %s %s' "$arg" "$PWD" "$leads" > "$0.part" && mv "$0.part" "$0"`
	if err := startLinuxHostCommand([]string{"sh", "-c", script, marker, "two words"}); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		data, err := os.ReadFile(marker)
		if err == nil {
			if string(data) != "two words "+home+" leader" {
				t.Fatalf("command ran as %q, want its argument, the home folder and its own session", data)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not run")
		}
	}
	if err := startLinuxHostCommand([]string{filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("missing command started")
	}
}

// An older guest agent would label these entries as Windows apps, so it gets
// no list. A launch request still needs an approval.
func TestGuestAgentHoldsAppsBackFromOlderGuests(t *testing.T) {
	dir := t.TempDir()
	id := strings.Repeat("a", 32)
	if err := saveApprovedWindowsApps(dir, approvedAppPreferences{Apps: []approvedWindowsApp{{ID: id, Name: "Calculator", Path: "calc.desktop"}}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		hello string
		apps  bool
	}{{"hello 4\n", false}, {"hello 5\n", true}, {"hello later\n", false}} {
		a := newGuestAgent()
		a.batteryLine = nil
		a.appsDir = dir
		a.appsMinVersion = linuxHostAppsAgentVersion
		host, guest := net.Pipe()
		go func() {
			guest.Write([]byte(tc.hello))
		}()
		go a.serve(host)
		reader := bufio.NewReader(guest)
		guest.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		sawApps := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			if strings.HasPrefix(line, "apps ") {
				sawApps = true
				break
			}
		}
		guest.Close()
		if sawApps != tc.apps {
			t.Errorf("%q: app list sent=%v, want %v", strings.TrimSpace(tc.hello), sawApps, tc.apps)
		}
	}
}

// This helper speaks the setup pipe protocol without loading GTK. It records
// the apps Settings offered, answers with the chosen desktop IDs, and cancels
// when Settings comes back with a problem.
func TestLinuxHostAppsSettingsHelper(t *testing.T) {
	dir := os.Getenv("TRY_OMARCHY_HOST_APPS_DIR")
	if dir == "" {
		return
	}
	fail := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	fmt.Println(`{"event":"ready"}`)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(nil, 1<<20)
	count := 0
	for scanner.Scan() {
		var state linuxSetupState
		fail(json.Unmarshal(scanner.Bytes(), &state))
		if state.Prompt != "settings" {
			continue
		}
		count++
		value := "cancel"
		if count == 1 {
			offered, err := json.Marshal(map[string][]linuxHostApp{"approved": state.Settings.HostApps, "choices": state.Settings.HostAppChoices})
			fail(err)
			fail(os.WriteFile(filepath.Join(dir, "offered"), offered, 0o600))
			form := *state.Settings
			form.HostApps = nil
			for _, id := range strings.Split(os.Getenv("TRY_OMARCHY_HOST_APPS_CHOSEN"), ",") {
				// A window could send any name; the launcher must use its own.
				form.HostApps = append(form.HostApps, linuxHostApp{ID: id, Name: "Name from the window"})
			}
			data, err := json.Marshal(form)
			fail(err)
			value = string(data)
		} else {
			fail(os.WriteFile(filepath.Join(dir, "problem"), []byte(state.Status), 0o600))
		}
		fail(json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: value}))
	}
}

func TestLinuxSettingsSavesOnlyInstalledHostApps(t *testing.T) {
	files := []hostDesktopFile{desktopFile("calc.desktop", "Name=Calculator\nExec=calc\n"), desktopFile("editor.desktop", "Name=Editor\nExec=editor %F\n"), desktopFile("shell.desktop", "Name=Shell\nExec=shell\nTerminal=true\n")}
	oldRead := readHostDesktopFiles
	t.Cleanup(func() { readHostDesktopFiles = oldRead })
	readHostDesktopFiles = func(context.Context) ([]hostDesktopFile, error) { return files, nil }
	calcID := strings.Repeat("c", 32)
	for _, tc := range []struct {
		name, chosen, problem string
		want                  []string
	}{
		{"adds an installed app and keeps the earlier approval", "editor.desktop,calc.desktop", "", []string{"editor.desktop=Editor", "calc.desktop=Calculator"}},
		{"refuses an app this computer does not offer", "calc.desktop,shell.desktop", "no longer installed", []string{"calc.desktop=Calculator"}},
		{"refuses a path", "/usr/bin/sh", "invalid app selection", []string{"calc.desktop=Calculator"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := saveApprovedWindowsApps(dir, approvedAppPreferences{Apps: []approvedWindowsApp{{ID: calcID, Name: "Calculator", Path: "calc.desktop"}}}); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxHostAppsSettingsHelper$")
			cmd.Env = append(os.Environ(), "TRY_OMARCHY_HOST_APPS_DIR="+dir, "TRY_OMARCHY_HOST_APPS_CHOSEN="+tc.chosen)
			w := launchLinuxWindow(cmd, func() {})
			if w == nil {
				t.Fatal("settings helper did not start")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			feedback := showLinuxSettingsInWindow(ctx, w, dir, false)
			cancel()
			w.stop()
			offered, err := os.ReadFile(filepath.Join(dir, "offered"))
			if err != nil || string(offered) != `{"approved":[{"id":"calc.desktop","name":"Calculator"}],"choices":[{"id":"calc.desktop","name":"Calculator"},{"id":"editor.desktop","name":"Editor"}]}` {
				t.Fatalf("offered %s, %v", offered, err)
			}
			problem, _ := os.ReadFile(filepath.Join(dir, "problem"))
			if tc.problem == "" && (len(problem) != 0 || !strings.Contains(feedback, "Settings saved")) {
				t.Fatalf("save: %q, problem %q", feedback, problem)
			}
			if tc.problem != "" && !strings.Contains(string(problem), tc.problem) {
				t.Fatalf("problem %q, want %q", problem, tc.problem)
			}
			saved, err := loadApprovedWindowsApps(dir)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, app := range saved.Apps {
				got = append(got, app.Path+"="+app.Name)
				if app.Path == "calc.desktop" && app.ID != calcID {
					t.Error("an approval kept across Save changed its ID")
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("saved %q, want %q", got, tc.want)
			}
		})
	}
}
