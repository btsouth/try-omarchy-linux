//go:build windows

package main

// Structured status metadata is only used by Linux's friendly progress UI.
// Windows receives exactly the same resolved text as setStatus did before.
func (u *progressUI) setCatalogStatus(key string, values map[string]string) {
	u.setStatus("%s", uiStatusText(key, values))
}

func (u *progressUI) setArtifactStatus(text, path string) {
	u.setStatus("%s", text)
}
