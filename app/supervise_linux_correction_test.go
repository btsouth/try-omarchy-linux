//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A running guest must see ACPI before QEMU exits when setup is cancelled.
// The fake QMP peer stops only after system_powerdown, so this also checks
// that supervision waits for the process instead of abandoning it.
func TestLinuxBootCancelRequestsCleanShutdown(t *testing.T) {
	dir := t.TempDir()
	oldQMP := qmpControlDirectory
	qmpControlDirectory = func() (string, error) { return filepath.Join(dir, "ipc"), nil }
	defer func() { qmpControlDirectory = oldQMP }()
	oldGUI := linuxGUIEnabled
	linuxGUIEnabled = true
	defer func() { linuxGUIEnabled = oldGUI }()
	oldUI := linuxUI
	linuxUI = newLinuxProgressUI()
	defer func() { linuxUI = oldUI }()
	configureSetupCancellation(false)
	defer configureSetupCancellation(false)
	fake := filepath.Join(dir, "fake-qemu")
	script := `#!/usr/bin/python3
import sys,socket,json,pathlib
root=pathlib.Path(__file__).parent
address=next(a.split(',')[0][5:] for a in sys.argv if a.startswith('unix:') and '/supervisor.sock,' in a)
s=socket.socket(socket.AF_UNIX);s.bind(address);s.listen(1)
c,_=s.accept();f=c.makefile('rwb',buffering=0)
f.write(b'{"QMP":{"version":{},"capabilities":[]}}\n')
for line in f:
 msg=json.loads(line);cmd=msg.get('execute')
 with (root/'commands').open('a') as log:log.write(cmd+'\n')
 f.write((json.dumps({'return':{},'id':msg.get('id')})+'\n').encode())
 if cmd=='qmp_capabilities':(root/'ready').touch()
 if cmd in ('quit','system_powerdown'):break
`
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
				time.Sleep(100 * time.Millisecond)
				requestSetupCancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		requestSetupCancel()
	}()
	cfg := &config{qemu: fake, dir: dir, vmDir: dir, guestDir: dir,
		disk: filepath.Join(dir, "disk.raw"), diskFormat: "raw", audio: "none", cpus: 1, memMiB: 1024}
	superviseLinux(cfg, "", make(chan os.Signal))
	<-done
	data, err := os.ReadFile(filepath.Join(dir, "commands"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\nquit\n") || !strings.Contains(string(data), "system_powerdown") {
		t.Fatalf("ordinary boot Cancel did not cleanly power down: %s", data)
	}
}
