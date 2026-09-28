//go:build linux

package main

import (
	"fmt"
	"strings"
)

func linuxAudioMode(mode string) (string, error) {
	switch mode {
	case "auto", "pipewire":
		return "pipewire", nil
	case "sdl", "none":
		return mode, nil
	}
	return "", fmt.Errorf("-audio must be auto, pipewire, sdl, or none")
}

func linuxAudioArgs(cfg *config, args []string) []string {
	if cfg.audio == "none" {
		return args
	}
	for i := 1; i < len(args); i++ {
		if args[i-1] == "-audiodev" && strings.HasSuffix(args[i], ",id=snd") {
			if cfg.audio == "pipewire" && cfg.audioDevices.Output != "" {
				args[i] += ",out.name=" + qemuOptionValue(cfg.audioDevices.Output)
			}
			if cfg.desktop.MicrophoneDisabled {
				args[i] += ",in.voices=0"
			} else if cfg.audio == "pipewire" && cfg.audioDevices.Input != "" {
				args[i] += ",in.name=" + qemuOptionValue(cfg.audioDevices.Input)
			}
		}
	}
	return args
}
