package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/engines/local"
)

// ServerStatus describes the VieNeu sidecar for the dashboard.
type ServerStatus struct {
	State  string // "stopped" | "downloading" | "running" | "error"
	Detail string
	Uptime time.Duration
}

// persona voice style -> VieNeu v3 Turbo preset name.
// Preset names verified from the VieNeu-TTS README (25 presets, 3 regions);
// "Hải Đăng" is the server's own default. Unknown voice strings are passed
// through verbatim so real preset names (e.g. "Mai Anh") keep working.
var vieNeuVoices = map[string]string{
	"default":          "Hải Đăng", // server default, editors' pick
	"warm-female":      "Mai Anh",
	"clear-teacher":    "Thùy Dung",
	"singer":           "Trúc Ly",
	"upbeat-host":      "Ngọc Huyền",
	"energetic-caster": "Adam",     // southern male, high energy
	"male":             "Minh Đức", // northern male
}

// VieNeuVoice is one entry of GET /v1/voices.
type VieNeuVoice struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Gender      string   `json:"gender"`
	Featured    bool     `json:"featured"`
	Aliases     []string `json:"aliases"`
}

// VieNeuProvider is the tier-2 TTS provider: VieNeu-TTS v3 Turbo served by
// its own OpenAI-compatible API server, managed as a supervised sidecar
// subprocess (like FFmpeg). Go never imports Python; it only supervises the
// third-party process and talks HTTP to it.
type VieNeuProvider struct {
	mu      sync.RWMutex
	preset  string // runtime default preset (SetVoice); "" = server default
	dataDir string
	baseURL string
	reg     *local.Registry
	proc    *local.Process

	startTimeout time.Duration // default 10m (first start downloads ~334MB)
	http         *http.Client

	downloading atomic.Bool
	note        atomic.Value // string: note about the last output format
}

// NewVieNeuProvider builds the tier-2 TTS provider. The sidecar is NOT
// started automatically — call Start (or EnsureModel) explicitly, e.g. from
// the dashboard or the orchestrator.
func NewVieNeuProvider(dataDir string, reg *local.Registry) *VieNeuProvider {
	if reg == nil {
		reg = local.NewRegistry(dataDir)
	}
	return &VieNeuProvider{
		dataDir: dataDir,
		baseURL: "http://127.0.0.1:8000",
		reg:     reg,
		http:    &http.Client{},
	}
}

func (v *VieNeuProvider) Name() string { return "vieneu" }

// SetBaseURL overrides the sidecar URL (default http://127.0.0.1:8000).
func (v *VieNeuProvider) SetBaseURL(u string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.baseURL = strings.TrimSuffix(u, "/")
}

func (v *VieNeuProvider) getBaseURL() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.baseURL
}

// Healthy is false when the sidecar repo has not been fetched yet ("chưa tải
// VieNeu") or when GET /health does not report ok.
func (v *VieNeuProvider) Healthy(ctx context.Context) bool {
	if !v.reg.VieNeuPresent() {
		return false
	}
	return v.ping(ctx) == nil
}

func (v *VieNeuProvider) ping(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, v.getBaseURL()+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// /health -> {"status":"ok",...} (200) or {"status":"error",...} (503).
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&body); err == nil &&
		body.Status != "" && body.Status != "ok" {
		return fmt.Errorf("status=%s", body.Status)
	}
	return nil
}

// SetVoice changes the default voice preset at runtime (Bắc/Trung/Nam).
// It applies to the next Synthesize call whose voice is "" or "default".
// Pass any preset name from GET /v1/voices, e.g. "Hải Đăng", "Mai Anh".
func (v *VieNeuProvider) SetVoice(preset string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.preset = preset
}

// Voice returns the current runtime default preset.
func (v *VieNeuProvider) Voice() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.preset != "" {
		return v.preset
	}
	return vieNeuVoices["default"]
}

func (v *VieNeuProvider) resolveVoice(voice string) string {
	if voice == "" || voice == "default" {
		return v.Voice()
	}
	if m, ok := vieNeuVoices[voice]; ok {
		return m
	}
	return voice // real preset name passed through verbatim
}

// Synthesize POSTs to the sidecar's OpenAI-compatible /v1/audio/speech and
// returns WAV bytes. Emotion cues like [cười], [thở dài], [hắng giọng] are
// passed through VERBATIM in "input" — never stripped or altered.
func (v *VieNeuProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("vieneu: empty text")
	}
	if !v.reg.VieNeuPresent() {
		return nil, fmt.Errorf("vieneu: chưa tải VieNeu (%s) — gọi Start() trước để tải repo", v.reg.VieNeuDir())
	}
	if err := v.ping(ctx); err != nil {
		return nil, fmt.Errorf("vieneu: sidecar chưa chạy tại %s (%v) — gọi Start() trước", v.getBaseURL(), err)
	}
	preset := v.resolveVoice(voice)

	sr24 := 24000
	body := v.speechBody(text, preset, &sr24)
	wav, status, err := v.postSpeech(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("vieneu: %w", err)
	}
	if status == http.StatusBadRequest {
		// Old server without sample_rate support: retry plain, resample locally.
		wav, status, err = v.postSpeech(ctx, v.speechBody(text, preset, nil))
		if err != nil {
			return nil, fmt.Errorf("vieneu: %w", err)
		}
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("vieneu: HTTP %d: %s", status, truncate(string(wav), 200))
	}
	return v.normalizeWav(ctx, wav)
}

// speechBody builds the OpenAI-compatible request. The server natively
// supports sample_rate=24000 ("OpenAI's pcm rate"), so we ask for 24 kHz
// directly and usually skip ffmpeg entirely.
func (v *VieNeuProvider) speechBody(text, preset string, sampleRate *int) map[string]any {
	body := map[string]any{
		"model":           "vieneu-v3-turbo",
		"input":           text, // emotion cues verbatim — never modified
		"voice":           preset,
		"response_format": "wav",
	}
	if sampleRate != nil {
		body["sample_rate"] = *sampleRate
	}
	return body
}

func (v *VieNeuProvider) postSpeech(ctx context.Context, body map[string]any) ([]byte, int, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, 0, err
	}
	// Local CPU inference: RTF ~0.5, plus queueing — allow 180s per request.
	cctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, v.getBaseURL()+"/v1/audio/speech", &buf)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// normalizeWav enforces the chain output contract (24 kHz mono s16le).
func (v *VieNeuProvider) normalizeWav(ctx context.Context, wav []byte) ([]byte, error) {
	rate, ch, bits, err := wavParams(wav)
	if err != nil {
		return nil, fmt.Errorf("vieneu: bad WAV from server: %w", err)
	}
	if rate == 24000 && ch == 1 && bits == 16 {
		v.note.Store("")
		return wav, nil
	}
	if _, ferr := ffmpegBin(); ferr == nil {
		out, rerr := wavTo24kMono(ctx, wav)
		if rerr == nil {
			v.note.Store(fmt.Sprintf("resampled %dHz -> 24kHz via ffmpeg", rate))
			return out, nil
		}
	}
	v.note.Store(fmt.Sprintf("giữ nguyên %dHz (không có ffmpeg để resample về 24kHz)", rate))
	return wav, nil
}

// OutputNote reports a note about the last synthesized output ("" when the
// server already returned 24 kHz mono).
func (v *VieNeuProvider) OutputNote() string {
	if s, ok := v.note.Load().(string); ok {
		return s
	}
	return ""
}

// Voices lists the server's preset voices (GET /v1/voices).
func (v *VieNeuProvider) Voices(ctx context.Context) ([]VieNeuVoice, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, v.getBaseURL()+"/v1/voices", nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vieneu: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vieneu: /v1/voices HTTP %d", resp.StatusCode)
	}
	var vlist struct {
		Data []VieNeuVoice `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&vlist); err != nil {
		return nil, fmt.Errorf("vieneu: bad /v1/voices JSON: %w", err)
	}
	return vlist.Data, nil
}

// ---------------------------------------------------------------------------
// sidecar lifecycle

func (v *VieNeuProvider) startTimeoutOrDefault() time.Duration {
	if v.startTimeout > 0 {
		return v.startTimeout
	}
	return 10 * time.Minute // first start downloads the ~334MB model
}

// SetStartTimeout overrides how long Start/EnsureModel wait for /health.
func (v *VieNeuProvider) SetStartTimeout(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.startTimeout = d
}

// Start ensures the repo, launches the sidecar and waits until /health is ok.
// The first start downloads the ~334MB model from HuggingFace and may take
// minutes; stdout shows state "downloading" meanwhile.
func (v *VieNeuProvider) Start(ctx context.Context) error {
	v.mu.RLock()
	proc := v.proc
	v.mu.RUnlock()
	if proc != nil && proc.Running() {
		return nil
	}
	if err := v.startProcess(ctx); err != nil {
		return err
	}
	v.mu.RLock()
	proc = v.proc
	v.mu.RUnlock()
	return proc.WaitHealthy(ctx, v.startTimeoutOrDefault())
}

func (v *VieNeuProvider) startProcess(ctx context.Context) error {
	v.downloading.Store(true)
	err := v.reg.EnsureVieNeuRepo(ctx, nil)
	v.downloading.Store(false)
	if err != nil {
		return fmt.Errorf("vieneu: %w", err)
	}
	proc, err := v.buildProcess()
	if err != nil {
		return err
	}
	if err := proc.Start(ctx); err != nil {
		return fmt.Errorf("vieneu: %w", err)
	}
	v.mu.Lock()
	v.proc = proc
	v.mu.Unlock()
	return nil
}

// buildProcess assembles the supervised sidecar command:
//   - default: `uv run python -m apps.openai_speech` in the repo dir
//     (uv manages its own Python; torch-free ONNX on CPU)
//   - TTS_SIDECAR_MODE=docker: `docker compose --profile api-cpu up`
func (v *VieNeuProvider) buildProcess() (*local.Process, error) {
	repoDir := v.reg.VieNeuDir()
	healthURL := v.getBaseURL() + "/health"
	if strings.ToLower(strings.TrimSpace(os.Getenv("TTS_SIDECAR_MODE"))) == "docker" {
		if _, err := exec.LookPath("docker"); err != nil {
			return nil, fmt.Errorf("TTS_SIDECAR_MODE=docker nhưng không tìm thấy docker trong PATH")
		}
		args := []string{"compose", "--profile", "api-cpu", "up"}
		if _, err := os.Stat(filepath.Join(repoDir, "docker", "docker-compose.yml")); err == nil {
			args = []string{"compose", "-f", "docker/docker-compose.yml", "--profile", "api-cpu", "up"}
		}
		return &local.Process{
			Name: "vieneu-sidecar", Bin: "docker", Args: args,
			Dir: repoDir, HealthURL: healthURL,
		}, nil
	}
	if _, err := exec.LookPath("uv"); err != nil {
		return nil, fmt.Errorf("không tìm thấy uv trong PATH — cài từ https://astral.sh/uv (hoặc TTS_SIDECAR_MODE=docker để dùng Docker)")
	}
	return &local.Process{
		Name: "vieneu-sidecar", Bin: "uv",
		Args: []string{"run", "python", "-m", "apps.openai_speech"},
		Dir:  repoDir, HealthURL: healthURL,
	}, nil
}

// Stop terminates the sidecar.
func (v *VieNeuProvider) Stop() error {
	v.mu.RLock()
	proc := v.proc
	v.mu.RUnlock()
	if proc == nil {
		return nil
	}
	return proc.Stop()
}

// Restart is Stop followed by Start.
func (v *VieNeuProvider) Restart(ctx context.Context) error {
	if err := v.Stop(); err != nil {
		return err
	}
	return v.Start(ctx)
}

// EnsureModel drives the ~334MB model download for the dashboard "tải model"
// button: it ensures the repo, starts the sidecar (which self-downloads the
// model from HuggingFace on first boot) and waits until healthy.
// onProgress receives best-effort (downloaded, total) bytes parsed from the
// sidecar's stdout; (-1, -1) means indeterminate progress.
func (v *VieNeuProvider) EnsureModel(ctx context.Context, onProgress func(downloaded, total int64)) error {
	if v.Healthy(ctx) {
		return nil
	}
	if err := v.startProcess(ctx); err != nil {
		return err
	}
	return v.waitHealthyWithProgress(ctx, v.startTimeoutOrDefault(), onProgress)
}

func (v *VieNeuProvider) waitHealthyWithProgress(ctx context.Context, timeout time.Duration, onProgress func(int64, int64)) error {
	v.mu.RLock()
	proc := v.proc
	v.mu.RUnlock()
	if proc == nil {
		return fmt.Errorf("vieneu: sidecar chưa được khởi tạo")
	}
	deadline := time.Now().Add(timeout)
	seen := 0
	if onProgress != nil {
		onProgress(-1, -1) // indeterminate until we parse real numbers
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if v.ping(ctx) == nil {
			return nil
		}
		if onProgress != nil {
			lines := proc.RecentOutput()
			if seen > len(lines) {
				seen = len(lines) // ring rolled over; best-effort
			}
			for ; seen < len(lines); seen++ {
				if d, t, ok := parseDownloadProgress(lines[seen]); ok {
					onProgress(d, t)
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("vieneu: model chưa sẵn sàng sau %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// Status reports the sidecar state for the dashboard.
func (v *VieNeuProvider) Status() ServerStatus {
	v.mu.RLock()
	proc := v.proc
	v.mu.RUnlock()
	downloading := v.downloading.Load()
	var running bool
	var recent []string
	var uptime time.Duration
	if proc != nil {
		running = proc.Running()
		recent = proc.RecentOutput()
		uptime = proc.Uptime()
	}
	healthy := running && v.ping(context.Background()) == nil
	state, detail := classifyVieNeuState(downloading, running, healthy, recent)
	return ServerStatus{State: state, Detail: detail, Uptime: uptime}
}

// classifyVieNeuState maps supervisor signals to a dashboard state.
// Pure function, unit-tested.
func classifyVieNeuState(downloading, procRunning, healthy bool, recent []string) (state, detail string) {
	switch {
	case downloading:
		return "downloading", "đang tải VieNeu repo/model…"
	case !procRunning:
		return "stopped", "sidecar chưa chạy (gọi Start)"
	case healthy:
		return "running", "healthy"
	default:
		tail := lastNonEmptyLine(recent)
		if hasDownloadKeyword(recent) {
			return "downloading", "đang tải model (~334MB): " + truncate(tail, 120)
		}
		if tail != "" {
			return "error", "không healthy: " + truncate(tail, 160)
		}
		return "error", "không healthy (health check thất bại)"
	}
}

func lastNonEmptyLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

func hasDownloadKeyword(lines []string) bool {
	for _, l := range lines {
		low := strings.ToLower(l)
		if strings.Contains(low, "downloading") || strings.Contains(low, "download") ||
			strings.Contains(low, "loading") || strings.Contains(low, "it/s") ||
			strings.Contains(low, "%|") {
			return true
		}
	}
	return false
}

// dlProgressRe matches tqdm-style progress: "150M/334M", "1.2G/4.7G".
var dlProgressRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*([KMG])i?B?\s*/\s*(\d+(?:\.\d+)?)\s*([KMG])i?B?`)

func sizeMultiplier(u string) int64 {
	switch strings.ToUpper(u) {
	case "K":
		return 1024
	case "M":
		return 1024 * 1024
	case "G":
		return 1024 * 1024 * 1024
	}
	return 1
}

// parseDownloadProgress extracts (downloaded, total) from a progress line.
// Best-effort: returns ok=false when nothing parseable is found.
func parseDownloadProgress(line string) (downloaded, total int64, ok bool) {
	m := dlProgressRe.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, false
	}
	d, err1 := strconv.ParseFloat(m[1], 64)
	t, err2 := strconv.ParseFloat(m[3], 64)
	if err1 != nil || err2 != nil || t <= 0 {
		return 0, 0, false
	}
	downloaded = int64(d * float64(sizeMultiplier(m[2])))
	total = int64(t * float64(sizeMultiplier(m[4])))
	if downloaded > total {
		return 0, 0, false
	}
	return downloaded, total, true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
