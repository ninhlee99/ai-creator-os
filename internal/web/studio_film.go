//go:build parked

package web

// studio_film.go — handler phim ngắn/điện ảnh (PIVOT 2026-10-02: đã park).
// Chuyển từ studio.go: form tạo job phim, rerun job, rerender shot,
// film estimate/rate, storyboard bundle (truyện/kịch bản/breakdown),
// trailer seq. Chỉ biên dịch với -tags parked.

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// filmRateValue reads the Veo price from the environment. The default is
// explicitly an unverified estimate until Ninh confirms real pricing.
func filmRateValue() string {
	rate := strings.TrimSpace(os.Getenv("VEO_USD_PER_SEC"))
	if rate == "" {
		return "0.05"
	}
	return rate
}

// filmEstimate renders the one-line cost/ETA estimate shown on the film
// form before creation. Mode-aware (Film Wave 3): the cinematic stills mode
// costs ~$0 (image gen only); Veo ≈ ceil(seconds/8) shots × $rate/s with
// ~3 min polling per shot.
func filmEstimate(mode string, videoOK bool) string {
	eff := mode
	if eff == "" || eff == studio.RenderModeAuto {
		if videoOK {
			eff = studio.RenderModeVeo
		} else {
			eff = studio.RenderModeCinematic
		}
	}
	if eff == studio.RenderModeCinematic {
		return "Điện ảnh từ ảnh: ≈ $0 — chỉ tốn image gen (rẻ), không dùng Veo (ước tính chưa kiểm chứng)"
	}
	rateF, _ := strconv.ParseFloat(filmRateValue(), 64)
	if rateF <= 0 {
		rateF = 0.05
	}
	shots := (90 + 7) / 8
	cost := float64(shots*8) * rateF
	return fmt.Sprintf("≈ %d shot × 8s Veo × $%s/s ≈ $%.2f · render ~%d phút (ước tính chưa kiểm chứng)",
		shots, filmRateValue(), cost, shots*3)
}

// handleStudioFilmCreate queues a film job. Ninh 2026-10-02 (final):
// every film is 16:9 — no aspect choice on the form (the Wave-1 radio is
// gone); vertical 9:16 trailers are center-cropped automatically.
func (s *Server) handleStudioFilmCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.fail(w, fmt.Errorf("studio nil"), "studio film")
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		s.fail(w, err, "parse film form")
		return
	}
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	if topic == "" {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Nhập chủ đề phim trước đã"))
		return
	}
	seconds, _ := strconv.Atoi(r.PostFormValue("seconds"))
	if seconds < 30 {
		seconds = 30
	}
	if seconds > 3600 {
		seconds = 3600
	}
	genre := strings.TrimSpace(r.PostFormValue("genre"))
	musicPath, err := saveUpload(r, "music_file", s.Studio.UploadDir())
	if err != nil {
		s.fail(w, err, "upload music")
		return
	}
	upscale := r.PostFormValue("upscale_final") == "1"
	renderMode := strings.TrimSpace(r.PostFormValue("render_mode"))
	switch renderMode {
	case studio.RenderModeAuto, studio.RenderModeCinematic, studio.RenderModeVeo, "":
	default:
		renderMode = studio.RenderModeAuto
	}
	if _, err := s.Studio.CreateFilmJob(studio.FilmParams{
		Topic: topic, Seconds: seconds, Aspect: "16:9",
		Genre: genre, MusicPath: musicPath, UpscaleFinal: upscale,
		RenderMode: renderMode,
	}); err != nil {
		s.fail(w, err, "create film job")
		return
	}
	http.Redirect(w, r, "/studio/jobs", http.StatusSeeOther)
}

// handleStudioJobRerun resumes a failed/interrupted film job from its
// unfinished scenes (Film Wave 1 / P0-3).
func (s *Server) handleStudioJobRerun(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	if err := s.Studio.RerunFilmJob(id); err != nil {
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape(err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs?ok="+url.QueryEscape("Đang chạy tiếp job phim"))
}

// handleStudioShotRerun re-renders one storyboard shot, then re-assembles
// the film (Film Wave 2 / P1-3: "Quay lại shot này"). Runs synchronously in
// a goroutine and redirects with ?ok=/?err= like every other POST.
func (s *Server) handleStudioShotRerun(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	seq, _ := strconv.Atoi(r.PathValue("seq"))
	if err := s.Studio.QueueRerenderShot(id, seq); err != nil {
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape(err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs?ok="+url.QueryEscape(fmt.Sprintf("Đang quay lại shot %d — xong sẽ tự dựng lại phim", seq+1)))
}

// filmMethodLabel renders the honest per-scene render method for the
// storyboard (Film Wave 1 / P0-7): never overstates Veo usage.
func filmMethodLabel(m string) string {
	switch m {
	case "veo":
		return "🎬 Veo"
	case "anh-tts":
		return "🖼 Ảnh + giọng đọc"
	case "cinematic":
		return "🎞 Điện ảnh"
	case "trailer":
		return "📱 Trailer 9:16"
	default:
		return ""
	}
}

// filmScriptBundleView trả truyện/kịch bản/breakdown của job để storyboard UI
// cho xem lại từng pha sáng tạo (truyện → kịch bản → breakdown). Job chưa có
// bundle (chưa tới pha viết) trả map rỗng, UI tự ẩn.
func (s *Server) filmScriptBundleView(id string) map[string]any {
	out := map[string]any{}
	if s.Studio == nil {
		return out
	}
	b, err := s.Studio.ScriptBundle(id)
	if err != nil {
		return out
	}
	if b.Story.Text != "" {
		out["story"] = b.Story
	}
	if len(b.Screenplay.Scenes) > 0 {
		out["screenplay"] = b.Screenplay
	}
	if len(b.Breakdown.Characters) > 0 {
		out["breakdown"] = b.Breakdown
	}
	return out
}

// filmTrailerSeq returns the exact render-shot sequence numbers the
// director marked trailer_worthy (recomputed deterministically from the
// cached script, so it matches the storyboard asset indexes).
func (s *Server) filmTrailerSeq(id string) map[int]bool {
	out := map[int]bool{}
	if s.Studio == nil {
		return out
	}
	for _, seq := range s.Studio.TrailerShotSeqs(id) {
		out[seq] = true
	}
	return out
}
