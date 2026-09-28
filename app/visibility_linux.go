//go:build linux

package main

import (
	"encoding/json"
	"time"
)

// Only the patched primary SDL window can renew the lease. Old runtimes and
// lost QMP connections leave guest idle behavior alone.
type linuxVisibility struct {
	visible  bool
	reported time.Time
}

func (v *linuxVisibility) receive(line string, now time.Time) bool {
	var event struct {
		Event string `json:"event"`
		Data  struct {
			Console *int  `json:"console"`
			Visible *bool `json:"visible"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(line), &event) != nil || event.Event != "DISPLAY_VISIBILITY" || event.Data.Console == nil || *event.Data.Console != 0 || event.Data.Visible == nil {
		return false
	}
	v.visible, v.reported = *event.Data.Visible, now
	return true
}

func (v *linuxVisibility) command(now time.Time) string {
	if v.visible && now.Sub(v.reported) < 15*time.Second {
		return "host-window visible"
	}
	return "host-window hidden"
}

func sendLinuxVisibility(v *linuxVisibility) {
	if a := theAgent.Load(); a != nil {
		a.sendLine(v.command(time.Now()) + "\n")
	}
}
