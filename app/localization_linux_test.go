//go:build linux

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useLinuxTestLanguage(t *testing.T, language string) {
	t.Helper()
	uiText("brand.name") // Complete the lazy initialization before swapping it.
	previous := activeUI
	catalogs, err := readUICatalogs(uiLocaleFiles)
	if err != nil {
		t.Fatal(err)
	}
	activeUI = uiTranslator{language: language, catalogs: catalogs}
	t.Cleanup(func() { activeUI = previous })
}

func TestLinuxEnglishScreensStayUnchanged(t *testing.T) {
	useLinuxTestLanguage(t, "en")
	about := linuxAboutState()
	if about.Status != "Try Omarchy for Linux "+linuxVersionLabel(linuxAppVersion)+" runs Omarchy in a virtual machine on this computer." || about.Sections[0].Heading != "Keyboard, mouse and window" {
		t.Fatalf("About changed: %+v", about)
	}
	consent := linuxClipboardConsentState()
	if consent.Title != "Share your clipboard with Omarchy?" || consent.Primary != "Continue" || consent.Secondary != "Not now" {
		t.Fatalf("clipboard question changed: %+v", consent)
	}
	stop := linuxForceStopState()
	if stop.Title != "Force stop Omarchy?" || stop.Status != "Omarchy has not shut down yet. Force stopping turns off the VM immediately. Unsaved work may be lost." {
		t.Fatalf("shutdown question changed: %+v", stop)
	}
	if got := linuxSnapshotRows(nil); got[0].Title != "No snapshots yet" || got[0].Detail != "Create one before trying something you may want to undo, such as a big update or a new setup." {
		t.Fatalf("snapshots changed: %+v", got)
	}
	for key, want := range map[string]string{"setup.cancel": "Cancel", "settings.tab.general": "General", "launcher.close": "Close"} {
		if got := linuxUILabels()[key]; got != want {
			t.Errorf("GTK %s = %q, want %q", key, got, want)
		}
	}
}

func TestLinuxChineseProgressUsesIdentityInsteadOfTranslatedText(t *testing.T) {
	useLinuxTestLanguage(t, "zh-Hans")
	// Existing contributed wording supplies this test-only fixture. The real
	// Chinese catalog is deliberately left unchanged. Reordered placeholders
	// and identical wording for different stages must not affect classification.
	chinese := activeUI.catalogs["zh-Hans"]
	checking := chinese["about.checking"]
	chinese["status.downloading_omarchy"] = checking + " {total} / {part}"
	for _, key := range []string{"status.checking_cached_file", "status.checking_downloaded_file", "status.resuming_file"} {
		chinese[key] = "{file} " + checking
	}
	for _, key := range []string{"status.checking_cached_system", "status.unpacking_system", "status.checking_unpacked_system", "status.ready_starting", "status.preparing_disk"} {
		chinese[key] = checking
	}
	u := &progressUI{lastPercent: -1}
	u.setCatalogStatus("status.downloading_omarchy", map[string]string{"part": "2", "total": "5"})
	u.setProgress(50, 100)
	if u.stage != stagePrepare || u.state.Total != 0 {
		t.Fatalf("small preparation download showed a bar: %+v", u.state)
	}
	u.setArtifactStatus(uiTextWith("status.downloading_omarchy", map[string]string{"part": "5", "total": "5"}), "/tmp/rootfs.ext4.zst")
	u.setProgress(50, 100)
	if u.stage != stageDownload || u.state.Total != 100 || u.state.Detail == "" {
		t.Fatalf("Chinese system download lost its bar: %+v", u.state)
	}
	for key, stage := range map[string]linuxStage{"status.checking_cached_system": stageCheck, "status.unpacking_system": stageUnpack, "status.checking_unpacked_system": stageCheck, "status.ready_starting": stagePrepare, "status.preparing_disk": stageDisk} {
		u.setCatalogStatus(key, nil)
		u.setProgress(50, 100)
		if u.stage != stage || (stage == stagePrepare) != (u.state.Total == 0) {
			t.Errorf("%s lost its stage/bar: %+v", key, u.state)
		}
	}
	// Exercise shared download code, including partial-file verification,
	// resume and final verification. It must retain stageCheck after completion.
	payload := []byte("prefix plus the remaining payload")
	dest := filepath.Join(t.TempDir(), "rootfs.ext4.zst")
	if err := os.WriteFile(dest+".part", payload[:7], 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=7-" {
			t.Errorf("unexpected resume range: %q", r.Header.Get("Range"))
		}
		u.mu.Lock()
		stage := u.stage
		u.mu.Unlock()
		if stage != stageDownload {
			t.Errorf("translated resume did not restore download stage: %d", stage)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 7-%d/%d", len(payload)-1, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[7:])
	}))
	defer server.Close()
	if err := ensureVerifiedDownload(server.Client(), server.URL, dest, testSHA256(payload), uiTextWith("status.downloading_omarchy", map[string]string{"part": "5", "total": "5"}), u); err != nil {
		t.Fatal(err)
	}
	if u.stage != stageCheck || u.state.Total != int64(len(payload)) {
		t.Fatalf("translated verification lost its bar: %+v (stage %d)", u.state, u.stage)
	}
	u.setUpdating(true)
	u.setArtifactStatus("", dest)
	if u.state.Status != "Downloading the Omarchy update" {
		t.Fatalf("update fallback changed: %q", u.state.Status)
	}
	if got := linuxUILabels()["about.title"]; !strings.Contains(got, "关于") {
		t.Fatalf("GTK did not receive Chinese catalog text: %q", got)
	}
}
