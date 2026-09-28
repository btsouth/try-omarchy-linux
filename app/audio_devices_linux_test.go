//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParsePipeWireAudioDevices(t *testing.T) {
	data := []byte(`[
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Audio/Sink","node.name":"sink.z","node.description":"Z Speakers"}}},
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Audio/Sink","node.name":"sink.a","node.description":"A Speakers"}}},
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Audio/Sink","node.name":"sink.a","node.description":"Duplicate"}}},
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Audio/Source","node.name":"source.one","node.description":"Microphone\nOne"}}},
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Video/Source","node.name":"camera"}}}
    ]`)
	outputs, inputs, err := parsePipeWireAudioDevices(data)
	if err != nil {
		t.Fatal(err)
	}
	if want := []linuxAudioDevice{{"sink.a", "A Speakers"}, {"sink.z", "Z Speakers"}}; !reflect.DeepEqual(outputs, want) {
		t.Fatalf("outputs: %+v", outputs)
	}
	if want := []linuxAudioDevice{{"source.one", "Microphone One"}}; !reflect.DeepEqual(inputs, want) {
		t.Fatalf("inputs: %+v", inputs)
	}
	if _, _, err := parsePipeWireAudioDevices([]byte(`{"not":"a list"}`)); err == nil {
		t.Fatal("accepted malformed device inventory")
	}
}

func TestLinuxAudioSettingsHelper(t *testing.T) {
	if os.Getenv("TRY_OMARCHY_AUDIO_SETTINGS_HELPER") == "" {
		return
	}
	fmt.Println(`{"event":"ready"}`)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var state linuxSetupState
		if err := json.Unmarshal(scanner.Bytes(), &state); err != nil {
			panic(err)
		}
		if state.Prompt != "settings" {
			continue
		}
		form := *state.Settings
		form.AudioOutput = "sink.test"
		form.AudioInput = "source.test"
		form.DiskGiB = "32"
		form.Scale = "1.5"
		form.Keyboard = "de"
		value, err := json.Marshal(form)
		if err != nil {
			panic(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(value)}); err != nil {
			panic(err)
		}
	}
}

func TestLinuxAudioAndDiskSettingsPersistForNextLaunch(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "default")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxAudioSettingsHelper$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_AUDIO_SETTINGS_HELPER=1")
	w := launchLinuxWindow(cmd, func() {})
	if w == nil {
		t.Fatal("audio settings helper did not start")
	}
	defer w.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if message := showLinuxSettingsInWindow(ctx, w, dir, false); message == "" {
		t.Fatal("audio settings were not saved")
	}
	prefs, err := loadAudioPreferences(dir)
	if err != nil || prefs.Output != "sink.test" || prefs.Input != "source.test" {
		t.Fatalf("saved routes: %+v %v", prefs, err)
	}
	storage, err := loadStorageSettings(dir)
	if err != nil || storage.DiskGiB != 32 {
		t.Fatalf("saved capacity: %+v %v", storage, err)
	}
	experience, err := loadLinuxExperiencePreferences(dir)
	if err != nil || experience.Scale != "1.5" || experience.Keyboard != "de" {
		t.Fatalf("saved display and keyboard choices: %+v %v", experience, err)
	}
	cfg := &config{audio: "pipewire", audioDevices: prefs}
	args := linuxAudioArgs(cfg, []string{"qemu", "-audiodev", "pipewire,id=snd"})
	if want := "pipewire,id=snd,out.name=sink.test,in.name=source.test"; args[2] != want {
		t.Fatalf("next launch audio: %q", args[2])
	}
	selected := filepath.Join(root, "custom")
	got, proceed, err := resolveLinuxDataDirectory(dir, dir, false, func(string) (string, bool, error) {
		return selected, true, nil
	})
	if err != nil || !proceed || got != selected {
		t.Fatalf("custom storage: %q %t %v", got, proceed, err)
	}
	carried, err := loadAudioPreferences(selected)
	if err != nil || carried.Output != prefs.Output || carried.Input != prefs.Input {
		t.Fatalf("audio routes lost on storage choice: %+v %v", carried, err)
	}
	carriedStorage, err := loadStorageSettings(selected)
	if err != nil || carriedStorage.DiskGiB != storage.DiskGiB {
		t.Fatalf("disk capacity lost on storage choice: %+v %v", carriedStorage, err)
	}
	carriedExperience, err := loadLinuxExperiencePreferences(selected)
	if err != nil || carriedExperience.Scale != experience.Scale || carriedExperience.Keyboard != experience.Keyboard {
		t.Fatalf("display or keyboard choice lost on storage choice: %+v %v", carriedExperience, err)
	}
}
