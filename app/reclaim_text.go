package main

// Reclaim wording that names the host. The guest agent and request path are
// shared; the "@linux" catalog variants say where the space returns to there.
func reclaimReadyStatus() string        { return uiText("reclaim.status.finished") }
func reclaimNeedsSpaceMessage() string  { return uiText("reclaim.error.low_space") }
func reclaimUnsupportedMessage() string { return uiText("reclaim.error.unsupported") }
func reclaimStartedMessage() string     { return uiText("reclaim.started") }
