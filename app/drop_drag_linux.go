//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// On Linux, QEMU's SDL window reports where files were dropped, with the
// window size, and reports once when the pointer moves away afterwards
// (runtime-build/linux/patches/qemu/0111). Wayland gives the launcher no
// global pointer position, so that report stands in for the Windows pointer
// check. A runtime without the patch reports no position, so the guest keeps
// the files in Downloads.

const (
	linuxDropWindowLimit = 1 << 16
	// QEMU watches the pointer for 30 seconds from the drop. Drag only
	// well inside that, timed from when the launcher saw the drop, so a
	// move QEMU no longer reports cannot go unnoticed.
	linuxDropDragWindow = 25 * time.Second
)

var (
	// dropPointerMoves counts DISPLAY_DROP_POINTER_MOVED events. A drop
	// records the count it saw; a different count at drag time means the
	// pointer moved after the drop.
	dropPointerMoves atomic.Uint64
	linuxDragBusy    atomic.Bool
)

// linuxDropPosition returns x, y, width and height from a positioned
// DISPLAY_FILE_DROP event, or nil when the event has no usable position.
func linuxDropPosition(line string) []int {
	var event struct {
		Data struct {
			X      *int `json:"x"`
			Y      *int `json:"y"`
			Width  *int `json:"width"`
			Height *int `json:"height"`
		} `json:"data"`
	}
	if len(line) > 1<<20 || json.Unmarshal([]byte(line), &event) != nil {
		return nil
	}
	d := event.Data
	if d.X == nil || d.Y == nil || d.Width == nil || d.Height == nil {
		return nil
	}
	x, y, width, height := *d.X, *d.Y, *d.Width, *d.Height
	if width < 2 || height < 2 || width > linuxDropWindowLimit || height > linuxDropWindowLimit ||
		x < 0 || y < 0 || x >= width || y >= height {
		return nil
	}
	return []int{x, y, width, height}
}

func linuxDropPointerMoved(line string) bool {
	var event struct {
		Event string `json:"event"`
	}
	return len(line) <= 4096 && json.Unmarshal([]byte(line), &event) == nil && event.Event == "DISPLAY_DROP_POINTER_MOVED"
}

// performLinuxDropDrag answers the guest's "drop-drag" request for one drop
// by driving the guest tablet through QEMU's otherwise unused forward QMP
// socket. It returns once the drag has started.
func performLinuxDropDrag(id string, x, y int) error {
	drop, err := takeDrop(id, time.Now())
	if err != nil {
		return err
	}
	if !guestReady.Load() {
		return errors.New("Omarchy is not running")
	}
	if dropPointerMoves.Load() != drop.pointerMoves {
		return errors.New("the pointer moved after the drop")
	}
	deadline := drop.at.Add(linuxDropDragWindow)
	if !time.Now().Before(deadline) {
		return errors.New("the drop is too old")
	}
	steps, err := dropDragSteps(x, y, drop)
	if err != nil {
		return err
	}
	if !linuxDragBusy.CompareAndSwap(false, true) {
		return errors.New("another drag is in progress")
	}
	ctx, cancel := context.WithTimeout(context.Background(), dragScriptLifetime)
	qmp, err := dialQMPControl(ctx, qmpFwdPort)
	if err != nil {
		cancel()
		linuxDragBusy.Store(false)
		return fmt.Errorf("QEMU input is unavailable: %w", err)
	}
	logf("file drop: dragging into the app under the pointer")
	go func() {
		defer linuxDragBusy.Store(false)
		defer cancel()
		defer qmp.Close()
		held, err := runLinuxDragSteps(ctx, qmp, steps, drop.pointerMoves, deadline)
		if err == nil {
			return
		}
		logf("file drop: drag into the app stopped: %v", err)
		if held {
			releaseLinuxDragButton(qmp)
		}
	}()
	return nil
}

// releaseLinuxDragButton lets go of the tablet button after an interrupted
// drag, over a new connection if the drag's own connection broke.
func releaseLinuxDragButton(qmp pointerInput) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	release := json.RawMessage(`{"events":[{"type":"btn","data":{"down":false,"button":"left"}}]}`)
	if qmp.Call(ctx, "input-send-event", release, nil) == nil {
		return
	}
	fresh, err := dialQMPControl(ctx, qmpFwdPort)
	if err == nil {
		err = fresh.Call(ctx, "input-send-event", release, nil)
		fresh.Close()
	}
	if err != nil {
		logf("file drop: could not release the guest pointer button: %v", err)
	}
}

type pointerInput interface {
	Call(ctx context.Context, command string, arguments any, result any) error
}

// runLinuxDragSteps sends the steps and stops if the pointer moves on the
// host meanwhile or the drop gets too old. held reports whether the button
// may be left pressed: a press counts from before it is sent, since QEMU can
// apply it even when its reply is lost, and only a confirmed release clears it.
func runLinuxDragSteps(ctx context.Context, qmp pointerInput, steps []pointerStep, moves uint64, deadline time.Time) (held bool, err error) {
	for _, step := range steps {
		if dropPointerMoves.Load() != moves {
			return held, errors.New("the pointer moved during the drag")
		}
		if !time.Now().Before(deadline) {
			return held, errors.New("the drop got too old during the drag")
		}
		if step.button == 1 {
			held = true
		}
		if err := qmp.Call(ctx, "input-send-event", json.RawMessage(pointerStepArguments(step)), nil); err != nil {
			return held, err
		}
		if step.button == 0 {
			held = false
		}
		select {
		case <-time.After(step.pause):
		case <-ctx.Done():
			return held, ctx.Err()
		}
	}
	return held, nil
}
