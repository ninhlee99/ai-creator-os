//go:build parked

package studio

// film.go — toàn bộ pipeline phim ngắn/điện ảnh (PIVOT 2026-10-02: đã park).
// Chuyển từ studio.go: FilmParams, CreateFilmJob, director 3 pha (truyện →
// kịch bản → breakdown), render cinematic/Veo từng shot, trailer 9:16,
// rerun/rerender. Chỉ biên dịch với -tags parked. Affiliate flow ở
// studio.go + assemble.go + trends.go không chạm vào đây.

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// KindFilm là job phim ngắn (đã park cùng pipeline phim).
const KindFilm = "film"

// FilmParams describes one short-film job.
// FilmParams describes one film job. Ninh 2026-10-02 (final): every film
// is 16:9 (cinematic standard) — Aspect is always "16:9"; vertical 9:16
// trailers are center-cropped automatically, never re-shot.
type FilmParams struct {
	Topic   string `json:"topic"`
	Seconds int    `json:"seconds"`
	// Aspect is the delivery frame: always "16:9" for films. Empty means
	// "16:9" — every pre-existing flow keeps working.
	Aspect string `json:"aspect"`
	// Genre drives the director's 3-act script (tâm lý, hành động…).
	Genre string `json:"genre,omitempty"`
	// MusicPath is an optional music bed (uploaded like affiliate music).
	MusicPath string `json:"music_path,omitempty"`
	// UpscaleFinal renders an extra lanczos-upscaled master. This is an
	// UPSCALE, not native 4K — the UI labels it honestly.
	UpscaleFinal bool `json:"upscale_final,omitempty"`
	// RenderMode: "auto" (default, khuyên dùng — Veo nếu key quay được,
	// ngược lại điện ảnh từ ảnh), "cinematic" (chỉ dựng từ ảnh, miễn phí),
	// "veo" (luôn thử Veo từng shot, tốn phí). "" = "auto".
	RenderMode string `json:"render_mode,omitempty"`
}

// orientationWord renders the aspect for image/video generation prompts
// (director + Veo keyframe/video prompts must ask for the real frame,
// otherwise a "long" YouTube cut would be generated vertical).
func orientationWord(aspect string) string {
	if aspect == "16:9" {
		return "horizontal 16:9"
	}
	return "vertical 9:16"
}

// setAssetMethod records how an asset was really rendered ("veo",
// "cinematic", "anh-tts", "trailer") so the storyboard stays honest about
// Veo usage.
func (s *Studio) setAssetMethod(id int64, method string) {
	s.db.Exec(`UPDATE studio_assets SET method=? WHERE id=?`, method, id)
}

// ---------------------------------------------------------------------------
// short-film jobs
// ---------------------------------------------------------------------------

// CreateFilmJob queues a film job and starts it. Aspect is forced to 16:9:
// every film is the cinematic standard (Ninh 2026-10-02); vertical 9:16
// trailers are center-cropped automatically, never re-shot.
func (s *Studio) CreateFilmJob(p FilmParams) (string, error) {
	if p.Seconds <= 0 {
		p.Seconds = 90
	}
	p.Aspect = "16:9"
	if strings.TrimSpace(p.Genre) == "" {
		p.Genre = defaultFilmGenre
	}
	// Film Wave 3: chuẩn hoá chế độ dựng — giá trị lạ → "auto".
	switch p.RenderMode {
	case "", RenderModeAuto, RenderModeCinematic, RenderModeVeo:
	default:
		p.RenderMode = RenderModeAuto
	}
	id, err := s.insertJob(KindFilm, p.Topic, p)
	if err != nil {
		return "", err
	}
	go s.runFilm(id, p)
	return id, nil
}

// veoRateUSD reads the Veo price per billed second from the environment.
// The default is explicitly an UNVERIFIED ESTIMATE until Ninh confirms real
// pricing — every surface showing money must carry that label.
func veoRateUSD() float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("VEO_USD_PER_SEC")), 64); err == nil && v > 0 {
		return v
	}
	return 0.05
}

// veoBudget tracks Veo spend inside one film job against the API budget
// ceiling (R2-W4 knob ops.api_budget_usd, wired via Studio.BudgetUSD).
type veoBudget struct {
	spentSecs float64
	rate      float64
	cap       float64 // 0 = no ceiling
}

// check refuses another `next` billed seconds when it would cross the cap.
func (b *veoBudget) check(next float64) error {
	if b.cap > 0 && (b.spentSecs+next)*b.rate > b.cap {
		return fmt.Errorf("vượt trần chi phí API $%.2f (ước tính chưa kiểm chứng) — dừng để không phát sinh thêm. Nâng trần ở Cài đặt · Hệ thống rồi bấm Chạy tiếp", b.cap)
	}
	return nil
}

// RerunFilmJob resumes a film job from its stored params. Scenes whose output
// asset is done and still on disk are skipped by runFilm (see
// resumeAssetPath), so this is a true resume, not a from-scratch restart.
func (s *Studio) RerunFilmJob(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	if j.Kind != KindFilm {
		return fmt.Errorf("chỉ job phim mới chạy tiếp được")
	}
	s.mu.Lock()
	_, already := s.running[id]
	s.mu.Unlock()
	if already {
		return fmt.Errorf("job đang chạy")
	}
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	s.appendLog(id, "Chạy tiếp từ cảnh chưa xong…")
	s.setStatus(id, StatusRunning, 5)
	go s.runFilm(id, p)
	return nil
}

// resumeAssetPath returns the on-disk output of an already-finished asset
// (same job, index and kind), or "" when the scene must be rendered again.
// A "done" asset whose file is missing or empty does NOT count — the scene
// is rebuilt instead of silently linking a dead file.
func (s *Studio) resumeAssetPath(jobID string, idx int, kind string) string {
	var path, status string
	_ = s.db.QueryRow(
		`SELECT path, status FROM studio_assets WHERE job_id=? AND idx=? AND kind=? ORDER BY id DESC LIMIT 1`,
		jobID, idx, kind).Scan(&path, &status)
	if status != StatusDone || path == "" {
		return ""
	}
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return ""
	}
	return path
}

// maxTTSChunkChars caps one TTS call: long narration is split by sentence so
// a film never becomes a single giant synthesis request (P0-6).
const maxTTSChunkChars = 800

// portraitIdxBase keeps portrait assets in their own index space, after the
// scene clips (which use 0..n), in the storyboard ordering.
const portraitIdxBase = 1000

// splitTextChunks splits text into sentence-boundary chunks of at most
// maxChars runes, preserving order. Sentence ends are . ! ? and newlines;
// "…" is kept inside chunks as a pause cue for the TTS voice.
