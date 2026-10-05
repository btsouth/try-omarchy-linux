//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const linuxExperiencePreferencesFilename = "linux-experience.json"

// Separate from settings.json so Windows and older Linux launchers can still
// read their known settings after a rollback.
type linuxExperiencePreferences struct {
	SchemaVersion int    `json:"schemaVersion"`
	Scale         string `json:"scale"`
	Keyboard      string `json:"keyboard"`
}

func (p linuxExperiencePreferences) validate() error {
	if _, err := parseLinuxScale(p.Scale); err != nil {
		return err
	}
	if p.Keyboard != "" && p.Keyboard != "keep" {
		layout, variant := splitKeyboardSpec(p.Keyboard)
		if !validLayoutName.MatchString(layout) || !validVariantName.MatchString(variant) || strings.Count(p.Keyboard, ":") > 1 {
			return uiError(uiText("settings.linux.invalid_keyboard"), nil)
		}
	}
	return nil
}

func loadLinuxExperiencePreferences(dir string) (linuxExperiencePreferences, error) {
	defaults := linuxExperiencePreferences{SchemaVersion: 1, Scale: "auto"}
	data, err := os.ReadFile(filepath.Join(dir, linuxExperiencePreferencesFilename))
	if os.IsNotExist(err) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	if len(data) > 4096 {
		return defaults, uiError(uiText("settings.linux.experience_too_large"), nil)
	}
	var p linuxExperiencePreferences
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return defaults, err
	}
	if d.Decode(&struct{}{}) != io.EOF || p.SchemaVersion != 1 {
		return defaults, uiError(uiText("settings.linux.invalid_experience"), nil)
	}
	return p, p.validate()
}

func saveLinuxExperiencePreferences(dir string, p linuxExperiencePreferences) error {
	p.SchemaVersion = 1
	if err := p.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".linux-experience-*")
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
	return os.Rename(f.Name(), filepath.Join(dir, linuxExperiencePreferencesFilename))
}
