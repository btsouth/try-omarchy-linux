//go:build linux

package main

import (
	"context"
	"fmt"
	"time"
)

const linuxAudioObject = "/audiodevs/snd"
const linuxAudioOutputProperty = "try-omarchy-output-device"
const linuxAudioInputProperty = "try-omarchy-input-device"

func linuxAudioRoute(ctx context.Context, qmp *qmpClient, property string) (string, error) {
	var name string
	err := qmp.Call(ctx, "qom-get", map[string]any{"path": linuxAudioObject, "property": property}, &name)
	return name, err
}

func linuxLiveAudioAvailable(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	qmp, err := dialQMPControl(ctx, qmpToolsPort)
	if err != nil {
		return false
	}
	defer qmp.Close()
	if _, err := linuxAudioRoute(ctx, qmp, linuxAudioOutputProperty); err != nil {
		return false
	}
	_, err = linuxAudioRoute(ctx, qmp, linuxAudioInputProperty)
	return err == nil
}

func applyLinuxAudioRoutes(parent context.Context, next audioPreferences) error {
	if err := next.validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	qmp, err := dialQMPControl(ctx, qmpToolsPort)
	if err != nil {
		return err
	}
	defer qmp.Close()
	return applyLinuxAudioRoutesWithClient(ctx, qmp, next)
}

func applyLinuxAudioRoutesWithClient(ctx context.Context, qmp *qmpClient, next audioPreferences) error {
	output, err := linuxAudioRoute(ctx, qmp, linuxAudioOutputProperty)
	if err != nil {
		return err
	}
	input, err := linuxAudioRoute(ctx, qmp, linuxAudioInputProperty)
	if err != nil {
		return err
	}
	set := func(property, name string) error {
		return qmp.Call(ctx, "qom-set", map[string]any{"path": linuxAudioObject, "property": property, "value": name}, nil)
	}
	changedOutput := output != next.Output
	if changedOutput {
		if err := set(linuxAudioOutputProperty, next.Output); err != nil {
			return fmt.Errorf("playback route: %w", err)
		}
	}
	if input != next.Input {
		if err := set(linuxAudioInputProperty, next.Input); err != nil {
			if changedOutput {
				if rollbackErr := set(linuxAudioOutputProperty, output); rollbackErr != nil {
					return fmt.Errorf("microphone route: %v; could not restore playback: %w", err, rollbackErr)
				}
			}
			return fmt.Errorf("microphone route: %w", err)
		}
	}
	return nil
}
