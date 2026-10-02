//go:build linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Approved host apps on Linux are desktop entries. Settings stores the desktop
// file ID in the shared approved-apps file; the guest only ever sees the random
// approval ID and the name. A launch looks the entry up again and runs its Exec
// line on this computer, outside the Flatpak sandbox when there is one.

const approvedAppsFilename = "approved-host-apps.json"

// The first guest agent that labels approved apps for a Linux host.
const linuxHostAppsAgentVersion = 5

const (
	maximumHostDesktopFiles     = 2000
	maximumHostDesktopFileBytes = 256 << 10
	maximumHostDesktopBytes     = 16 << 20
)

type linuxHostApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	file string
	exec []string
}

// hostDesktopFile is one desktop file as found on this computer. Earlier
// directories take precedence, as the desktop entry specification says.
type hostDesktopFile struct {
	id, path string
	data     []byte
}

var readHostDesktopFiles = readLinuxHostDesktopFiles
var startHostCommand = startLinuxHostCommand

func hostDesktopDirectories(env func(string) string, home string) []string {
	data := env("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local/share")
	}
	dirs := []string{data}
	system := env("XDG_DATA_DIRS")
	if system == "" {
		system = "/usr/local/share:/usr/share"
	}
	for _, dir := range strings.Split(system, ":") {
		if filepath.IsAbs(dir) {
			dirs = append(dirs, dir)
		}
	}
	// Flatpak's own launchers, for sessions that leave them out of XDG_DATA_DIRS.
	dirs = append(dirs, filepath.Join(data, "flatpak/exports/share"), "/var/lib/flatpak/exports/share")
	seen := map[string]bool{}
	var result []string
	for _, dir := range dirs {
		dir = filepath.Join(filepath.Clean(dir), "applications")
		if !seen[dir] {
			seen[dir] = true
			result = append(result, dir)
		}
	}
	return result
}

// hostDesktopScript prints every desktop file the host session can launch as
// "path NUL size NUL content". It runs outside the sandbox, where the
// launcher cannot read these directories itself.
const hostDesktopScript = `set -f
data=${XDG_DATA_HOME:-$HOME/.local/share}
case $data in /*) ;; *) data=$HOME/.local/share ;; esac
old=$IFS; IFS=:
set -- "$data" ${XDG_DATA_DIRS:-/usr/local/share:/usr/share} "$data/flatpak/exports/share" /var/lib/flatpak/exports/share
IFS=$old
for dir do
  case $dir in /*) ;; *) continue ;; esac
  find -L "$dir/applications" -maxdepth 3 -type f -name '*.desktop' -size -257k 2>/dev/null | head -n 2000 | while IFS= read -r file; do
    size=$(wc -c <"$file" 2>/dev/null) || continue
    printf '%s\0%s\0' "$file" "$size"
    cat -- "$file" 2>/dev/null || exit 0
  done
done
`

func readLinuxHostDesktopFiles(ctx context.Context) ([]hostDesktopFile, error) {
	if inFlatpak() {
		cmd := exec.CommandContext(ctx, "flatpak-spawn", "--host", "sh", "-c", hostDesktopScript)
		var out limitedBuffer
		out.limit = maximumHostDesktopBytes
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("list apps on this computer: %w", err)
		}
		return parseHostDesktopStream(out.Bytes()), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var files []hostDesktopFile
	total := 0
	for _, dir := range hostDesktopDirectories(os.Getenv, home) {
		// Some distributions make the applications directory itself a link.
		dir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || len(files) >= maximumHostDesktopFiles {
				return filepath.SkipDir
			}
			if entry.IsDir() {
				if strings.Count(strings.TrimPrefix(path, dir), "/") > 2 {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".desktop") {
				return nil
			}
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > maximumHostDesktopFileBytes || total+int(info.Size()) > maximumHostDesktopBytes {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			total += len(data)
			files = append(files, hostDesktopFile{id: hostDesktopID(dir, path), path: path, data: data})
			return nil
		})
	}
	return files, nil
}

// limitedBuffer keeps a host command's output within a fixed size.
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("too much output")
	}
	return b.Buffer.Write(p)
}

func parseHostDesktopStream(stream []byte) []hostDesktopFile {
	var files []hostDesktopFile
	for len(stream) > 0 && len(files) < maximumHostDesktopFiles {
		pathEnd := bytes.IndexByte(stream, 0)
		if pathEnd < 0 {
			break
		}
		path := string(stream[:pathEnd])
		stream = stream[pathEnd+1:]
		sizeEnd := bytes.IndexByte(stream, 0)
		if sizeEnd < 0 {
			break
		}
		size := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(string(stream[:sizeEnd])), "%d", &size); err != nil || size < 0 || size > maximumHostDesktopFileBytes {
			break
		}
		stream = stream[sizeEnd+1:]
		if size > len(stream) {
			break
		}
		at := strings.LastIndex(path, "/applications/")
		if filepath.IsAbs(path) && at >= 0 {
			files = append(files, hostDesktopFile{id: hostDesktopID(path[:at+len("/applications")], path), path: path, data: stream[:size]})
		}
		stream = stream[size:]
	}
	return files
}

// A desktop file ID is its path below "applications" with "/" written as "-".
func hostDesktopID(dir, path string) string {
	return strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(path, dir), "/"), "/", "-")
}

func validHostDesktopID(id string) bool {
	if len(id) > 255 || !strings.HasSuffix(id, ".desktop") || len(id) == len(".desktop") || strings.HasPrefix(id, ".") {
		return false
	}
	for _, ch := range id {
		if ch <= 0x20 || ch == 0x7f || ch == '/' {
			return false
		}
	}
	return true
}

// desktopEntryValues reads the [Desktop Entry] group's unlocalized keys.
func desktopEntryValues(data []byte) map[string]string {
	values := map[string]string{}
	inEntry := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "[") {
			if inEntry {
				break
			}
			inEntry = strings.TrimSpace(line) == "[Desktop Entry]"
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !inEntry || !found || strings.HasPrefix(line, "#") {
			continue
		}
		key = strings.TrimSpace(key)
		if _, seen := values[key]; !seen {
			values[key] = strings.TrimLeft(value, " \t")
		}
	}
	return values
}

// desktopString undoes the escapes every desktop entry string value may carry.
func desktopString(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 == len(value) {
			b.WriteByte(value[i])
			continue
		}
		i++
		switch value[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(value[i])
		}
	}
	return b.String()
}

// desktopExecArguments splits an Exec value the way the desktop entry
// specification describes. An approved app starts without files, so the file
// and URL field codes expand to nothing.
func desktopExecArguments(value, name, file string) ([]string, error) {
	value = desktopString(value)
	var args []string
	var current strings.Builder
	started, quoted := false, false
	flush := func() {
		if started {
			args = append(args, current.String())
		}
		current.Reset()
		started = false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch {
		case quoted && ch == '\\':
			if i+1 == len(value) {
				return nil, fmt.Errorf("unfinished escape")
			}
			i++
			current.WriteByte(value[i])
		case ch == '"':
			quoted = !quoted
			started = true
		case !quoted && (ch == ' ' || ch == '\t'):
			flush()
		case ch == '%':
			if i+1 == len(value) {
				return nil, fmt.Errorf("unfinished field code")
			}
			i++
			switch value[i] {
			case '%':
				current.WriteByte('%')
				started = true
			case 'c':
				current.WriteString(name)
				started = true
			case 'k':
				current.WriteString(file)
				started = true
			case 'f', 'F', 'u', 'U', 'i', 'd', 'D', 'n', 'N', 'v', 'm':
			default:
				return nil, fmt.Errorf("unknown field code %%%c", value[i])
			}
		default:
			current.WriteByte(ch)
			started = true
		}
	}
	if quoted {
		return nil, fmt.Errorf("unfinished quote")
	}
	flush()
	if len(args) == 0 || args[0] == "" {
		return nil, fmt.Errorf("no command")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return nil, fmt.Errorf("invalid command")
		}
	}
	return args, nil
}

// linuxHostApps lists the graphical apps this computer's launcher would show,
// by name. A desktop file earlier in the search path hides a later one with
// the same ID, including when the earlier one hides the app.
func linuxHostApps(files []hostDesktopFile) []linuxHostApp {
	seen := map[string]bool{}
	var apps []linuxHostApp
	for _, file := range files {
		if !validHostDesktopID(file.id) || seen[file.id] {
			continue
		}
		seen[file.id] = true
		values := desktopEntryValues(file.data)
		name := strings.TrimSpace(desktopString(values["Name"]))
		if values["Type"] != "Application" || values["NoDisplay"] == "true" || values["Hidden"] == "true" || values["Terminal"] == "true" || file.id == linuxAppID+".desktop" {
			continue
		}
		if name == "" || len([]rune(name)) > 80 || strings.ContainsFunc(name, func(ch rune) bool { return ch < 0x20 || ch == 0x7f }) {
			continue
		}
		args, err := desktopExecArguments(values["Exec"], name, file.path)
		if err != nil {
			continue
		}
		apps = append(apps, linuxHostApp{ID: file.id, Name: name, file: file.path, exec: args})
	}
	sort.SliceStable(apps, func(i, j int) bool {
		if a, b := strings.ToLower(apps[i].Name), strings.ToLower(apps[j].Name); a != b {
			return a < b
		}
		return apps[i].ID < apps[j].ID
	})
	return apps
}

// approvedLinuxHostApps is the saved approval list as Settings shows it.
func approvedLinuxHostApps(prefs approvedAppPreferences) []linuxHostApp {
	apps := make([]linuxHostApp, 0, len(prefs.Apps))
	for _, app := range prefs.Apps {
		apps = append(apps, linuxHostApp{ID: app.Path, Name: app.Name})
	}
	return apps
}

// approveLinuxHostApps turns the desktop IDs chosen in Settings into the saved
// list. A choice already approved keeps its approval ID, so its entry inside
// Omarchy stays the same one. A newly chosen app must be installed now; an
// approved app that has since been removed may stay until the owner drops it.
func approveLinuxHostApps(prefs approvedAppPreferences, chosen []string, installed []linuxHostApp) (approvedAppPreferences, error) {
	if len(chosen) > maximumApprovedApps {
		return prefs, fmt.Errorf("you can approve up to %d apps", maximumApprovedApps)
	}
	current := map[string]approvedWindowsApp{}
	for _, app := range prefs.Apps {
		current[app.Path] = app
	}
	available := map[string]linuxHostApp{}
	for _, app := range installed {
		available[app.ID] = app
	}
	next := approvedAppPreferences{SchemaVersion: 1, Apps: []approvedWindowsApp{}}
	seen := map[string]bool{}
	for _, id := range chosen {
		if !validHostDesktopID(id) || seen[id] {
			return prefs, fmt.Errorf("invalid app selection")
		}
		seen[id] = true
		if app, ok := current[id]; ok {
			if found, ok := available[id]; ok {
				app.Name = found.Name
			}
			next.Apps = append(next.Apps, app)
			continue
		}
		found, ok := available[id]
		if !ok {
			return prefs, fmt.Errorf("that app is no longer installed on this computer")
		}
		approval, err := newApprovedAppID()
		if err != nil {
			return prefs, err
		}
		next.Apps = append(next.Apps, approvedWindowsApp{ID: approval, Name: found.Name, Path: id})
	}
	return next, next.validate()
}

func sameApprovedApps(a, b approvedAppPreferences) bool {
	if len(a.Apps) != len(b.Apps) {
		return false
	}
	for i := range a.Apps {
		if a.Apps[i] != b.Apps[i] {
			return false
		}
	}
	return true
}

func launchApprovedLinuxApp(dir, id string) error {
	if !validApprovedAppID(id) {
		return fmt.Errorf("invalid app ID")
	}
	prefs, err := loadApprovedWindowsApps(dir)
	if err != nil {
		return err
	}
	for _, approved := range prefs.Apps {
		if approved.ID != id {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		files, err := readHostDesktopFiles(ctx)
		if err != nil {
			return err
		}
		for _, app := range linuxHostApps(files) {
			if app.ID == approved.Path {
				logf("apps: starting approved app %s", app.ID)
				return startHostCommand(app.exec)
			}
		}
		return fmt.Errorf("approved app %s is no longer installed", approved.Path)
	}
	return fmt.Errorf("app is not approved")
}

// startLinuxHostCommand starts an approved app as its own session, so it
// outlives this launcher and the VM. Inside Flatpak the session helper runs
// it on the host; it keeps running after flatpak-spawn itself is gone.
func startLinuxHostCommand(args []string) error {
	if inFlatpak() {
		home, _ := os.UserHomeDir()
		args = append([]string{"flatpak-spawn", "--host", "--directory=" + home, "--"}, args...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	if !inFlatpak() {
		if home, err := os.UserHomeDir(); err == nil {
			cmd.Dir = home
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start approved app: %w", err)
	}
	go cmd.Wait()
	return nil
}
