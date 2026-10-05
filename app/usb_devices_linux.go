//go:build linux

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var linuxUSBRequests = make(chan struct{}, 1)
var linuxUSBOpen atomic.Bool

func showLinuxUSBDevices(parent context.Context) {
	if !linuxUSBOpen.CompareAndSwap(false, true) {
		return
	}
	defer linuxUSBOpen.Store(false)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	w := startLinuxWindow(cancel)
	if w == nil {
		return
	}
	defer w.stop()
	showLinuxUSBInWindow(ctx, w, "", false)
}

func linuxUSBState(devices []usbDevice, selection bool, pref usbPreferences, status string) linuxSetupState {
	intro := uiText("usb.linux.intro")
	if selection {
		intro = uiText("usb.linux.next_start")
	}
	rows := []linuxRow{}
	for i, d := range devices {
		state := uiText("usb.state.available")
		if !d.Connected {
			state = uiText("usb.state.not_connected")
		}
		if d.Claimed {
			state = uiText("usb.state.attached")
			if !d.Connected {
				state = uiText("usb.state.unplugged")
			}
		}
		if selection && pref.Enabled && pref.Device != nil && pref.Device.matches(d) {
			state += " · " + uiText("usb.linux.selected")
		}
		rows = append(rows, linuxRow{Title: d.Name, Detail: fmt.Sprintf("%s · USB %d/%s", state, d.Bus, d.Port), Reply: "device:" + strconv.Itoa(i)})
	}
	if len(rows) == 0 {
		rows = append(rows, linuxRow{Title: uiText("usb.count.none")})
	}
	actions := []linuxAction{{Label: uiText("usb.refresh"), Reply: "refresh"}, {Label: uiText("usb.close"), Reply: "close"}}
	if selection {
		actions = append([]linuxAction{{Label: uiText("usb.dont_attach"), Reply: "disable"}}, actions...)
	}
	return linuxSetupState{Prompt: "usb", Title: uiText("usb.title"), Status: intro + "\n\n" + status, Sections: []linuxSection{{Rows: rows}}, Actions: actions}
}

// Each round refreshes identities before presenting actionable rows. Attach
// validates the fresh address again; Release can also remove an unplugged claim.
func showLinuxUSBInWindow(ctx context.Context, w *linuxSetupWindow, dir string, selection bool) {
	status := ""
	for ctx.Err() == nil {
		pref := usbPreferences{SchemaVersion: 1}
		var devices []usbDevice
		var err error
		if selection {
			pref, err = loadUSBPreferences(dir)
			if err == nil {
				devices, err = readLinuxUSBDevices(ctx, "/sys/bus/usb/devices")
			}
			devices = usbSelectionChoices(devices, pref.Device)
		} else {
			callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			var client *qmpClient
			client, err = dialQMPControl(callCtx, qmpToolsPort)
			if err == nil {
				devices, err = newLinuxUSBBroker(client).Devices(callCtx)
				client.Close()
			}
			cancel()
		}
		if err != nil {
			status = err.Error()
			devices = nil
		}
		state := linuxUSBState(devices, selection, pref, status)
		if err != nil {
			state.Actions = state.Actions[len(state.Actions)-2:]
		}
		reply, askErr := w.ask(ctx, state)
		if askErr != nil || reply == "close" || reply == "cancel" {
			return
		}
		status = ""
		if reply == "refresh" {
			continue
		}
		if selection && reply == "disable" {
			pref.Enabled = false
			err = saveLinuxUSBChoice(dir, pref)
			if err == nil {
				status = uiText("usb.save.none")
			}
		} else {
			index, parseErr := strconv.Atoi(strings.TrimPrefix(reply, "device:"))
			if parseErr != nil || !strings.HasPrefix(reply, "device:") || index < 0 || index >= len(devices) {
				continue
			}
			d := devices[index]
			label := uiText("usb.attach")
			body := uiTextWith("usb.linux.attach_confirm", map[string]string{"device": d.Name})
			if selection {
				label = uiText("usb.save_choice")
				body = uiTextWith("usb.linux.start_confirm", map[string]string{"device": d.Name})
			} else if d.Claimed {
				label = uiText("usb.release")
				body = uiTextWith("usb.linux.release_confirm", map[string]string{"device": d.Name})
			}
			answer, e := w.ask(ctx, linuxSetupState{Prompt: "choice", Title: uiText("usb.title"), Status: body, Primary: label, Secondary: uiText("setup.cancel")})
			if e != nil {
				return
			}
			if answer != "primary" {
				continue
			}
			if selection {
				pref.Device, pref.Enabled = selectionForUSB(d), true
				err = saveLinuxUSBChoice(dir, pref)
				if err == nil {
					status = uiText("usb.save.device")
				}
			} else {
				callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				var client *qmpClient
				client, err = dialQMPControl(callCtx, qmpToolsPort)
				if err == nil {
					broker := newLinuxUSBBroker(client)
					if d.Claimed {
						err = broker.Detach(callCtx, d.ID)
					} else {
						err = broker.Attach(callCtx, d)
					}
					client.Close()
				}
				cancel()
			}
		}
		if err != nil {
			status = err.Error()
		}
	}
}

func saveLinuxUSBChoice(dir string, pref usbPreferences) error {
	return saveUSBPreferences(dir, pref)
}

func startLinuxBootUSB(parent context.Context, dir string, vmDone <-chan struct{}) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	go func() {
		select {
		case <-vmDone:
			cancel()
		case <-ctx.Done():
		}
	}()
	err := applyLinuxBootUSB(ctx, dir, func(ctx context.Context) (usbBroker, func(), error) {
		client, err := dialQMPControl(ctx, qmpToolsPort)
		if err != nil {
			return usbBroker{}, func() {}, err
		}
		return newLinuxUSBBroker(client), func() { client.Close() }, nil
	})
	if err != nil && ctx.Err() == nil {
		tellLinuxUser("usb", uiText("usb.linux.not_attached"), err.Error())
	}
}

// Resolve the saved grant once, and make exactly one attachment attempt.
// No monitor is opened when the grant is disabled.
func applyLinuxBootUSB(ctx context.Context, dir string, connect func(context.Context) (usbBroker, func(), error)) error {
	pref, err := loadUSBPreferences(dir)
	if err != nil || !pref.Enabled {
		return err
	}
	broker, close, err := connect(ctx)
	if err != nil {
		return err
	}
	defer close()
	return broker.AttachSaved(ctx, pref)
}
