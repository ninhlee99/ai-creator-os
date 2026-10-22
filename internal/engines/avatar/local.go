//go:build parked

package avatar

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Default sidecar endpoint. Override with AVATAR_SIDECAR_URL.
const defaultSidecarURL = "http://127.0.0.1:18080"

// Sidecar HTTP contract (v1). The sidecar is third-party software (default:
// a MuseTalk v1.5 based server with an MPS/Apple-Silicon port — see
// docs/RESEARCH.md §6); Go only speaks this contract and
// manages the process. Endpoints:
//
//	GET  /health → 200 {"status":"ok","model":"<id>","device":"mps","version":"1"}
//	POST /warmup {"avatar_id","reference_image_b64","seed"}
//	  → 200 {"avatar_id":"...","cached":true}
//	  Optional but recommended: caches the avatar so repeated renders skip
//	  the image upload and face-alignment work.
//	POST /render {"avatar_id"|"reference_image_b64","audio_wav_b64",
//	              "emotion_cues","seed","identity_lock","width","height","fps"}
//	  → 200 {"video_mp4_b64":"...","duration_sec":12.5,"frames":312,
//	         "identity_lock":"<echo>"}
//	POST /stream/open  {"avatar_id"|"reference_image_b64","seed",
//	                   "identity_lock","width","height","fps"}
//	  → 200 {"session_id":"..."}
//	POST /stream/push  {"session_id":"...","pcm_b64":"<s16le mono 24kHz>"}
//	  → 200 {"accepted":true}
//	GET  /stream/frame?session_id=... → 200 image/jpeg | 204 no frame yet
//	POST /stream/close {"session_id":"..."} → 200
//
// HARD CONTRACT on /render and /stream/frame: every frame is synthesized
// from (identity, audio at that timestamp). Static images with pan/zoom or
// crossfades are a contract violation — the Go client rejects responses
// whose identity_lock echo does not match.

// LocalAvatarProvider is the free default tier: it drives the sidecar over
// HTTP. It never fabricates frames: when the sidecar is not running,
// RenderClip/OpenStream fail with a clear error telling the owner to start
// it from the dashboard.
type LocalAvatarProvider struct {
	mu      sync.RWMutex
	baseURL string
	client  *http.Client
	// renderClient serves /render only. Offline renders take minutes on
	// M1 (MuseTalk-class, ~2.5–4 fps), far beyond the shared client's
	// 60s timeout — with the shared client every real render died at
	// 60s even though the chain allows 30m. The caller's ctx (the
	// chain's per-attempt timeout) still bounds the wait.
	renderClient *http.Client
	enabled      bool

	// lifecycle (see sidecar.go)
	lc *sidecarLifecycle
}

// NewLocalAvatarProvider builds the local tier. dataDir is the aicos data
// directory (models + third-party sidecars live under it).
func NewLocalAvatarProvider(dataDir string) *LocalAvatarProvider {
	url := os.Getenv("AVATAR_SIDECAR_URL")
	if url == "" {
		url = defaultSidecarURL
	}
	return &LocalAvatarProvider{
		baseURL:      url,
		client:       &http.Client{Timeout: 60 * time.Second},
		renderClient: &http.Client{Timeout: 30 * time.Minute},
		enabled:      true,
		lc:           newSidecarLifecycle(dataDir, url),
	}
}

func (p *LocalAvatarProvider) Name() string { return "local" }

// SupportsRealtime is false on purpose: the local provider is an OFFLINE
// renderer. A MuseTalk-class sidecar renders a 60s clip in minutes on an
// M1 Pro — far from wall-clock fps (see docs/MAC_M1_CORE_AUDIT.md) — so
// its frames can never feed a live stream, and the chain must not treat
// a paced sidecar session as realtime. Only cloud tiers (HeyGen/D-ID)
// report true; OpenStream stays implemented for the interface contract
// but is unreachable through the chain while this reports false.
func (p *LocalAvatarProvider) SupportsRealtime() bool { return false }

func (p *LocalAvatarProvider) SetEnabled(b bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = b
}

// Healthy is true only when the sidecar process is up AND /health answers.
// A missing sidecar is not healthy — the chain then fails over honestly.
func (p *LocalAvatarProvider) Healthy(ctx context.Context) bool {
	p.mu.RLock()
	enabled := p.enabled
	p.mu.RUnlock()
	if !enabled {
		return false
	}
	var hs healthResp
	if err := p.get(ctx, "/health", &hs); err != nil {
		return false
	}
	return hs.Status == "ok"
}

// RenderClip POSTs the render job to the sidecar and writes the returned
// mp4 to outDir (default <dataDir>/output). The identity_lock echo is
// verified: a mismatch means the sidecar rendered the wrong identity.
func (p *LocalAvatarProvider) RenderClip(ctx context.Context, ch Character, audioWAV []byte, opts RenderOpts) (string, error) {
	p.mu.RLock()
	enabled := p.enabled
	p.mu.RUnlock()
	if !enabled {
		return "", fmt.Errorf("local avatar: provider disabled")
	}
	if len(audioWAV) == 0 {
		return "", fmt.Errorf("local avatar: empty audio")
	}
	if !p.Healthy(ctx) {
		return "", fmt.Errorf("local avatar: sidecar not running at %s — start it from the dashboard (Settings → Avatar sidecar)", p.baseURL)
	}
	img, err := os.ReadFile(ch.ReferenceImage)
	if err != nil {
		return "", fmt.Errorf("local avatar: read reference image: %w", err)
	}
	seed := ch.Seed
	if opts.Seed != 0 {
		seed = opts.Seed
	}
	lock := ComputeIdentityLock(img, seed)

	req := renderReq{
		AvatarID:          fmt.Sprintf("char-%d", ch.ID),
		ReferenceImageB64: base64.StdEncoding.EncodeToString(img),
		AudioWavB64:       base64.StdEncoding.EncodeToString(audioWAV),
		EmotionCues:       opts.Emotions,
		Seed:              seed,
		IdentityLock:      lock,
		Width:             orDefault(opts.Width, 720),
		Height:            orDefault(opts.Height, 1280),
		FPS:               orDefault(opts.FPS, 25),
	}
	var resp renderResp
	if err := p.postRender(ctx, req, &resp); err != nil {
		return "", fmt.Errorf("local avatar: render: %w", err)
	}
	if resp.IdentityLock != "" && resp.IdentityLock != lock {
		return "", fmt.Errorf("local avatar: sidecar identity_lock echo mismatch — refusing output (wrong identity?)")
	}
	if resp.VideoMP4B64 == "" {
		return "", fmt.Errorf("local avatar: sidecar returned no video")
	}
	mp4, err := base64.StdEncoding.DecodeString(resp.VideoMP4B64)
	if err != nil {
		return "", fmt.Errorf("local avatar: decode mp4: %w", err)
	}
	if len(mp4) < 1024 {
		return "", fmt.Errorf("local avatar: sidecar returned suspiciously small video (%d bytes)", len(mp4))
	}
	outDir := opts.OutDir
	if outDir == "" {
		outDir = p.lc.outputDir()
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(outDir, fmt.Sprintf("avatar-%d-%d.mp4", ch.ID, time.Now().Unix()))
	if err := os.WriteFile(path, mp4, 0o644); err != nil {
		return "", fmt.Errorf("local avatar: write mp4: %w", err)
	}
	return path, nil
}

// OpenStream opens a frame-by-frame session against the sidecar.
