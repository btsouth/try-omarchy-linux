//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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

func parsePipeWireAudioDevices(data []byte) (outputs, inputs []linuxAudioDevice, err error) {
	var objects []pipeWireObject
	if err := json.Unmarshal(data, &objects); err != nil {
		return nil, nil, fmt.Errorf("cannot read audio devices: %w", err)
	}
	seen := map[string]bool{}
	for _, object := range objects {
		if object.Type != "PipeWire:Interface:Node" {
			continue
		}
		class, _ := object.Info.Props["media.class"].(string)
		if class != "Audio/Sink" && class != "Audio/Source" {
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
		label = strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 32 || r == 127 {
				return ' '
			}
			return r
		}, label))
		if runes := []rune(label); len(runes) > 80 {
			label = string(runes[:80]) + "..."
		}
		device := linuxAudioDevice{Name: name, Label: label}
		if class == "Audio/Sink" {
			outputs = append(outputs, device)
		} else {
			inputs = append(inputs, device)
		}
	}
	sort.Slice(outputs, func(i, j int) bool { return outputs[i].Label < outputs[j].Label })
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Label < inputs[j].Label })
	return outputs, inputs, nil
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
