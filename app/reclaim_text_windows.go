//go:build windows

package main

// Reclaim wording that names the host. The guest agent and request path are
// shared; only the place the space returns to differs.
const (
	reclaimReadyStatus        = "Preparation finished. Shut down Omarchy to return the space to Windows."
	reclaimNeedsSpaceMessage  = "Reclaim needs at least 4.25 GiB free on the Windows drive."
	reclaimUnsupportedMessage = "Reclaim is available for standard raw disks only."
	reclaimStartedMessage     = "Preparing free space. Check Reclaim status in the tray before shutting down."
)
