//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLinuxDropPositionNeedsTheWholeWindow(t *testing.T) {
	event := `{"event":"DISPLAY_FILE_DROP","data":{"display":0,"files":["/tmp/a"],"x":640,"y":359,"width":1280,"height":720}}`
	if got := linuxDropPosition(event); len(got) != 4 || got[0] != 640 || got[1] != 359 || got[2] != 1280 || got[3] != 720 {
		t.Fatalf("position = %v", got)
	}
	if _, point, ok := droppedFilesEvent(event); !ok || point == nil || point[0] != 640 {
		t.Fatalf("shared parser rejected a positioned drop: %v %v", point, ok)
	}
	for _, data := range []string{
		`"files":["/tmp/a"]`,
		`"files":["/tmp/a"],"x":1,"y":1`,
		`"files":["/tmp/a"],"x":1280,"y":1,"width":1280,"height":720`,
		`"files":["/tmp/a"],"x":-1,"y":1,"width":1280,"height":720`,
		`"files":["/tmp/a"],"x":0,"y":0,"width":1,"height":720`,
		`"files":["/tmp/a"],"x":0,"y":0,"width":70000,"height":720`,
	} {
		if got := linuxDropPosition(`{"event":"DISPLAY_FILE_DROP","data":{"display":0,` + data + `}}`); got != nil {
			t.Fatalf("accepted %s as %v", data, got)
		}
	}
}

func TestLinuxDropPointerMovedEvent(t *testing.T) {
	if !linuxDropPointerMoved(`{"event":"DISPLAY_DROP_POINTER_MOVED","data":{"display":0},"timestamp":{"seconds":1,"microseconds":2}}`) {
		t.Fatal("pointer move was not recognized")
	}
	for _, line := range []string{`{"event":"DISPLAY_FILE_DROP","data":{}}`, `{"return":{}}`, `broken`} {
		if linuxDropPointerMoved(line) {
			t.Fatalf("accepted %s", line)
		}
	}
}

func TestLinuxDropDragRefusesAfterThePointerMoved(t *testing.T) {
	ready := guestReady.Load()
	guestReady.Store(true)
	defer guestReady.Store(ready)
	const id = "0123456789abcdef0123456789abcdef"
	recordDrop(id, recordedDrop{at: time.Now(), point: []int{500, 400, 1000, 800}, pointerMoves: dropPointerMoves.Load()})
	dropPointerMoves.Add(1)
	if err := performLinuxDropDrag(id, 16400, 16400); err == nil || !strings.Contains(err.Error(), "pointer moved") {
		t.Fatalf("drag after a pointer move: %v", err)
	}
	if err := performLinuxDropDrag(id, 16400, 16400); err == nil || !strings.Contains(err.Error(), "no recent drop") {
		t.Fatalf("a refused drop stayed available: %v", err)
	}
	guestReady.Store(false)
	recordDrop(id, recordedDrop{at: time.Now(), point: []int{500, 400, 1000, 800}, pointerMoves: dropPointerMoves.Load()})
	if err := performLinuxDropDrag(id, 16400, 16400); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("drag without a running guest: %v", err)
	}
}

type recordedPointer struct {
	calls  []string
	fail   int // fail the call with this 1-based index
	onCall func(int)
}

func (p *recordedPointer) Call(_ context.Context, command string, arguments any, _ any) error {
	data, _ := json.Marshal(arguments)
	p.calls = append(p.calls, command+" "+string(data))
	if p.onCall != nil {
		p.onCall(len(p.calls))
	}
	if len(p.calls) == p.fail {
		return errors.New("connection lost")
	}
	return nil
}

func TestLinuxDragStepsSendEveryStep(t *testing.T) {
	steps := []pointerStep{{x: 10, y: 20, button: -1}, {x: 10, y: 20, button: 1}, {x: 30, y: 40, button: -1}, {x: 30, y: 40, button: 0}}
	pointer := &recordedPointer{}
	held, err := runLinuxDragSteps(context.Background(), pointer, steps, dropPointerMoves.Load())
	if err != nil || held || len(pointer.calls) != len(steps) {
		t.Fatalf("held=%v err=%v calls=%v", held, err, pointer.calls)
	}
	want := `input-send-event {"events":[{"type":"abs","data":{"axis":"x","value":10}},{"type":"abs","data":{"axis":"y","value":20}},{"type":"btn","data":{"down":true,"button":"left"}}]}`
	if pointer.calls[1] != want {
		t.Fatalf("press = %s", pointer.calls[1])
	}
	if !strings.Contains(pointer.calls[3], `"down":false`) {
		t.Fatalf("drag did not release: %s", pointer.calls[3])
	}
}

func TestLinuxDragStopsWhenThePointerMovesMidDrag(t *testing.T) {
	steps := []pointerStep{{x: 10, y: 20, button: 1}, {x: 20, y: 30, button: -1}, {x: 30, y: 40, button: 0}}
	pointer := &recordedPointer{onCall: func(n int) {
		if n == 2 {
			dropPointerMoves.Add(1)
		}
	}}
	held, err := runLinuxDragSteps(context.Background(), pointer, steps, dropPointerMoves.Load())
	if err == nil || !held || len(pointer.calls) != 2 {
		t.Fatalf("held=%v err=%v calls=%v", held, err, pointer.calls)
	}
	releaseLinuxDragButton(pointer)
	if last := pointer.calls[len(pointer.calls)-1]; !strings.Contains(last, `"down":false`) || strings.Contains(last, "abs") {
		t.Fatalf("button not released: %s", last)
	}
	failing := &recordedPointer{fail: 2}
	if held, err := runLinuxDragSteps(context.Background(), failing, steps, dropPointerMoves.Load()); err == nil || !held {
		t.Fatalf("broken connection: held=%v err=%v", held, err)
	}
}
