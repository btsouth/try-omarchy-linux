//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const linuxDisplaysFilename = "linux-displays.json"

type linuxGuestOutput struct {
	Monitor    string `json:"monitor"`
	Fullscreen bool   `json:"fullscreen"`
}
type linuxDisplayPreferences struct {
	SchemaVersion int                `json:"schemaVersion"`
	Outputs       []linuxGuestOutput `json:"outputs"`
}
type linuxHostMonitor struct {
	Connector string `json:"connector"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func (p linuxDisplayPreferences) validate() error {
	if p.SchemaVersion != 1 || len(p.Outputs) < 1 || len(p.Outputs) > maximumGuestDisplays {
		return uiError(uiText("settings.linux.invalid_displays"), nil)
	}
	for _, output := range p.Outputs {
		if len(output.Monitor) > 64 || strings.ContainsAny(output.Monitor, ",\x00\r\n") {
			return uiError(uiText("settings.linux.fullscreen_display_name_is_invalid"), nil)
		}
	}
	return nil
}

func migratedLinuxDisplays(s settings) linuxDisplayPreferences {
	p := linuxDisplayPreferences{SchemaVersion: 1, Outputs: make([]linuxGuestOutput, guestDisplayCount(s.Displays))}
	p.Outputs[0] = linuxGuestOutput{Monitor: s.FullscreenDisplay, Fullscreen: s.Fullscreen}
	return p
}
func loadLinuxDisplays(dir string, s settings) (linuxDisplayPreferences, error) {
	defaults := migratedLinuxDisplays(s)
	data, err := os.ReadFile(filepath.Join(dir, linuxDisplaysFilename))
	if os.IsNotExist(err) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	if len(data) > 8192 {
		return defaults, uiError(uiText("settings.linux.invalid_displays"), nil)
	}
	var p linuxDisplayPreferences
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return defaults, err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return defaults, uiError(uiText("settings.linux.invalid_displays"), nil)
	}
	return p, p.validate()
}
func saveLinuxDisplays(dir string, p linuxDisplayPreferences) error {
	if err := p.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".linux-displays-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, linuxDisplaysFilename))
}

func registerLinuxDisplayFlags(flags *flag.FlagSet, cfg *config) {
	flags.IntVar(&cfg.displays, "displays", 1, uiText("launcher.linux.flag.displays"))
}

func applyLinuxDisplays(cfg *config, p linuxDisplayPreferences, explicit map[string]bool) {
	cfg.displayTargets = make([]string, guestDisplayCount(cfg.displays))
	cfg.displayFullscreen = make([]bool, len(cfg.displayTargets))
	for i := range cfg.displayTargets {
		if i < len(p.Outputs) {
			cfg.displayTargets[i], cfg.displayFullscreen[i] = p.Outputs[i].Monitor, p.Outputs[i].Fullscreen
		}
		if explicit["fullscreen"] {
			cfg.displayFullscreen[i] = cfg.fullscreen
		}
	}
	if explicit["fullscreen-display"] {
		cfg.displayTargets[0] = cfg.fullscreenDisplay
	}
}

// The GTK helper probes the same monitor list Settings uses, without creating
// a window. Missing UI/display access leaves the explicit/default guest size.
func probeLinuxMonitors() []linuxHostMonitor {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, filepath.Join(filepath.Dir(self), "try-omarchy-setup"), "--list-monitors").Output()
	if err != nil || len(data) > 65536 {
		return nil
	}
	var monitors []linuxHostMonitor
	if json.Unmarshal(data, &monitors) != nil {
		return nil
	}
	return monitors
}
func setLinuxDisplaySizes(cfg *config, monitors []linuxHostMonitor, explicit map[string]bool) {
	cfg.displaySizes = make([][2]int, guestDisplayCount(cfg.displays))
	for i := range cfg.displaySizes {
		size := [2]int{cfg.displayWidth, cfg.displayHeight}
		target := ""
		if i < len(cfg.displayTargets) {
			target = cfg.displayTargets[i]
		}
		index := i % max(1, len(monitors))
		if target != "" {
			index = -1
			for n, m := range monitors {
				if m.Connector == target {
					index = n
					break
				}
			}
		}
		if index >= 0 && index < len(monitors) {
			m := monitors[index]
			// Resolve Automatic using the same ordering as the size probe. SDL's
			// monitor ordering need not match GDK's.
			if target == "" && i < len(cfg.displayTargets) && len(m.Connector) <= 64 && !strings.ContainsAny(m.Connector, ",\x00\r\n") {
				cfg.displayTargets[i] = m.Connector
			}
			if !explicit["width"] && m.Width >= 640 && m.Width <= 8192 {
				size[0] = m.Width
			}
			if !explicit["height"] && m.Height >= 480 && m.Height <= 8192 {
				size[1] = m.Height
			}
		}
		cfg.displaySizes[i] = size
	}
}

func linuxDisplayEnvironment(env []string, cfg *config) []string {
	clean := make([]string, 0, len(env)+2*cfg.displays)
	for _, e := range env {
		if !strings.HasPrefix(e, "QEMU_SDL_OUTPUT_") && !strings.HasPrefix(e, "SDL_VIDEO_DISPLAY_PRIORITY=") {
			clean = append(clean, e)
		}
	}
	for i := 0; i < guestDisplayCount(cfg.displays); i++ {
		target := ""
		if i < len(cfg.displayTargets) {
			target = cfg.displayTargets[i]
		}
		fullscreen := cfg.fullscreen
		if i < len(cfg.displayFullscreen) {
			fullscreen = cfg.displayFullscreen[i]
		}
		clean = append(clean, fmt.Sprintf("QEMU_SDL_OUTPUT_%d=%s", i, target), fmt.Sprintf("QEMU_SDL_OUTPUT_FULLSCREEN_%d=%s", i, strconv.FormatBool(fullscreen)))
	}
	return clean
}

// Keep the shared Windows device builder untouched. Linux supplies per-output
// EDID sizes, including output 1, before its graphics policy removes Venus.
func linuxDisplayArgs(cfg *config, args []string) []string {
	if len(cfg.displaySizes) > 0 {
		outputs := make([]map[string]any, guestDisplayCount(cfg.displays))
		for i := range outputs {
			size := [2]int{cfg.displayWidth, cfg.displayHeight}
			if i < len(cfg.displaySizes) {
				size = cfg.displaySizes[i]
			}
			outputs[i] = map[string]any{"name": fmt.Sprintf("Omarchy %d", i+1), "xres": size[0], "yres": size[1]}
		}
		// Only the typed JSON device from displayDevice is rewritten. QEMU
		// rejects JSON strings such as "on" for boolean and size properties.
		for i := 1; i < len(args); i++ {
			if args[i-1] != "-device" || !strings.HasPrefix(args[i], "{") {
				continue
			}
			device := map[string]any{}
			if json.Unmarshal([]byte(args[i]), &device) != nil || (device["driver"] != "virtio-gpu-pci" && device["driver"] != "virtio-vga-gl") {
				continue
			}
			device["max_outputs"], device["outputs"] = len(outputs), outputs
			data, _ := json.Marshal(device)
			args[i] = string(data)
		}
	}
	// Per-console startup fullscreen is handled by the Linux SDL patch.
	for i := 1; i < len(args); i++ {
		if args[i-1] == "-display" {
			args[i] = strings.ReplaceAll(args[i], ",full-screen=on", "")
		}
	}
	clean := args[:0]
	for _, arg := range args {
		if arg != "-full-screen" {
			clean = append(clean, arg)
		}
	}
	return clean
}

func linuxDisplaysFromForm(form *linuxSettingsForm, previous linuxDisplayPreferences, saved settings) (linuxDisplayPreferences, error) {
	// Older helpers and saved-form retries did not include these fields.
	if form.Displays == 0 {
		form.Displays = guestDisplayCount(saved.Displays)
		form.DisplayOutputs = append([]linuxGuestOutput(nil), previous.Outputs...)
		form.DisplayOutputs[0] = linuxGuestOutput{form.FullscreenDisplay, form.Fullscreen}
	}
	if len(form.DisplayOutputs) > 0 {
		form.DisplayOutputs[0] = linuxGuestOutput{strings.TrimSpace(form.FullscreenDisplay), form.Fullscreen}
	}
	p := linuxDisplayPreferences{SchemaVersion: 1, Outputs: form.DisplayOutputs}
	if form.Displays < 1 || form.Displays > maximumGuestDisplays || len(p.Outputs) != form.Displays {
		return p, uiError(uiText("settings.linux.invalid_displays"), nil)
	}
	return p, p.validate()
}

// A generic video= mode would force every DRM connector to output 1's size.
// Multi-output guests instead choose each output's EDID preferred mode.
func linuxDisplayKernelOption(cfg *config) string {
	if guestDisplayCount(cfg.displays) > 1 {
		return ""
	}
	size := [2]int{cfg.displayWidth, cfg.displayHeight}
	if len(cfg.displaySizes) > 0 {
		size = cfg.displaySizes[0]
	}
	return fmt.Sprintf(" video=%dx%d", size[0], size[1])
}
