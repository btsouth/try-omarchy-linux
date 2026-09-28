//go:build linux

package main

import "testing"

func TestLinuxAudioRoutesAndMicrophone(t *testing.T) {
	cfg := &config{audio: "pipewire", audioDevices: audioPreferences{Output: "USB, speakers", Input: "microphone"}, desktop: desktopPreferences{MicrophoneDisabled: true}}
	args := linuxAudioArgs(cfg, []string{"-audiodev", "pipewire,id=snd"})
	if args[1] != "pipewire,id=snd,out.name=USB,, speakers,in.voices=0" {
		t.Fatal(args)
	}
	cfg.desktop.MicrophoneDisabled = false
	args = linuxAudioArgs(cfg, []string{"-audiodev", "pipewire,id=snd"})
	if args[1] != "pipewire,id=snd,out.name=USB,, speakers,in.name=microphone" {
		t.Fatal(args)
	}
	cfg.audio = "sdl"
	cfg.desktop.MicrophoneDisabled = true
	if got := linuxAudioArgs(cfg, []string{"-audiodev", "sdl,id=snd"})[1]; got != "sdl,id=snd,in.voices=0" {
		t.Fatal(got)
	}
	if _, err := linuxAudioMode("unknown"); err == nil {
		t.Fatal("unknown backend accepted")
	}
}
