//go:build linux

package main

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
	showLinuxRuntimeError("Omarchy update", "The updated Omarchy image did not finish starting last time, so Try Omarchy restored the previous image. Your files stay in place. Try Omarchy will try the update again next time.")
	return nil
}
