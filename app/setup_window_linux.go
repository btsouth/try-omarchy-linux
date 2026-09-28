//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Keep the protocol here and in linux-ui/main.go small: full snapshots in,
// ready/cancel/dismissed/reply events out. GTK is a separate executable and module.
type linuxSetupState struct {
	Status         string             `json:"status"`
	Current        int64              `json:"current"`
	Total          int64              `json:"total"`
	Error          bool               `json:"error"`
	ErrorTitle     string             `json:"errorTitle,omitempty"`
	Booting        bool               `json:"booting,omitempty"`
	NonCancellable bool               `json:"nonCancellable,omitempty"`
	Prompt         string             `json:"prompt,omitempty"`
	Request        uint64             `json:"request,omitempty"`
	Path           string             `json:"path,omitempty"`
	Version        string             `json:"version,omitempty"`
	CanAttach      bool               `json:"canAttach,omitempty"`
	CanForget      bool               `json:"canForget,omitempty"`
	CanDelete      bool               `json:"canDelete,omitempty"`
	Settings       *linuxSettingsForm `json:"settings,omitempty"`
	// Plain-language content laid out by the window: a one-line headline, a
	// notice shown as a banner (with an optional help link), and headed rows.
	Headline string         `json:"headline,omitempty"`
	Notice   string         `json:"notice,omitempty"`
	HelpURL  string         `json:"helpUrl,omitempty"`
	Sections []linuxSection `json:"sections,omitempty"`
	// Progress detail under the bar, and whether an error page offers Try again.
	Detail   string `json:"detail,omitempty"`
	CanRetry bool   `json:"canRetry,omitempty"`
	// The home replaces Launch with Check again while KVM is unavailable.
	CheckAgain bool `json:"checkAgain,omitempty"`
	Installed  bool `json:"installed,omitempty"`
	// Setup is "customize" when a first setup cannot use the default location.
	Setup string `json:"setup,omitempty"`
	// The home's buttons are decided here: Actions in the footer, Menu behind
	// the header's overflow button. Each reply is what the window sends back.
	Actions []linuxAction `json:"actions,omitempty"`
	Menu    []linuxAction `json:"menu,omitempty"`
	// A "choice" prompt: Primary replies "primary", Secondary "secondary".
	Title       string `json:"title,omitempty"`
	Primary     string `json:"primary,omitempty"`
	Secondary   string `json:"secondary,omitempty"`
	Destructive bool   `json:"destructive,omitempty"`
	// Backup and recovery actions available for the selected VM.
	CanMove       bool `json:"canMove,omitempty"`
	CanReset      bool `json:"canReset,omitempty"`
	CanCleanMove  bool `json:"canCleanMove,omitempty"`
	CanCleanReset bool `json:"canCleanReset,omitempty"`
}

type linuxAction struct {
	Label       string `json:"label"`
	Reply       string `json:"reply"`
	Suggested   bool   `json:"suggested,omitempty"`
	Destructive bool   `json:"destructive,omitempty"`
}

type linuxSetupReply struct {
	Event   string `json:"event"`
	Request uint64 `json:"request"`
	Value   string `json:"value"`
}

type linuxSetupWindow struct {
	replies  chan linuxSetupReply
	serial   atomic.Uint64
	cancel   func()
	input    io.WriteCloser
	updates  chan linuxSetupState
	done     chan struct{}
	expected atomic.Bool
	stopOnce sync.Once
	cmd      *exec.Cmd
}

func startLinuxSetupWindow() *linuxSetupWindow {
	return startLinuxWindow(requestSetupCancel)
}

func startLinuxWindow(cancel func()) *linuxSetupWindow {
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	// Resolve beside the launcher, never from a writable current directory.
	return launchLinuxWindow(exec.Command(filepath.Join(filepath.Dir(self), "try-omarchy-setup")), cancel)
}

func launchLinuxSetupWindow(cmd *exec.Cmd) *linuxSetupWindow {
	return launchLinuxWindow(cmd, requestSetupCancel)
}

func launchLinuxWindow(cmd *exec.Cmd, cancel func()) *linuxSetupWindow {
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		logf("Setup window unavailable: %v; using terminal status", err)
		return nil
	}
	w := &linuxSetupWindow{cancel: cancel, replies: make(chan linuxSetupReply, 8), input: input, updates: make(chan linuxSetupState, 1), done: make(chan struct{}), cmd: cmd}
	ready := make(chan struct{})
	go func() {
		defer close(w.done)
		scanner := bufio.NewScanner(output)
		isReady := false
		for scanner.Scan() {
			var event linuxSetupReply
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				break
			}
			switch event.Event {
			case "ready":
				if !isReady {
					isReady = true
					close(ready)
				}
			case "cancel":
				w.cancel()
			case "reply":
				select {
				case w.replies <- event:
				default:
				}
			case "dismissed":
				w.expected.Store(true)
			}
		}
		// A helper that closes stdout without exiting must not hang the launcher.
		cmd.Process.Kill()
		cmd.Wait()
		input.Close()
		if isReady && !w.expected.Load() {
			w.cancel()
		}
	}()
	select {
	case <-ready:
	case <-w.done:
		return nil
	case <-time.After(10 * time.Second):
		w.stop()
		return nil
	}
	go func() {
		encoder := json.NewEncoder(input)
		for {
			select {
			case state := <-w.updates:
				if encoder.Encode(state) != nil {
					return
				}
			case <-w.done:
				return
			}
		}
	}()
	return w
}

// Called under progressUI.mu. Coalescing keeps a stalled helper from blocking
// verification or disk writes, while retaining the latest complete state.
func (w *linuxSetupWindow) update(state linuxSetupState) {
	select {
	case <-w.updates:
	default:
	}
	select {
	case w.updates <- state:
	default:
	}
}

func (w *linuxSetupWindow) stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		w.expected.Store(true)
		w.input.Close()
		select {
		case <-w.done:
		case <-time.After(2 * time.Second):
			w.cmd.Process.Kill()
			<-w.done
		}
	})
}

// Prompts run while the caller has paused setup. Request IDs reject late or
// duplicate replies, including a folder dialog closing after cancellation.
func (w *linuxSetupWindow) ask(ctx context.Context, state linuxSetupState) (string, error) {
	state.Request = w.serial.Add(1)
	w.update(state)
	for {
		select {
		case reply := <-w.replies:
			if reply.Request == state.Request {
				return reply.Value, nil
			}
		case <-w.done:
			return "", fmt.Errorf("setup window closed")
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}
