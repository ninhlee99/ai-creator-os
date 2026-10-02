package avatar

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockSidecar implements the sidecar HTTP contract for tests.
type mockSidecar struct {
	t            *testing.T
	expectLock   string
	renderCalled int
}

func (m *mockSidecar) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok", "model": "mock-musetalk", "device": "cpu", "version": "1",
		})
	})
	mux.HandleFunc("/render", func(w http.ResponseWriter, r *http.Request) {
		m.renderCalled++
		var req renderReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if m.expectLock != "" && req.IdentityLock != m.expectLock {
			http.Error(w, "identity lock mismatch", 400)
			return
		}
		if req.AudioWavB64 == "" || req.ReferenceImageB64 == "" {
			http.Error(w, "missing fields", 400)
			return
		}
		// Return a fake-but-plausible mp4 payload (>1KB so the size guard passes).
		fake := make([]byte, 2048)
		copy(fake, []byte("ftyp"))
		_ = json.NewEncoder(w).Encode(renderResp{
			VideoMP4B64:  base64.StdEncoding.EncodeToString(fake),
			DurationSec:  2.0,
			Frames:       50,
			IdentityLock: req.IdentityLock, // echo
		})
	})
	mux.HandleFunc("/stream/open", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openResp{SessionID: "sess-1"})
	})
	mux.HandleFunc("/stream/push", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
	})
	mux.HandleFunc("/stream/frame", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("session_id") != "sess-1" {
			http.Error(w, "bad session", 400)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xD9}) // minimal jpeg
	})
	mux.HandleFunc("/stream/close", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return mux
}

func testLocalProvider(t *testing.T, ms *mockSidecar) (*LocalAvatarProvider, Character) {
	t.Helper()
	srv := httptest.NewServer(ms.handler())
	t.Cleanup(srv.Close)
	p := NewLocalAvatarProvider(t.TempDir())
	p.baseURL = srv.URL
	imgPath := filepath.Join(t.TempDir(), "face.png")
	img := []byte("fake-image-bytes")
	if err := os.WriteFile(imgPath, img, 0o644); err != nil {
		t.Fatal(err)
	}
	ch := Character{Name: "MC", ReferenceImage: imgPath, Seed: 9}
	ch.IdentityLock = ComputeIdentityLock(img, 9)
	ms.expectLock = ch.IdentityLock
	return p, ch
}

func TestLocalHealthy(t *testing.T) {
	ms := &mockSidecar{}
	p, _ := testLocalProvider(t, ms)
	if !p.Healthy(context.Background()) {
		t.Error("mock sidecar must be healthy")
	}
	// dead sidecar → not healthy
	p2 := NewLocalAvatarProvider(t.TempDir())
	p2.baseURL = "http://127.0.0.1:1"
	if p2.Healthy(context.Background()) {
		t.Error("dead sidecar must not be healthy")
	}
}

func TestLocalRenderClip(t *testing.T) {
	ms := &mockSidecar{}
	p, ch := testLocalProvider(t, ms)
	outDir := t.TempDir()
	path, err := p.RenderClip(context.Background(), ch, make([]byte, 48000), RenderOpts{OutDir: outDir})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasSuffix(path, ".mp4") || !strings.HasPrefix(path, outDir) {
		t.Errorf("bad mp4 path: %q", path)
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() < 1024 {
		t.Errorf("mp4 not written properly: %v size=%d", err, st.Size())
	}
	if ms.renderCalled != 1 {
		t.Errorf("renderCalled = %d", ms.renderCalled)
	}
}

func TestLocalRenderRefusesWithoutSidecar(t *testing.T) {
	p := NewLocalAvatarProvider(t.TempDir())
	p.baseURL = "http://127.0.0.1:1" // nothing there
	imgPath := filepath.Join(t.TempDir(), "f.png")
	_ = os.WriteFile(imgPath, []byte("x"), 0o644)
	ch := Character{Name: "MC", ReferenceImage: imgPath, Seed: 1}
	ch.IdentityLock = ComputeIdentityLock([]byte("x"), 1)
	_, err := p.RenderClip(context.Background(), ch, []byte("wav"), RenderOpts{})
	if err == nil || !strings.Contains(err.Error(), "sidecar not running") {
		t.Errorf("expected honest sidecar-down error, got %v", err)
	}
}

func TestLocalStream(t *testing.T) {
	ms := &mockSidecar{}
	p, ch := testLocalProvider(t, ms)
	st, err := p.OpenStream(context.Background(), ch, StreamOpts{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if err := st.WriteAudio(make([]byte, 4800)); err != nil {
		t.Fatalf("push: %v", err)
	}
	frame, err := st.ReadFrame(context.Background())
	if err != nil {
		t.Fatalf("frame: %v", err)
	}
	if len(frame) < 4 || frame[0] != 0xFF || frame[1] != 0xD8 {
		t.Errorf("not a jpeg frame: %v", frame[:4])
	}
}

func TestSidecarLifecycleHonest(t *testing.T) {
	lc := newSidecarLifecycle(t.TempDir(), "http://127.0.0.1:18080")
	if lc.ModelConfigured() {
		t.Error("no AVATAR_MODEL_URL in test env — must be unconfigured")
	}
	if err := lc.EnsureModel(context.Background(), nil); err == nil ||
		!strings.Contains(err.Error(), "AVATAR_MODEL_URL") {
		t.Errorf("expected honest missing-URL error, got %v", err)
	}
	state, _ := lc.Status()
	if state != "missing" {
		t.Errorf("state = %q, want missing", state)
	}
	if err := lc.Start(context.Background()); err == nil {
		t.Error("start without binary must fail")
	}
}

// TestLocalSupportsRealtimeIsFalse pins the honest capability contract:
// the local provider is an offline renderer (minutes per 60s clip on an
// M1 Pro), so it must never claim realtime — the chain then only lets
// cloud tiers open live streams.
func TestLocalSupportsRealtimeIsFalse(t *testing.T) {
	p, _ := testLocalProvider(t, &mockSidecar{})
	if p.SupportsRealtime() {
		t.Error("local avatar provider must not claim realtime")
	}
}
