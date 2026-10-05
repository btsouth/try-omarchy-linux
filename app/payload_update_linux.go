//go:build linux

package main

import (
	"os"
	"path/filepath"
)

// The Flatpak carries QEMU, so only the guest image updates in the data
// folder. The supervisor keeps an update once guest userspace reports ready.
// A launch that finds an update still unconfirmed restores the previous image
// and uses it for this launch; the next launch offers the update again.
func recoverLinuxGuestUpdate(dir string, release, sumsSHA256 *string) error {
	rolledBack, err := rollbackPendingPayloadUpdates(dir)
	if err != nil || !rolledBack {
		return err
	}
	var runtimeRelease, runtimeManifest string
	if err := pinRestoredPayloads(dir, release, sumsSHA256, &runtimeRelease, &runtimeManifest); err != nil {
		return err
	}
	logf("restored the previous guest image after an unconfirmed update")
	showLinuxRuntimeError(uiText("update.linux.title"), uiText("update.linux.restored_previous"))
	return nil
}

// Linux's Flatpak owns launcher/runtime rollback. Once the guest is confirmed,
// its previous unpacked image can be removed as before this Windows sync.
func commitLinuxGuestPayloadUpdate(dir string) {
	state, err := readPayloadUpdateState(dir)
	if err != nil || state == nil || !state.GuestPending {
		return
	}
	commitGuestPayloadUpdate(dir)
	state, err = readPayloadUpdateState(dir)
	if err == nil && (state == nil || !state.GuestPending) {
		_ = os.RemoveAll(filepath.Join(dir, "guest.previous"))
	}
}
