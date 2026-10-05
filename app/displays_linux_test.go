//go:build linux

package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLinuxDisplaysMigrationAndPersistence(t *testing.T) {
	dir := t.TempDir()
	saved := settings{Displays: 3, Fullscreen: true, FullscreenDisplay: "DP-2"}
	p, err := loadLinuxDisplays(dir, saved)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Outputs) != 3 || p.Outputs[0] != (linuxGuestOutput{"DP-2", true}) || p.Outputs[1].Fullscreen {
		t.Fatalf("migration: %+v", p)
	}
	p.Outputs[1] = linuxGuestOutput{"HDMI-A-1", true}
	if err := saveLinuxDisplays(dir, p); err != nil {
		t.Fatal(err)
	}
	got, err := loadLinuxDisplays(dir, settings{})
	if err != nil || !reflect.DeepEqual(p, got) {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
	for _, bad := range []string{`{"schemaVersion":2,"outputs":[]}`, `{"schemaVersion":1,"outputs":[{"monitor":"DP-1,DP-2"}]}`, `{"schemaVersion":1,"outputs":[]} trailing`, `{"schemaVersion":1,"outputs":[{}],"unknown":true}`} {
		os.WriteFile(filepath.Join(dir, linuxDisplaysFilename), []byte(bad), 0600)
		if _, err := loadLinuxDisplays(dir, saved); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
func TestLinuxDisplayFlagsAndOverrides(t *testing.T) {
	cfg := &config{}
	f := flag.NewFlagSet("test", flag.ContinueOnError)
	registerLinuxDisplayFlags(f, cfg)
	if err := f.Parse([]string{"--displays=4"}); err != nil {
		t.Fatal(err)
	}
	explicit := map[string]bool{"displays": true, "fullscreen": true, "fullscreen-display": true}
	cfg.fullscreen = true
	cfg.fullscreenDisplay = "DP-9"
	var forwards forwardList
	var key string
	if err := applySettings(cfg, settings{Displays: 2}, explicit, &forwards, &key); err != nil {
		t.Fatal(err)
	}
	applyLinuxDisplays(cfg, migratedLinuxDisplays(settings{Displays: 2, FullscreenDisplay: "DP-1"}), explicit)
	if cfg.displays != 4 || cfg.displayTargets[0] != "DP-9" || !cfg.displayFullscreen[3] {
		t.Fatalf("overrides: %+v", cfg)
	}
	cfg.displays = 17
	if err := applySettings(cfg, settings{}, explicit, &forwards, &key); err == nil {
		t.Fatal("accepted too many outputs")
	}
}
func TestLinuxDisplaySizesAndArguments(t *testing.T) {
	cfg := &config{displays: 3, displayWidth: 1280, displayHeight: 800, displayTargets: []string{"HDMI-A-1", "DP-1", "missing"}}
	monitors := []linuxHostMonitor{{"DP-1", 1920, 1080}, {"HDMI-A-1", 2560, 1440}}
	setLinuxDisplaySizes(cfg, monitors, nil)
	want := [][2]int{{2560, 1440}, {1920, 1080}, {1280, 800}}
	if !reflect.DeepEqual(cfg.displaySizes, want) {
		t.Fatalf("sizes: %v", cfg.displaySizes)
	}
	single := linuxDisplayArgs(&config{displays: 1}, []string{"-device", "virtio-vga-gl,blob=on,hostmem=1073741824"})
	if single[1] != "virtio-vga-gl,blob=on,hostmem=1073741824" {
		t.Fatalf("single display device changed: %v", single)
	}
	gpu := `{"driver":"virtio-vga-gl","id":"gpu0","blob":true,"hostmem":1073741824,"venus":true,"max_outputs":3,"outputs":[]}`
	args := linuxDisplayArgs(cfg, []string{"-device", gpu, "-full-screen"})
	if !strings.Contains(args[1], `"blob":true`) || !strings.Contains(args[1], `"hostmem":1073741824`) {
		t.Fatalf("typed GPU properties changed: %v", args[1])
	}
	args = linuxGraphicsArgs(args, false, false)
	var device struct {
		Max     int `json:"max_outputs"`
		Outputs []struct {
			X int `json:"xres"`
			Y int `json:"yres"`
		}
		Driver string `json:"driver"`
	}
	if err := json.Unmarshal([]byte(args[1]), &device); err != nil {
		t.Fatal(err)
	}
	if device.Max != 3 || device.Driver != "virtio-vga-gl" || device.Outputs[0].X != 2560 || strings.Contains(args[1], "venus") || len(args) != 2 {
		t.Fatalf("args: %v", args)
	}
	setLinuxDisplaySizes(cfg, monitors, map[string]bool{"width": true, "height": true})
	if cfg.displaySizes[0] != ([2]int{1280, 800}) {
		t.Fatal("ignored explicit size")
	}
	cfg.displayFullscreen = []bool{true, false, true}
	env := linuxDisplayEnvironment([]string{"PATH=/bin", "SDL_VIDEO_DISPLAY_PRIORITY=old", "QEMU_SDL_OUTPUT_0=stale"}, cfg)
	if strings.Join(env, "\n") != "PATH=/bin\nQEMU_SDL_OUTPUT_0=HDMI-A-1\nQEMU_SDL_OUTPUT_FULLSCREEN_0=true\nQEMU_SDL_OUTPUT_1=DP-1\nQEMU_SDL_OUTPUT_FULLSCREEN_1=false\nQEMU_SDL_OUTPUT_2=missing\nQEMU_SDL_OUTPUT_FULLSCREEN_2=true" {
		t.Fatalf("env: %v", env)
	}
}
func TestLinuxDisplaysFormPreservesLegacyAndValidates(t *testing.T) {
	saved := settings{Displays: 2}
	previous := migratedLinuxDisplays(saved)
	previous.Outputs[1] = linuxGuestOutput{"DP-2", true}
	p, err := linuxDisplaysFromForm(&linuxSettingsForm{Fullscreen: true, FullscreenDisplay: "DP-1"}, previous, saved)
	if err != nil || p.Outputs[1] != previous.Outputs[1] || !p.Outputs[0].Fullscreen {
		t.Fatalf("legacy form: %+v %v", p, err)
	}
	if _, err := linuxDisplaysFromForm(&linuxSettingsForm{Displays: 2, DisplayOutputs: []linuxGuestOutput{{}}}, previous, saved); err == nil {
		t.Fatal("accepted count mismatch")
	}
}

func TestLinuxDisplaysLocationAndKernelMode(t *testing.T) {
	root := t.TempDir()
	initial := filepath.Join(root, "default")
	selected := filepath.Join(root, "chosen")
	saved := settings{Displays: 2, Fullscreen: true, FullscreenDisplay: "DP-1"}
	if err := saveSettings(settingsPath(initial), saved); err != nil {
		t.Fatal(err)
	}
	prefs := migratedLinuxDisplays(saved)
	prefs.Outputs[1] = linuxGuestOutput{"HDMI-A-1", true}
	if err := saveLinuxDisplays(initial, prefs); err != nil {
		t.Fatal(err)
	}
	got, proceed, err := resolveLinuxDataDirectory(initial, initial, false, func(string) (string, bool, error) { return selected, true, nil })
	if err != nil || !proceed || got != selected {
		t.Fatalf("location: %s %v %v", got, proceed, err)
	}
	carried, err := loadLinuxDisplays(selected, saved)
	if err != nil || !reflect.DeepEqual(carried, prefs) {
		t.Fatalf("carried: %+v %v", carried, err)
	}
	cfg := &config{displays: 2, displaySizes: [][2]int{{2560, 1440}, {1920, 1080}}}
	if got := linuxDisplayKernelOption(cfg); got != "" {
		t.Fatalf("multi-output generic mode: %s", got)
	}
	cfg.displays = 1
	if got := linuxDisplayKernelOption(cfg); got != " video=2560x1440" {
		t.Fatalf("single mode: %s", got)
	}
}
