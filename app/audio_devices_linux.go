//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type linuxAudioDevice struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type pipeWireObject struct {
	Type string `json:"type"`
	Info struct {
		Props map[string]any `json:"props"`
	} `json:"info"`
}

func parsePipeWireMediaDevices(data []byte, classes ...string) (map[string][]linuxAudioDevice, error) {
	var objects []pipeWireObject
	if err := json.Unmarshal(data, &objects); err != nil {
		return nil, fmt.Errorf("cannot read devices: %w", err)
	}
	result := map[string][]linuxAudioDevice{}
	for _, class := range classes {
		result[class] = nil
	}
	seen := map[string]bool{}
	for _, object := range objects {
		if object.Type != "PipeWire:Interface:Node" {
			continue
		}
		class, _ := object.Info.Props["media.class"].(string)
		if _, wanted := result[class]; !wanted {
			continue
		}
		name, _ := object.Info.Props["node.name"].(string)
		if name == "" || len(name) > 4096 || strings.ContainsRune(name, 0) || seen[class+"\x00"+name] {
			continue
		}
		seen[class+"\x00"+name] = true
		label, _ := object.Info.Props["node.description"].(string)
		if strings.TrimSpace(label) == "" {
			label = name
		}
		if class == "Video/Source" {
			if path, _ := object.Info.Props["api.v4l2.path"].(string); path != "" {
				label += " (" + filepath.Base(path) + ")"
			}
		}
		label = strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 32 || r == 127 {
				return ' '
			}
			return r
		}, label))
		if runes := []rune(label); len(runes) > 80 {
			label = string(runes[:80]) + "..."
		}
		result[class] = append(result[class], linuxAudioDevice{Name: name, Label: label})
	}
	for _, devices := range result {
		sort.Slice(devices, func(i, j int) bool {
			if devices[i].Label == devices[j].Label {
				return devices[i].Name < devices[j].Name
			}
			return devices[i].Label < devices[j].Label
		})
	}
	return result, nil
}

func parsePipeWireAudioDevices(data []byte) (outputs, inputs []linuxAudioDevice, err error) {
	devices, err := parsePipeWireMediaDevices(data, "Audio/Sink", "Audio/Source")
	return devices["Audio/Sink"], devices["Audio/Source"], err
}

func listLinuxCameraDevices() ([]linuxAudioDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return nil, fmt.Errorf("camera devices are unavailable")
	}
	devices, err := parsePipeWireMediaDevices(data, "Video/Source")
	return devices["Video/Source"], err
}

// pw-dump is present in the Flatpak runtime. Its socket is restricted by the
// app sandbox and may be absent, for example in a private desktop without audio.
func listLinuxAudioDevices() (outputs, inputs []linuxAudioDevice, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("audio devices are unavailable")
	}
	return parsePipeWireAudioDevices(data)
}
