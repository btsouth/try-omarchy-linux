//go:build !windows

package main

// Reclaim wording that names the host. The guest agent and request path are
// shared; only the place the space returns to differs.
const (
	reclaimReadyStatus        = "Preparation finished. Shut down Omarchy to give the space back to this computer."
	reclaimNeedsSpaceMessage  = "Reclaim needs more than 4 GB free on the drive that holds Omarchy. Free some space, then try again."
	reclaimUnsupportedMessage = "The drive that holds Omarchy cannot give unused space back, so Reclaim is unavailable."
	reclaimStartedMessage     = "Preparing free space. Keep Omarchy running until it finishes, then shut it down to give the space back."
)
