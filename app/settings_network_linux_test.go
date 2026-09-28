//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLinuxNetworkFormPreservesOtherForwards(t *testing.T) {
	enabled, port, others := linuxNetworkForm([]string{"tcp:8080:80", "tcp:23456:22", "udp:5353:53"})
	if !enabled || port != "23456" || others != "tcp:8080:80\nudp:5353:53" {
		t.Fatalf("network form: %t %q %q", enabled, port, others)
	}
	text, err := linuxForwardsFromForm(enabled, port, others)
	if err != nil || text != "tcp:8080:80\nudp:5353:53\ntcp:23456:22" {
		t.Fatalf("saved forwards: %q %v", text, err)
	}
	for _, invalid := range []string{"", "23", "65536", "words"} {
		if _, err := linuxForwardsFromForm(true, invalid, ""); err == nil {
			t.Fatalf("accepted SSH port %q", invalid)
		}
	}
	if err := validateLinuxLocalForwards([]string{"tcp:0.0.0.0:8080:80"}); err == nil {
		t.Fatal("accepted a LAN forward in the Linux GUI")
	}
}

func TestLinuxNetworkSettingsHelper(t *testing.T) {
	if os.Getenv("TRY_OMARCHY_NETWORK_SETTINGS_HELPER") == "" {
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
		form.SSHEnabled = true
		form.StartAutomatically = true
		form.SSHPort = os.Getenv("TRY_OMARCHY_NETWORK_SSH_PORT")
		form.Forwards = "tcp:" + os.Getenv("TRY_OMARCHY_NETWORK_WEB_PORT") + ":80"
		value, err := json.Marshal(form)
		if err != nil {
			panic(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(value)}); err != nil {
			panic(err)
		}
	}
}

func TestLinuxNetworkSettingsReachGuestCommand(t *testing.T) {
	port := func() int {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		return listener.Addr().(*net.TCPAddr).Port
	}
	sshPort, webPort := port(), port()
	for webPort == sshPort {
		webPort = port()
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxNetworkSettingsHelper$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_NETWORK_SETTINGS_HELPER=1", fmt.Sprintf("TRY_OMARCHY_NETWORK_SSH_PORT=%d", sshPort), fmt.Sprintf("TRY_OMARCHY_NETWORK_WEB_PORT=%d", webPort))
	w := launchLinuxWindow(cmd, func() {})
	if w == nil {
		t.Fatal("network settings helper did not start")
	}
	defer w.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if message := showLinuxSettingsInWindow(ctx, w, dir, false); !strings.Contains(message, "Settings saved") {
		t.Fatalf("network settings were not saved: %q", message)
	}
	saved, err := loadSettings(settingsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Forwards) != 2 {
		t.Fatalf("forwards: %+v", saved.Forwards)
	}
	launch, err := loadLaunchPreferences(dir)
	if err != nil || !launch.StartAutomatically {
		t.Fatalf("startup choice: %+v %v", launch, err)
	}
	var forwards forwardList
	for _, value := range saved.Forwards {
		if err := forwards.Set(value); err != nil {
			t.Fatal(err)
		}
	}
	if !sshRequested(forwards) || !strings.Contains(sshCmdline(forwards, ""), "tryomarchy.sshd=1") {
		t.Fatal("saved SSH forward did not start guest sshd")
	}
	netdev := netdevArg(forwards)
	for _, value := range []string{strconv.Itoa(sshPort) + "-:22", strconv.Itoa(webPort) + "-:80"} {
		if !strings.Contains(netdev, value) {
			t.Fatalf("missing %q in %q", value, netdev)
		}
	}
}
