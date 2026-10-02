package avatar

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
// docs/RESEARCH/avatar_pipeline.md); Go only speaks this contract and
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

// SupportsRealtime reports the contract capability. Whether frames arrive
// at wall-clock speed depends on the sidecar's fps on the owner's machine
// (MuseTalk-class models do NOT reach realtime on M1 Pro — see the
// FrameStream contract); the stream engine must pace accordingly.
func (p *LocalAvatarProvider) SupportsRealtime() bool { return true }

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
func (p *LocalAvatarProvider) OpenStream(ctx context.Context, ch Character, opts StreamOpts) (FrameStream, error) {
	if !p.Healthy(ctx) {
		return nil, fmt.Errorf("local avatar: sidecar not running at %s — start it from the dashboard", p.baseURL)
	}
	img, err := os.ReadFile(ch.ReferenceImage)
	if err != nil {
		return nil, fmt.Errorf("local avatar: read reference image: %w", err)
	}
	seed := ch.Seed
	if opts.Seed != 0 {
		seed = opts.Seed
	}
	var opened openResp
	err = p.post(ctx, "/stream/open", openReq{
		AvatarID:          fmt.Sprintf("char-%d", ch.ID),
		ReferenceImageB64: base64.StdEncoding.EncodeToString(img),
		Seed:              seed,
		IdentityLock:      ComputeIdentityLock(img, seed),
		Width:             orDefault(opts.Width, 720),
		Height:            orDefault(opts.Height, 1280),
		FPS:               orDefault(opts.FPS, 25),
	}, &opened)
	if err != nil {
		return nil, fmt.Errorf("local avatar: stream open: %w", err)
	}
	if opened.SessionID == "" {
		return nil, fmt.Errorf("local avatar: sidecar returned no session_id")
	}
	return &sidecarStream{p: p, sessionID: opened.SessionID}, nil
}

// sidecarStream implements FrameStream over the sidecar's stream endpoints.
type sidecarStream struct {
	p         *LocalAvatarProvider
	sessionID string
	closed    bool
	mu        sync.Mutex
}

func (s *sidecarStream) WriteAudio(pcm []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("stream closed")
	}
	if len(pcm) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var ack map[string]any
	return s.p.post(ctx, "/stream/push", pushReq{
		SessionID: s.sessionID,
		PCMB64:    base64.StdEncoding.EncodeToString(pcm),
	}, &ack)
}

func (s *sidecarStream) ReadFrame(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("stream closed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.p.baseURL+"/stream/frame?session_id="+s.sessionID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, errNoFrameYet
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stream frame: HTTP %d", resp.StatusCode)
	}
	frame, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if len(frame) == 0 {
		return nil, errNoFrameYet
	}
	return frame, nil
}

func (s *sidecarStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var ack map[string]any
	_ = s.p.post(ctx, "/stream/close", map[string]string{"session_id": s.sessionID}, &ack)
	return nil
}

// errNoFrameYet signals the renderer has not produced this frame yet; the
// caller should wait for the next tick, not treat it as fatal.
var errNoFrameYet = fmt.Errorf("no frame yet")

// ---------------------------------------------------------------------------
// HTTP plumbing

type healthResp struct {
	Status  string `json:"status"`
	Model   string `json:"model"`
	Device  string `json:"device"`
	Version string `json:"version"`
}

type renderReq struct {
	AvatarID          string            `json:"avatar_id"`
	ReferenceImageB64 string            `json:"reference_image_b64"`
	AudioWavB64       string            `json:"audio_wav_b64"`
	EmotionCues       []TimedEmotionCue `json:"emotion_cues"`
	Seed              int64             `json:"seed"`
	IdentityLock      string            `json:"identity_lock"`
	Width             int               `json:"width"`
	Height            int               `json:"height"`
	FPS               int               `json:"fps"`
}

type renderResp struct {
	VideoMP4B64  string  `json:"video_mp4_b64"`
	DurationSec  float64 `json:"duration_sec"`
	Frames       int     `json:"frames"`
	IdentityLock string  `json:"identity_lock"`
}

type openReq struct {
	AvatarID          string `json:"avatar_id"`
	ReferenceImageB64 string `json:"reference_image_b64"`
	Seed              int64  `json:"seed"`
	IdentityLock      string `json:"identity_lock"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	FPS               int    `json:"fps"`
}

type openResp struct {
	SessionID string `json:"session_id"`
}

type pushReq struct {
	SessionID string `json:"session_id"`
	PCMB64    string `json:"pcm_b64"`
}

func (p *LocalAvatarProvider) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp, out)
}

func (p *LocalAvatarProvider) post(ctx context.Context, path string, body any, out any) error {
	return p.postWith(ctx, p.client, path, body, out)
}

// postRender is post() against /render with the long-timeout client (see
// the renderClient field). Falls back to the shared client when the
// provider was built as a struct literal (tests).
func (p *LocalAvatarProvider) postRender(ctx context.Context, body any, out any) error {
	c := p.renderClient
	if c == nil {
		c = p.client
	}
	return p.postWith(ctx, c, "/render", body, out)
}

func (p *LocalAvatarProvider) postWith(ctx context.Context, client *http.Client, path string, body any, out any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp, out)
}

func decodeJSON(resp *http.Response, out any) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("bad JSON: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}
