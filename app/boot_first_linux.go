//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

func linuxGuestPayloadNames() []string {
	return append([]string{"rootfs.ext4.zst"}, downloadedGuestArtifacts...)
}

// Boot intact installed guests before doing network work. The Flatpak updates
// the launcher and runtime; its embedded guest pin is the independent trust
// root even when automatic channel updates are disabled.
func configureLinuxGuestBootFirst(cfg *config, release, digest string) (func(), error) {
	noop := func() {}
	oldRelease, oldDigest, haveOld := installReceiptIdentity(cfg.guestDir)
	intact := false
	if haveOld {
		var err error
		intact, err = installReceiptMatches(cfg.guestDir, oldRelease, oldDigest, installedGuestArtifacts)
		if err != nil {
			return noop, err
		}
	}
	if cfg.fresh || !intact || releaseLocationsEquivalent(oldRelease, release) && oldDigest == normalizedSHA256(digest) {
		return noop, ensureLinuxGuest(cfg, release, digest)
	}
	root := updatePayloadRoot(cfg.dir, "", false)
	payload := filepath.Join(root, normalizedSHA256(digest))
	if _, err := os.Lstat(payload); err == nil {
		if err := verifyPayloadArtifacts(setupContext(), payload, digest, linuxGuestPayloadNames()); err == nil {
			local := *cfg
			local.payloadDir, local.localPayload, local.localPayloadSHA256 = root, true, normalizedSHA256(digest)
			if err := ensureLinuxGuest(&local, release, digest); err != nil {
				return noop, err
			}
			// Disk-full publication may defer the update and boot the old guest.
			if ready, _ := installReceiptMatches(cfg.guestDir, release, digest, installedGuestArtifacts); ready {
				return noop, nil
			}
		} else {
			logf("guest update: staged payload cannot be used: %v", err)
			if validateMovePath(payload) == nil {
				_ = os.RemoveAll(payload)
			}
		}
	}
	ctx, cancel := context.WithCancel(setupContext())
	done := make(chan struct{})
	var started atomic.Bool
	var once sync.Once
	cfg.startGuestUpdate = func() {
		once.Do(func() {
			started.Store(true)
			go func() {
				defer close(done)
				client := newDownloadClient()
				client.Transport = backgroundUpdateTransport{ctx: ctx, base: client.Transport}
				defer client.CloseIdleConnections()
				if err := pruneUpdatePayloads(root, normalizedSHA256(digest), oldDigest); err != nil {
					logf("guest update cache cleanup skipped: %v", err)
				}
				err := stagePayloadArtifacts(ctx, root, release, digest, client, nil, linuxGuestPayloadNames())
				if err != nil {
					if ctx.Err() == nil {
						logf("guest update staging postponed: %v", err)
						postponeLinuxGuestUpdate(err, cfg.dir)
					}
					return
				}
				if ctx.Err() == nil {
					logf("guest image %s verified and staged for the next start", releaseVersion(release))
					tellLinuxUser("guest-update", uiText("update.linux.staged"), uiText("update.linux.staged_detail"))
				}
			}()
		})
	}
	return func() {
		cancel()
		if started.Load() {
			<-done
		}
	}, nil
}
