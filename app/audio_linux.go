//go:build linux

package main

import (
	"slices"
	"strings"
)

func linuxAudioMode(mode string) (string, error) {
	switch mode {
	case "auto", "pipewire":
		return "pipewire", nil
	case "sdl", "none":
		return mode, nil
	}
	return "", uiError(uiText("error.linux.audio_mode"), nil)
}

func linuxAudioArgs(cfg *config, args []string) []string {
	if cfg.audio == "none" {
		return args
	}
	for i := 1; i < len(args); i++ {
		if args[i-1] != "-audiodev" {
			continue
		}
		// The backend options may already end with settings such as
		// in.voices=0, so match the id field rather than the suffix.
		fields := strings.Split(args[i], ",")
		if !slices.Contains(fields, "id=snd") {
			continue
		}
		if cfg.audio == "pipewire" && cfg.audioDevices.Output != "" {
			args[i] += ",out.name=" + qemuOptionValue(cfg.audioDevices.Output)
		}
		if cfg.desktop.MicrophoneDisabled {
			if !slices.Contains(fields, "in.voices=0") {
				args[i] += ",in.voices=0"
			}
		} else if cfg.audio == "pipewire" && cfg.audioDevices.Input != "" {
			args[i] += ",in.name=" + qemuOptionValue(cfg.audioDevices.Input)
		}
	}
	return args
}
