//go:build linux

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

// A release served from memory: the same files and authenticated SHA256SUMS a
// real linux-vX.Y.Z release has, small enough for a unit test.
type releaseFixture struct {
	srv     *httptest.Server
	tag     string
	files   map[string][]byte
	sumsSHA string

	mu       sync.Mutex
	status   map[string]int // forced HTTP status per file name
	cutAt    int            // if > 0, the system image is cut off after this many bytes
	rootfsGT []string       // Range headers seen for the system image download
}

func sha(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }

func newReleaseFixture(t *testing.T, tag string, rootfs []byte) *releaseFixture {
	t.Helper()
	var zst bytes.Buffer
	enc, err := zstd.NewWriter(&zst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Write(rootfs); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	f := &releaseFixture{tag: tag, status: map[string]int{}, files: map[string][]byte{
		"build-spec.json":     []byte(`{"tag":"` + tag + `"}`),
		"vmlinuz-linux":       []byte("kernel " + tag),
		"initramfs-linux.img": []byte("initramfs " + tag),
		"rootfs.ext4":         rootfs,
		"rootfs.ext4.zst":     zst.Bytes(),
	}}
	type artifact struct {
		Bytes  int64  `json:"bytes"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	manifest := struct {
		SchemaVersion int        `json:"schemaVersion"`
		Artifacts     []artifact `json:"artifacts"`
	}{SchemaVersion: 1}
	for _, name := range []string{"build-spec.json", "vmlinuz-linux", "initramfs-linux.img", "rootfs.ext4", "rootfs.ext4.zst"} {
		manifest.Artifacts = append(manifest.Artifacts, artifact{Bytes: int64(len(f.files[name])), Path: name, SHA256: sha(f.files[name])})
	}
	f.files["guest-manifest.json"], _ = json.Marshal(manifest)
	var sums strings.Builder
	for _, name := range []string{"guest-manifest.json", "build-spec.json", "vmlinuz-linux", "initramfs-linux.img", "rootfs.ext4", "rootfs.ext4.zst"} {
		fmt.Fprintf(&sums, "%s  %s\n", sha(f.files[name]), name)
	}
	f.files["SHA256SUMS"] = []byte(sums.String())
	f.sumsSHA = sha(f.files["SHA256SUMS"])
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *releaseFixture) url() string { return f.srv.URL + "/" + f.tag }

func (f *releaseFixture) serve(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/"+f.tag+"/")
	f.mu.Lock()
	forced, cut := f.status[name], f.cutAt
	if name == "rootfs.ext4.zst" {
		f.rootfsGT = append(f.rootfsGT, r.Header.Get("Range"))
	}
	f.mu.Unlock()
	data, ok := f.files[name]
	switch {
	case forced != 0:
		http.Error(w, "forced", forced)
	case !ok:
		http.NotFound(w, r)
	case name == "rootfs.ext4.zst" && cut > 0:
		// Send part of the body, then drop the connection, as a flaky network does.
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		w.Write(data[:min(cut, len(data))])
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	default:
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	}
}

func (f *releaseFixture) set(fn func()) { f.mu.Lock(); fn(); f.mu.Unlock() }

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func newUpdateConfig(t *testing.T) *config {
	t.Helper()
	dir := t.TempDir()
	configureSetupCancellation(false)
	t.Cleanup(func() { configureSetupCancellation(false); getUI().setUpdating(false) })
	return &config{dir: dir, guestDir: filepath.Join(dir, "guest"), vmDir: filepath.Join(dir, "vm")}
}

func installedTag(t *testing.T, cfg *config) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cfg.guestDir, "build-spec.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLinuxGuestFirstSetupDownloadsVerifiesAndUnpacks(t *testing.T) {
	cfg := newUpdateConfig(t)
	rootfs := randomBytes(t, 3<<20)
	v1 := newReleaseFixture(t, "linux-v0.1.0", rootfs)
	if err := ensureLinuxGuest(cfg, v1.url(), v1.sumsSHA); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(cfg.guestDir, "rootfs.ext4"))
	if err != nil || !bytes.Equal(got, rootfs) {
		t.Fatalf("unpacked system image differs: %v", err)
	}
	if ok, err := installReceiptMatches(cfg.guestDir, v1.url(), v1.sumsSHA, installedGuestArtifacts); err != nil || !ok {
		t.Fatalf("install receipt: %v %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.guestDir, "rootfs.ext4.zst")); !os.IsNotExist(err) {
		t.Fatalf("the download archive should be removed once unpacked: %v", err)
	}
}

func TestLinuxGuestFirstSetupFailureIsAnErrorNotASilentSkip(t *testing.T) {
	cfg := newUpdateConfig(t)
	v1 := newReleaseFixture(t, "linux-v0.1.0", randomBytes(t, 1<<20))
	v1.set(func() { v1.status["SHA256SUMS"] = http.StatusNotFound })
	if err := ensureLinuxGuest(cfg, v1.url(), v1.sumsSHA); err == nil {
		t.Fatal("with no guest installed, a failed download must be reported")
	}
}

func installFirst(t *testing.T, cfg *config, rootfs []byte) *releaseFixture {
	t.Helper()
	v1 := newReleaseFixture(t, "linux-v0.1.0", rootfs)
	if err := ensureLinuxGuest(cfg, v1.url(), v1.sumsSHA); err != nil {
		t.Fatal(err)
	}
	return v1
}

func TestLinuxGuestUpdateSwapsInAndKeepsThePreviousUntilConfirmed(t *testing.T) {
	cfg := newUpdateConfig(t)
	v1 := installFirst(t, cfg, randomBytes(t, 2<<20))
	v2 := newReleaseFixture(t, "linux-v0.1.1", randomBytes(t, 2<<20))
	if err := ensureLinuxGuest(cfg, v2.url(), v2.sumsSHA); err != nil {
		t.Fatal(err)
	}
	if installedTag(t, cfg) != string(v2.files["build-spec.json"]) {
		t.Fatalf("the update was not installed: %s", installedTag(t, cfg))
	}
	previous, err := os.ReadFile(filepath.Join(cfg.dir, "guest.previous", "build-spec.json"))
	if err != nil || string(previous) != string(v1.files["build-spec.json"]) {
		t.Fatalf("the previous guest must stay until the update is confirmed: %q %v", previous, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.dir, "guest", linuxUpdateTargetFile)); !os.IsNotExist(err) {
		t.Fatalf("the staging marker must not travel into the installed guest: %v", err)
	}
	if state, err := readPayloadUpdateState(cfg.dir); err != nil || state == nil || !state.GuestPending {
		t.Fatalf("rollback record: %+v %v", state, err)
	}
	// A guest that never reports ready is rolled back to matching old files.
	rolledBack, err := rollbackPendingPayloadUpdates(cfg.dir)
	if err != nil || !rolledBack || installedTag(t, cfg) != string(v1.files["build-spec.json"]) {
		t.Fatalf("rollback: %v %v %s", rolledBack, err, installedTag(t, cfg))
	}
	if ok, err := installReceiptMatches(cfg.guestDir, v1.url(), v1.sumsSHA, installedGuestArtifacts); err != nil || !ok {
		t.Fatalf("after rollback the guest must be the complete, matching old set: %v %v", ok, err)
	}
}

func TestLinuxGuestUpdateKeepsItsProgressWhenTheDownloadIsInterrupted(t *testing.T) {
	cfg := newUpdateConfig(t)
	installFirst(t, cfg, randomBytes(t, 2<<20))
	v2 := newReleaseFixture(t, "linux-v0.1.1", randomBytes(t, 4<<20))
	half := len(v2.files["rootfs.ext4.zst"]) / 2
	v2.set(func() { v2.cutAt = half })
	linuxGUIEnabled = false
	// The connection drops mid-image on every attempt: the update is postponed.
	if err := ensureLinuxGuest(cfg, v2.url(), v2.sumsSHA); err != nil {
		t.Fatalf("an update that cannot finish must not stop Omarchy: %v", err)
	}
	part, err := os.Stat(filepath.Join(cfg.dir, "guest.next", "rootfs.ext4.zst.part"))
	if err != nil || part.Size() < int64(half)-1<<10 {
		t.Fatalf("the partial download was thrown away: %v %v", part, err)
	}
	if installedTag(t, cfg) != `{"tag":"linux-v0.1.0"}` {
		t.Fatal("the installed guest changed although the update did not finish")
	}
	// The next launch continues from those bytes instead of starting over.
	v2.set(func() { v2.cutAt = 0; v2.rootfsGT = nil })
	if err := ensureLinuxGuest(cfg, v2.url(), v2.sumsSHA); err != nil {
		t.Fatal(err)
	}
	if installedTag(t, cfg) != string(v2.files["build-spec.json"]) {
		t.Fatalf("the update did not complete: %s", installedTag(t, cfg))
	}
	v2.mu.Lock()
	ranges := append([]string(nil), v2.rootfsGT...)
	v2.mu.Unlock()
	if len(ranges) == 0 || !strings.HasPrefix(ranges[0], "bytes=") || ranges[0] == "bytes=0-" {
		t.Fatalf("the second attempt must resume, not restart: %q", ranges)
	}
}

func TestLinuxGuestUpdateThatCannotBeFetchedTellsThePersonAndStartsTheInstalledGuest(t *testing.T) {
	cfg := newUpdateConfig(t)
	installFirst(t, cfg, randomBytes(t, 1<<20))
	v2 := newReleaseFixture(t, "linux-v0.1.1", randomBytes(t, 1<<20))
	v2.set(func() { v2.status["SHA256SUMS"] = http.StatusNotFound })
	oldGUI, oldNotify := linuxGUIEnabled, linuxNotify
	defer func() { linuxGUIEnabled, linuxNotify = oldGUI, oldNotify }()
	linuxGUIEnabled = true
	var titles, bodies []string
	linuxNotify = func(id, title, body string) error {
		titles = append(titles, title)
		bodies = append(bodies, body)
		return nil
	}
	if err := ensureLinuxGuest(cfg, v2.url(), v2.sumsSHA); err != nil {
		t.Fatalf("postponing an update is not a failure: %v", err)
	}
	if len(titles) != 1 || titles[0] != "Omarchy update postponed" || !strings.Contains(bodies[0], "starting instead and nothing you saved changed") {
		t.Fatalf("notice: %q %q", titles, bodies)
	}
	if state, err := readPayloadUpdateState(cfg.dir); err != nil || state != nil {
		t.Fatalf("no rollback record may exist for an update that never started: %+v %v", state, err)
	}
	if installedTag(t, cfg) != `{"tag":"linux-v0.1.0"}` {
		t.Fatal("the installed guest changed")
	}
}

func TestLinuxGuestUpdateStagedForAnotherReleaseIsDiscarded(t *testing.T) {
	cfg := newUpdateConfig(t)
	installFirst(t, cfg, randomBytes(t, 1<<20))
	v2 := newReleaseFixture(t, "linux-v0.1.1", randomBytes(t, 2<<20))
	v2.set(func() { v2.cutAt = len(v2.files["rootfs.ext4.zst"]) / 2 })
	if err := ensureLinuxGuest(cfg, v2.url(), v2.sumsSHA); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(cfg.dir, "guest.next", "rootfs.ext4.zst.part")
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("expected staged progress for v0.1.1: %v", err)
	}
	// The app now pins v0.1.2. Bytes staged for v0.1.1 must not be reused.
	v3 := newReleaseFixture(t, "linux-v0.1.2", randomBytes(t, 1<<20))
	if err := ensureLinuxGuest(cfg, v3.url(), v3.sumsSHA); err != nil {
		t.Fatal(err)
	}
	if installedTag(t, cfg) != string(v3.files["build-spec.json"]) {
		t.Fatalf("expected v0.1.2 to install: %s", installedTag(t, cfg))
	}
	if data, err := os.ReadFile(filepath.Join(cfg.dir, "guest", "rootfs.ext4")); err != nil || !bytes.Equal(data, v3.files["rootfs.ext4"]) {
		t.Fatal("the installed system image mixes bytes from another release")
	}
}

func TestLinuxGuestUpdateCancelledKeepsItsStagedFiles(t *testing.T) {
	cfg := newUpdateConfig(t)
	installFirst(t, cfg, randomBytes(t, 1<<20))
	v2 := newReleaseFixture(t, "linux-v0.1.1", randomBytes(t, 2<<20))
	v2.set(func() { v2.cutAt = len(v2.files["rootfs.ext4.zst"]) / 2 })
	requestSetupCancel()
	err := ensureLinuxGuest(cfg, v2.url(), v2.sumsSHA)
	if !strings.Contains(fmt.Sprint(err), "cancel") {
		t.Fatalf("a cancelled update must report the cancellation, not postpone: %v", err)
	}
	if installedTag(t, cfg) != `{"tag":"linux-v0.1.0"}` {
		t.Fatal("cancelling must leave the installed guest alone")
	}
}
