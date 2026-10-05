//go:build linux

package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const linuxInputSourcesNamespace = "org.gnome.desktop.input-sources"

// Detection is ordered by proximity to the user's desktop preference. Each
// source can be unavailable in a sandbox; no result means keep the guest's
// layout, and Settings explains that the host could not be detected.
func linuxHostKeyboard() (string, string) {
	return detectLinuxKeyboard(linuxPortalKeyboard, linuxKDEKeyboard, linuxLocaledKeyboard, func() (string, string) {
		return linuxKeyboardEnvironment(os.Getenv("XKB_DEFAULT_LAYOUT"), os.Getenv("XKB_DEFAULT_VARIANT"))
	})
}

func detectLinuxKeyboard(sources ...func() (string, string)) (string, string) {
	for _, source := range sources {
		layout, variant := source()
		if validLayoutName.MatchString(layout) && validVariantName.MatchString(variant) {
			return layout, variant
		}
	}
	return "", ""
}

func linuxPortalKeyboard() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return "", ""
	}
	defer conn.Close()
	return readPortalKeyboard(ctx, conn.Object(linuxPortalDesktop, linuxPortalObject))
}

// ReadAll is available even on version 1 of the Settings portal. GNOME's
// backend exports the input-sources schema without direct dconf access:
// https://gitlab.gnome.org/GNOME/xdg-desktop-portal-gnome/-/blob/main/src/settings.c
func readPortalKeyboard(ctx context.Context, object dbus.BusObject) (string, string) {
	var values map[string]map[string]dbus.Variant
	if err := object.CallWithContext(ctx, "org.freedesktop.portal.Settings.ReadAll", 0,
		[]string{linuxInputSourcesNamespace}).Store(&values); err != nil {
		return "", ""
	}
	settings := values[linuxInputSourcesNamespace]
	var sources, mru []linuxInputSource
	if value, ok := settings["sources"]; ok {
		_ = dbus.Store([]any{value.Value()}, &sources)
	}
	if value, ok := settings["mru-sources"]; ok {
		_ = dbus.Store([]any{value.Value()}, &mru)
	}
	return gnomeKeyboard(sources, mru)
}

type linuxInputSource struct{ Kind, ID string }

// GNOME puts the most recently used source first. Ignore stale MRU entries
// removed from sources, and input methods (IBus IDs are not XKB layouts).
func gnomeKeyboard(sources, mru []linuxInputSource) (string, string) {
	for _, recent := range mru {
		for _, source := range sources {
			if recent == source {
				if layout, variant := gnomeXKBSource(source); layout != "" {
					return layout, variant
				}
			}
		}
	}
	for _, source := range sources {
		if layout, variant := gnomeXKBSource(source); layout != "" {
			return layout, variant
		}
	}
	return "", ""
}

func gnomeXKBSource(source linuxInputSource) (string, string) {
	if source.Kind != "xkb" {
		return "", ""
	}
	// GNOME calls a variant "us+intl"; the guest needs "us:intl".
	layout, variant, _ := strings.Cut(source.ID, "+")
	if !validLayoutName.MatchString(layout) || !validVariantName.MatchString(variant) {
		return "", ""
	}
	return layout, variant
}

func linuxKDEKeyboard() (string, string) {
	// Flatpak's xdg-config/kxkbrc grant mounts the host file into the app's
	// XDG_CONFIG_HOME as well as its original host path.
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", ""
	}
	return readKDEKeyboard(filepath.Join(configDir, "kxkbrc"))
}

func readKDEKeyboard(path string) (string, string) {
	file, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer file.Close()
	return parseKDEKeyboard(file)
}

// KDE stores parallel comma-separated LayoutList and VariantList in [Layout].
// The file does not report the current group, so use the first configured one.
func parseKDEKeyboard(reader io.Reader) (string, string) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 64*1024))
	inLayout := false
	var layout, variant string
	// KDE defaults Use to false when its layout configuration is disabled.
	enabled := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inLayout = line == "[Layout]"
			continue
		}
		if !inLayout {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Use":
			enabled = strings.TrimSpace(value) == "true"
		case "LayoutList":
			layout = value
		case "VariantList":
			variant = value
		}
	}
	if scanner.Err() != nil || !enabled {
		return "", ""
	}
	if strings.TrimSpace(variant) == `\0` {
		variant = ""
	}
	return linuxKeyboardEnvironment(layout, variant)
}

func linuxLocaledKeyboard() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return "", ""
	}
	defer conn.Close()
	return readLocaledKeyboard(ctx, conn.Object("org.freedesktop.locale1", "/org/freedesktop/locale1"))
}

func readLocaledKeyboard(ctx context.Context, object dbus.BusObject) (string, string) {
	var values map[string]dbus.Variant
	if err := object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0,
		"org.freedesktop.locale1").Store(&values); err != nil {
		return "", ""
	}
	layout, layoutOK := values["X11Layout"].Value().(string)
	variant, variantOK := values["X11Variant"].Value().(string)
	if !layoutOK || !variantOK {
		return "", ""
	}
	return linuxKeyboardEnvironment(layout, variant)
}

// All list sources use the same group index. Preserve an empty first variant
// (",nodeadkeys" is not "nodeadkeys") and validate exactly as the guest does.
func linuxKeyboardEnvironment(layout, variant string) (string, string) {
	layout, _, _ = strings.Cut(layout, ",")
	variant, _, _ = strings.Cut(variant, ",")
	layout, variant = strings.TrimSpace(layout), strings.TrimSpace(variant)
	if !validLayoutName.MatchString(layout) || !validVariantName.MatchString(variant) {
		return "", ""
	}
	return layout, variant
}
