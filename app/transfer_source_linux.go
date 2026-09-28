//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Flatpak exposes granted documents through XDG_RUNTIME_DIR/doc, which is a
// symlink into its private document mount. Resolve that one portal-owned link
// before applying the archive's normal no-links rule. Other symlinks remain
// rejected, including links that escape the granted document mount.
func linuxTransferSource(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return "", errors.New("invalid file path")
	}
	clean := filepath.Clean(path)
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if filepath.IsAbs(runtime) {
		portalRoot := filepath.Join(runtime, "doc")
		if withinLinuxPath(portalRoot, clean) {
			root, err := filepath.EvalSymlinks(portalRoot)
			if err != nil {
				return "", fmt.Errorf("document portal is unavailable: %w", err)
			}
			resolved, err := filepath.EvalSymlinks(clean)
			if err != nil {
				return "", err
			}
			if !withinLinuxPath(root, resolved) {
				return "", errors.New("chosen file is outside the document grant")
			}
			clean = resolved
		}
	}
	if err := validateMovePath(clean); err != nil {
		return "", err
	}
	if _, err := os.Stat(clean); err != nil {
		return "", err
	}
	return clean, nil
}

func withinLinuxPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
