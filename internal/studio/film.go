//go:build parked

package studio

// film.go — toàn bộ pipeline phim ngắn/điện ảnh (PIVOT 2026-10-02: đã park).
// Chuyển từ studio.go: FilmParams, CreateFilmJob, director 3 pha (truyện →
// kịch bản → breakdown), render cinematic/Veo từng shot, trailer 9:16,
// rerun/rerender. Chỉ biên dịch với -tags parked. Affiliate flow ở
// studio.go + assemble.go + trends.go không chạm vào đây.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
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
func splitTextChunks(text string, maxChars int) []string {
	var sents []string
	var cur strings.Builder
	flush := func() {
		t := strings.TrimSpace(cur.String())
		if t != "" {
			sents = append(sents, t)
		}
		cur.Reset()
	}
	for _, r := range text {
		cur.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			flush()
		}
	}
	flush()
	var chunks []string
	var acc strings.Builder
	push := func() {
		t := strings.TrimSpace(acc.String())
		if t != "" {
			chunks = append(chunks, t)
		}
		acc.Reset()
	}
	for _, sn := range sents {
		if acc.Len() > 0 &&
			utf8.RuneCountInString(acc.String())+1+utf8.RuneCountInString(sn) > maxChars {
			push()
		}
		if acc.Len() > 0 {
			acc.WriteByte(' ')
		}
		acc.WriteString(sn)
	}
	push()
	if len(chunks) == 0 && strings.TrimSpace(text) != "" {
		chunks = []string{strings.TrimSpace(text)}
	}
	return chunks
}

// errBudgetExceeded stops a film job cleanly when the next Veo call would
// cross the API spend ceiling (P0-5). Wrapped by renderFilmShot; runFilm
// treats it as fatal, per-shot errors only skip the shot.
var errBudgetExceeded = errors.New("vượt trần chi phí API")

// trailerIdxBase keeps trailer assets in their own index space, after the
// shot clips (0..n) and portraits (1000+i).
const trailerIdxBase = 2000

// loadOrWriteScript returns the cached script.json when a previous run wrote
// it (resume renders the SAME scenes so index-based resume stays aligned),
// otherwise asks the director to write a new one.
func (s *Studio) loadOrWriteScript(ctx context.Context, id string, p FilmParams, work string) (FilmScriptPro, error) {
	// Director 3 pha: "truyện → kịch bản → breakdown → storyboard → quay →
	// dựng". Output từng pha lưu vào script.json (FilmScriptBundle) để
	// storyboard UI xem lại khi cần; quay + dựng dùng bản ráp cuối.
	if b, rerr := loadScriptBundle(work); rerr == nil && len(b.Script.Scenes) > 0 {
		s.appendLog(id, "Dùng lại kịch bản đã viết (resume)…")
		return b.Script, nil
	}
	s.appendLog(id, "Biên kịch 3 pha: truyện → kịch bản → breakdown…")
	bundle, err := WriteFilmScriptBundle(ctx, s.llm, p.Topic, p.Genre, p.Seconds, p.Aspect,
		func(line string) { s.appendLog(id, line) })
	if err != nil {
		return FilmScriptPro{}, err
	}
	if raw, jerr := json.Marshal(bundle); jerr == nil {
		_ = os.WriteFile(filepath.Join(work, "script.json"), raw, 0o644)
	}
	return bundle.Script, nil
}

// loadCachedScript reads script.json written by a previous run (used by
// "Quay lại shot này" and re-assembly — never re-rolls the director).
// Tương thích cả bundle mới lẫn FilmScriptPro cũ (Wave 1–2).
func (s *Studio) loadCachedScript(work string) (FilmScriptPro, error) {
	b, err := loadScriptBundle(work)
	if err != nil {
		return FilmScriptPro{}, err
	}
	if len(b.Script.Scenes) == 0 {
		return FilmScriptPro{}, fmt.Errorf("kịch bản rỗng")
	}
	return b.Script, nil
}

// ScriptBundle trả bundle từng pha của job (truyện/kịch bản/breakdown) để
// storyboard UI xem lại khi cần.
func (s *Studio) ScriptBundle(jobID string) (FilmScriptBundle, error) {
	var b FilmScriptBundle
	if _, ok := s.GetJob(jobID); !ok {
		return b, fmt.Errorf("job không tồn tại")
	}
	return loadScriptBundle(filepath.Join(s.workRoot, jobID))
}

// expandScriptShots flattens every scene into render shots (≤8s each),
// assigning global sequence numbers. Deterministic: resume, re-render and
// re-assembly all recompute the identical list.
func expandScriptShots(script FilmScriptPro) []RenderShot {
	var shots []RenderShot
	for _, sc := range script.Scenes {
		shots = append(shots, expandSceneShots(sc, len(shots))...)
	}
	return shots
}

func sceneMap(script FilmScriptPro) map[int]FilmScenePro {
	m := make(map[int]FilmScenePro, len(script.Scenes))
	for _, sc := range script.Scenes {
		m[sc.Index] = sc
	}
	return m
}

func characterLocks(script FilmScriptPro) string {
	var sb strings.Builder
	for _, c := range script.Characters {
		sb.WriteString("\n")
		sb.WriteString(c.LockBlock())
	}
	return sb.String()
}

func firstPortraitPath(refs []ImageRef) string {
	if len(refs) > 0 {
		return refs[0].Path
	}
	return ""
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(dst); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(dst, b, 0o644)
}

func (s *Studio) runFilm(id string, p FilmParams) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	// Render-slot gate: same cap as affiliate jobs (see runAffiliate).
	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		return
	}

	s.setStatus(id, StatusRunning, 5)
	fail := func(err error) {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
	}
	if s.llm == nil {
		fail(fmt.Errorf("thiếu LLM"))
		return
	}
	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}

	// 1. Kịch bản (3 hồi cho phim dài — xem director.go).
	script, err := s.loadOrWriteScript(ctx, id, p, work)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, fmt.Sprintf("Kịch bản: %q — %d nhân vật, %d cảnh, thể loại %s",
		script.Title, len(script.Characters), len(script.Scenes), script.Genre))

	if s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu GEMINI_API_KEYS hoặc key lỗi)"))
		return
	}

	// 2. Character portraits: khóa identity — mọi shot sau dùng làm reference.
	// Portraits stay vertical 9:16 on purpose: they are identity references
	// for generation, not delivery frames.
	for i := range script.Characters {
		c := &script.Characters[i]
		if done := s.resumeAssetPath(id, portraitIdxBase+i, "portrait"); done != "" {
			s.appendLog(id, fmt.Sprintf("Chân dung %q đã có — bỏ qua", c.Name))
			c.Portrait = done
			continue
		}
		pp := fmt.Sprintf("Cinematic character portrait, %s. Wardrobe: %s. "+
			"Photorealistic, vertical 9:16, neutral expression, plain background, no text, no watermark.",
			c.Appearance, c.Wardrobe)
		out := filepath.Join(work, fmt.Sprintf("char-%02d.png", i))
		pid := s.addAsset(id, portraitIdxBase+i, "portrait", pp)
		s.setAsset(pid, StatusRunning, "")
		s.appendLog(id, fmt.Sprintf("Vẽ chân dung nhân vật %q (khóa identity)…", c.Name))
		if err := s.mg.GenerateImage(ctx, pp, nil, out); err != nil {
			// P2-4: a broken portrait must show up failed on the storyboard,
			// never vanish silently — scenes still render from the text
			// description via charLocks.
			s.appendLog(id, fmt.Sprintf("⚠ Chân dung %q lỗi: %v — cảnh vẫn quay bằng mô tả chữ", c.Name, err))
			s.setAsset(pid, StatusFailed, "")
			continue
		}
		s.setAsset(pid, StatusDone, out)
		c.Portrait = out
	}
	charRefs := script.CharacterRefs()
	charLocks := characterLocks(script)

	// 3. Mở cảnh thành shot render ≤8s (đơn vị thật của Veo).
	scenes := sceneMap(script)
	shots := expandScriptShots(script)
	s.appendLog(id, fmt.Sprintf("Chia %d cảnh → %d shot (mỗi shot ≤%ds)",
		len(script.Scenes), len(shots), veoMaxSeconds))

	// 4. Chế độ dựng (Film Wave 3): "auto" → Veo nếu key quay được
	// (probe/lịch sử), ngược lại đi thẳng điện ảnh từ ảnh — không thử Veo,
	// đỡ tốn phút poll API chết.
	renderMode := s.resolveRenderMode(p)
	s.appendLog(id, "Chế độ dựng: "+renderModeLabel(renderMode))

	// 5. Trần chi phí Veo (P0-5): nối knob ops.api_budget_usd. Chế độ điện
	// ảnh không gọi Veo nên chỉ ghi nhận, không tính toán chi phí.
	budget := &veoBudget{rate: veoRateUSD()}
	if s.BudgetUSD != nil {
		budget.cap = s.BudgetUSD()
	}
	if renderMode == RenderModeCinematic {
		s.appendLog(id, "Không tốn Veo — chỉ tốn image gen cho keyframe (rẻ)")
	} else if budget.cap > 0 {
		s.appendLog(id, fmt.Sprintf("Trần chi phí API: $%.2f — vượt trần sẽ dừng (giá Veo ước tính chưa kiểm chứng)", budget.cap))
	} else {
		s.appendLog(id, fmt.Sprintf("Chưa đặt trần chi phí API — %.0f shot × ≤%ds × $%.3f/s (ước tính chưa kiểm chứng)",
			float64(len(shots)), veoMaxSeconds, budget.rate))
	}

	// 6. Render từng shot theo chế độ.
	var rendered []renderedShot
	var subTexts []string
	var subDurs []float64
	prevMp4 := ""
	for _, sh := range shots {
		if done := s.resumeAssetPath(id, sh.Seq, "shot"); done != "" {
			d := ProbeDuration(ctx, done)
			if d <= 0 {
				d = float64(sh.Seconds)
			}
			s.appendLog(id, fmt.Sprintf("Shot %d/%d đã dựng — bỏ qua", sh.Seq+1, len(shots)))
			rendered = append(rendered, renderedShot{Shot: sh, MP4: done, Dur: d})
			if t := shotSpokenText(sh); strings.TrimSpace(t) != "" {
				subTexts = append(subTexts, t)
				subDurs = append(subDurs, d)
			}
			prevMp4 = done
			s.setStatus(id, StatusRunning, 10+int(70*float64(len(rendered))/float64(len(shots))))
			continue
		}
		rs, rerr := s.renderFilmShot(ctx, id, p, work, scenes[sh.SceneIdx], charLocks, charRefs, sh, prevMp4, budget, renderMode)
		if rerr != nil {
			if errors.Is(rerr, errBudgetExceeded) {
				fail(rerr)
				return
			}
			s.appendLog(id, fmt.Sprintf("Shot %d/%d lỗi (%v) — bỏ qua", sh.Seq+1, len(shots), rerr))
			continue
		}
		rendered = append(rendered, *rs)
		prevMp4 = rs.MP4
		if t := shotSpokenText(sh); strings.TrimSpace(t) != "" {
			subTexts = append(subTexts, t)
			subDurs = append(subDurs, rs.Dur)
		}
		s.setStatus(id, StatusRunning, 10+int(70*float64(len(rendered))/float64(len(shots))))
	}
	if len(rendered) == 0 {
		fail(fmt.Errorf("không dựng được shot nào"))
		return
	}

	// 7. Dựng phim cuối: nối → phụ đề → nhạc → upscale.
	final, err := s.assembleFilmFinal(ctx, id, p, work, rendered, subTexts, subDurs, renderMode)
	if err != nil {
		fail(err)
		return
	}
	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))

	// 8. Trailer dọc 9:16 từ shot trailer_worthy (Ninh 2026-10-02).
	s.buildTrailers(ctx, id, p, work, rendered)

	s.fireOnDone(id)
}

// renderFilmShot renders one ≤8s shot and records it as a "shot" asset.
// Film Wave 3 — hai chế độ dựng:
//   - "veo": firstFrame chaining (shot 0 dùng keyframe của chính nó; các
//     shot sau đưa Veo frame cuối của shot trước) → Veo 8s; Veo lỗi thì rơi
//     về điện ảnh từ ảnh, không fail job.
//   - "cinematic" (CHẾ ĐỘ CHÍNH — key Gemini của Ninh không quay được
//     video): keyframe (bố cục cho chuyển động) → chuyển động điện ảnh theo
//     camera_move → voice/silence audio.
func (s *Studio) renderFilmShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, prevMp4 string, budget *veoBudget, renderMode string) (*renderedShot, error) {

	orient := orientationWord(p.Aspect)
	aid := s.addAsset(id, sh.Seq, "shot", sh.ImagePrompt)
	mp4 := filepath.Join(work, fmt.Sprintf("shot%04d.mp4", sh.Seq))
	s.setAsset(aid, StatusRunning, "")

	keyPath := filepath.Join(work, fmt.Sprintf("shot%04d.png", sh.Seq))
	// Continuity bible: khối bất di bất dịch của cảnh — kế thừa NGUYÊN VĂN
	// vào mọi prompt, không diễn đạt lại (continuity lock từng khung hình).
	contBlock := ""
	if strings.TrimSpace(sc.Continuity) != "" {
		contBlock = "\nCONTINUITY BIBLE — copy verbatim into the frame, do not paraphrase or alter:\n" +
			strings.TrimSpace(sc.Continuity)
	}
	keyPrompt := sh.ImagePrompt + charLocks + "\n" + sc.SceneLockBlock() + contBlock +
		" Cinematic photorealistic, " + orient + ", no text, no watermark."

	if renderMode == RenderModeVeo {
		if rs, ok, ferr := s.tryVeoShot(ctx, id, p, work, sc, charLocks, charRefs,
			sh, prevMp4, budget, aid, mp4, keyPath, keyPrompt, orient, contBlock); ferr != nil {
			return nil, ferr
		} else if ok {
			return rs, nil
		}
		// Veo lỗi → rơi về điện ảnh từ ảnh (log đã ghi trong tryVeoShot).
	}
	return s.renderCinematicShot(ctx, id, p, work, sc, charLocks, charRefs,
		sh, aid, mp4, keyPath, keyPrompt)
}

// tryVeoShot attempts one ≤8s Veo render with firstFrame chaining.
// (rs, true, nil) = quay được; (nil, false, nil) = Veo lỗi → caller rơi về
// điện ảnh từ ảnh; (nil, false, err) = lỗi nghiêm trọng (vượt trần chi phí).
func (s *Studio) tryVeoShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, prevMp4 string, budget *veoBudget,
	aid int64, mp4, keyPath, keyPrompt, orient, contBlock string) (*renderedShot, bool, error) {

	// FirstFrame: shot đầu dùng keyframe của chính nó; các shot sau dùng
	// frame cuối của shot trước (không cắt được frame → vẽ keyframe mới).
	firstFrame := ""
	if sh.Seq == 0 || prevMp4 == "" {
		if kerr := s.mg.GenerateImage(ctx, keyPrompt, charRefs, keyPath); kerr != nil {
			s.setAsset(aid, StatusFailed, "")
			return nil, false, fmt.Errorf("keyframe: %w", kerr)
		}
		firstFrame = keyPath
	} else if err := extractLastFrame(ctx, prevMp4, keyPath); err != nil {
		if kerr := s.mg.GenerateImage(ctx, keyPrompt, charRefs, keyPath); kerr != nil {
			s.setAsset(aid, StatusFailed, "")
			return nil, false, fmt.Errorf("keyframe: %w", kerr)
		}
		firstFrame = keyPath
	} else {
		firstFrame = keyPath
	}

	// QC chống drift (mặc định no-op trung thực — xem qc.go).
	if s.QC != nil {
		_ = s.QC.CheckShot(ctx, firstPortraitPath(charRefs), keyPath, "")
	}

	// Trần chi phí: kiểm tra TRƯỚC mỗi lần gọi Veo (P0-5).
	billed := float64(min(sh.Seconds, veoMaxSeconds))
	if err := budget.check(billed); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, false, fmt.Errorf("%w: %v", errBudgetExceeded, err)
	}
	vprompt := fmt.Sprintf("%s. Camera: %s, %s, %s. %s cinematic film, natural motion, no text.",
		sh.ImagePrompt, sh.ShotSize, sh.CameraMove, sh.LensLight, orient) +
		charLocks + "\n" + sc.SceneLockBlock() + contBlock + performanceBlock(sh)
	s.appendLog(id, fmt.Sprintf("Quay shot %d (%ds)…", sh.Seq+1, sh.Seconds))
	if err := s.mg.GenerateVideo(ctx, vprompt, firstFrame, sh.Seconds, p.Aspect, mp4); err != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: Veo lỗi (%v) — dùng điện ảnh từ ảnh", sh.Seq+1, err))
		return nil, false, nil
	}
	budget.spentSecs += billed
	s.appendLog(id, fmt.Sprintf("Shot %d: Veo ✓ (~$%.2f ước tính)", sh.Seq+1, billed*budget.rate))
	// Veo thành công → key quay được video (capability học từ lần chạy thật).
	_ = s.SetCapability(CapVideoGen, CapOK, fmt.Sprintf("shot %d quay bằng Veo thành công", sh.Seq+1))
	dur := ProbeDuration(ctx, mp4)
	if dur <= 0 {
		dur = float64(sh.Seconds)
	}
	s.setAssetMethod(aid, "veo")
	s.setAsset(aid, StatusDone, mp4)
	return &renderedShot{Shot: sh, MP4: mp4, Dur: dur}, true, nil
}

// renderCinematicShot renders one shot in the primary "cinematic stills"
// mode (Film Wave 3): keyframe (bố cục chừa biên cho chuyển động —
// prompts/film_keyframe.txt) → chuyển động điện ảnh theo đúng camera_move
// của đạo diễn → voice/silence audio. Không tốn Veo.
func (s *Studio) renderCinematicShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, aid int64, mp4, keyPath, keyPrompt string) (*renderedShot, error) {

	cinePrompt := keyPrompt + keyframeCompBlock()
	move := strings.ToLower(strings.TrimSpace(sh.CameraMove))
	if move == "" {
		move = "drift"
	}
	s.appendLog(id, fmt.Sprintf("Shot %d: dựng điện ảnh từ ảnh (%s)…", sh.Seq+1, move))
	if kerr := s.mg.GenerateImage(ctx, cinePrompt, charRefs, keyPath); kerr != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("keyframe: %w", kerr)
	}

	// QC chống drift (mặc định no-op trung thực — xem qc.go).
	if s.QC != nil {
		_ = s.QC.CheckShot(ctx, firstPortraitPath(charRefs), keyPath, "")
	}

	// Giọng đọc của shot (TTS từng đoạn ≤800 ký tự — P0-6); lỗi → bản câm.
	wavPath := s.synthShotVoice(ctx, id, sh, work)
	dur := float64(sh.Seconds)
	if wavPath != "" {
		if ws, err := engines.WavSeconds(wavPath); err == nil && ws > dur {
			dur = ws
		}
	}
	tmpV := filepath.Join(work, fmt.Sprintf("shot%04d.cine.mp4", sh.Seq))
	if err := RenderCinematicShot(ctx, CinematicShot{
		ImagePath: keyPath, Seconds: dur,
		CameraMove: sh.CameraMove, Mood: sc.Atmosphere,
	}, tmpV); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("dựng điện ảnh: %w", err)
	}
	defer os.Remove(tmpV)
	if err := AddShotAudio(ctx, tmpV, wavPath, dur, mp4); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("gắn giọng đọc: %w", err)
	}
	d := ProbeDuration(ctx, mp4)
	if d <= 0 {
		d = dur
	}
	s.setAssetMethod(aid, "cinematic")
	s.setAsset(aid, StatusDone, mp4)
	return &renderedShot{Shot: sh, MP4: mp4, Dur: d}, nil
}

// synthShotVoice synthesizes a shot's dialogue + narration in ≤800-char
// chunks (P0-6) and joins them into one WAV. Returns "" when there is
// nothing to say or synthesis fails — fail-soft: silent shot, logged.
func (s *Studio) synthShotVoice(ctx context.Context, id string, sh RenderShot, work string) string {
	speech := shotSpokenText(sh)
	if s.narrator == nil || strings.TrimSpace(speech) == "" {
		return ""
	}
	var wavs [][]byte
	for _, ch := range splitTextChunks(speech, maxTTSChunkChars) {
		w, err := s.narrator(ctx, ch)
		if err != nil || len(w) == 0 {
			s.appendLog(id, fmt.Sprintf("Shot %d: TTS lỗi (%v) — dựng bản câm", sh.Seq+1, err))
			return ""
		}
		wavs = append(wavs, w)
	}
	joined, jerr := tts.ConcatWavs(wavs)
	if jerr != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: nối WAV lỗi (%v) — dựng bản câm", sh.Seq+1, jerr))
		return ""
	}
	wavPath := filepath.Join(work, fmt.Sprintf("shot%04d.wav", sh.Seq))
	if werr := os.WriteFile(wavPath, joined, 0o644); werr != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: không ghi được WAV: %v", sh.Seq+1, werr))
		return ""
	}
	return wavPath
}

// assembleFilmFinal nối các shot → mux phụ đề → mix nhạc bed → upscale
// (nếu chọn) → file cuối trong outDir. Dùng chung cho runFilm và dựng lại
// sau "Quay lại shot này".
//
// Film Wave 3: chế độ "cinematic" dựng bằng xfade (chuyển cảnh mượt) +
// letterbox 2.35:1 + phụ đề theo đúng timeline xfade; chế độ "veo" giữ
// đường nối cứng cũ.
func (s *Studio) assembleFilmFinal(ctx context.Context, id string, p FilmParams, work string,
	rendered []renderedShot, subTexts []string, subDurs []float64, renderMode string) (string, error) {

	var clips []string
	for _, r := range rendered {
		clips = append(clips, r.MP4)
	}
	var cur string
	if renderMode == RenderModeCinematic {
		s.appendLog(id, fmt.Sprintf("Dựng điện ảnh: chuyển cảnh mượt %d shot…", len(clips)))
		var cc []CinematicClip
		for _, r := range rendered {
			cc = append(cc, CinematicClip{Path: r.MP4, CameraMove: r.Shot.CameraMove})
		}
		xfade := filepath.Join(work, "film_xfade.mp4")
		if err := AssembleCinematic(ctx, cc, xfade); err != nil {
			return "", err
		}
		cur = xfade
		// Letterbox 2.35:1 — viền điện ảnh, chỉ ở bản phim cuối (trailer
		// cắt từ shot gốc nên không dính viền).
		lb := filepath.Join(work, "film_letterbox.mp4")
		if err := ApplyLetterbox(ctx, cur, lb); err != nil {
			s.appendLog(id, "⚠ Letterbox lỗi ("+err.Error()+") — giữ bản không viền")
		} else {
			cur = lb
		}
		// Phụ đề theo đúng timeline xfade (shot sau bắt đầu sớm hơn 0.7s).
		if cues := cinematicCues(subTexts, subDurs); len(cues) > 0 {
			s.appendLog(id, fmt.Sprintf("Mux phụ đề (%d câu)…", len(cues)))
			subbed := filepath.Join(work, "film_subs.mp4")
			if err := MuxSubtitles(ctx, cur, formatSRT(cues), subbed); err != nil {
				return "", fmt.Errorf("mux phụ đề: %w", err)
			}
			cur = subbed
		}
	} else {
		s.appendLog(id, fmt.Sprintf("Nối %d shot…", len(clips)))
		concat := filepath.Join(work, "film_concat.mp4")
		if err := ConcatClips(ctx, clips, p.Aspect, concat); err != nil {
			return "", err
		}
		cur = concat
		// Phụ đề (P1-1): SRT từ thoại + lời dẫn, khớp thời lượng từng shot.
		var st []string
		var sd []float64
		for i, t := range subTexts {
			if strings.TrimSpace(t) != "" && i < len(subDurs) {
				st = append(st, t)
				sd = append(sd, subDurs[i])
			}
		}
		if len(st) > 0 {
			s.appendLog(id, fmt.Sprintf("Mux phụ đề (%d câu)…", len(st)))
			subbed := filepath.Join(work, "film_subs.mp4")
			if err := MuxSubtitles(ctx, cur, engines.BuildSRT(st, sd), subbed); err != nil {
				return "", fmt.Errorf("mux phụ đề: %w", err)
			}
			cur = subbed
		}
	}
	// Nhạc bed (P1-2): duck −8dB khi có voice + loudnorm −14 LUFS.
	if strings.TrimSpace(p.MusicPath) != "" {
		s.appendLog(id, "Mix nhạc nền…")
		mixed := filepath.Join(work, "film_music.mp4")
		if err := MixMusicBed(ctx, cur, p.MusicPath, mixed); err != nil {
			s.appendLog(id, "⚠ Mix nhạc lỗi ("+err.Error()+") — giữ bản không nhạc")
		} else {
			cur = mixed
		}
	}
	// Upscale cuối (P1-5): lanczos — KHÔNG phải 4K native, UI ghi rõ.
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	if p.UpscaleFinal {
		w, h := AspectDims(p.Aspect)
		s.appendLog(id, "Upscale lanczos (không phải 4K native) — bước này lâu…")
		if err := UpscaleVideo(ctx, cur, final, w*2, h*2); err != nil {
			s.appendLog(id, "⚠ Upscale lỗi ("+err.Error()+") — giữ bản gốc")
			if err := copyFile(cur, final); err != nil {
				return "", err
			}
		}
	} else if err := copyFile(cur, final); err != nil {
		return "", err
	}
	return final, nil
}

// buildTrailers cắt tối đa 2 trailer dọc 9:16 từ các shot trailer_worthy
// (Ninh 2026-10-02): center-crop từ phim 16:9, không quay lại. Không có shot
// nào được đánh dấu → bỏ qua, không lỗi.
func (s *Studio) buildTrailers(ctx context.Context, id string, p FilmParams, work string, rendered []renderedShot) {
	var marked []renderedShot
	for _, r := range rendered {
		if r.Shot.TrailerWorthy {
			marked = append(marked, r)
		}
	}
	groups := selectTrailerGroups(marked)
	if len(groups) == 0 {
		s.appendLog(id, "Không có shot trailer_worthy — bỏ qua cắt trailer")
		return
	}
	for gi, g := range groups {
		var vertical []string
		var total float64
		ok := true
		for _, m := range g {
			dst := filepath.Join(work, fmt.Sprintf("trailer%d_%04d.mp4", gi, m.Shot.Seq))
			if err := CropCenterVertical(ctx, m.MP4, dst); err != nil {
				s.appendLog(id, fmt.Sprintf("Trailer %d: crop shot %d lỗi (%v) — bỏ trailer này", gi+1, m.Shot.Seq+1, err))
				ok = false
				break
			}
			vertical = append(vertical, dst)
			total += m.Dur
		}
		if !ok || len(vertical) == 0 {
			continue
		}
		// Clips đã 1080x1920 sau crop → ConcatClips "9:16" nối thẳng,
		// không scale thừa.
		raw := filepath.Join(work, fmt.Sprintf("trailer%d_raw.mp4", gi))
		if err := ConcatClips(ctx, vertical, "9:16", raw); err != nil {
			s.appendLog(id, fmt.Sprintf("Trailer %d: nối lỗi (%v)", gi+1, err))
			continue
		}
		cur := raw
		if strings.TrimSpace(p.MusicPath) != "" {
			mixed := filepath.Join(work, fmt.Sprintf("trailer%d.mp4", gi))
			if err := MixMusicBed(ctx, cur, p.MusicPath, mixed); err != nil {
				s.appendLog(id, fmt.Sprintf("Trailer %d: mix nhạc lỗi — giữ bản không nhạc", gi+1))
			} else {
				cur = mixed
			}
		}
		final := filepath.Join(s.outDir, fmt.Sprintf("studio-%s-trailer%d.mp4", id, gi+1))
		if err := copyFile(cur, final); err != nil {
			s.appendLog(id, fmt.Sprintf("Trailer %d: lưu lỗi (%v)", gi+1, err))
			continue
		}
		aid := s.addAsset(id, trailerIdxBase+gi, "trailer",
			fmt.Sprintf("Trailer %d — %d shot (%.0fs), crop dọc 9:16 từ phim", gi+1, len(g), total))
		s.setAssetMethod(aid, "trailer")
		s.setAsset(aid, StatusDone, final)
		s.appendLog(id, fmt.Sprintf("Trailer %d xong (%.0fs, 9:16)", gi+1, total))
	}
}

// TrailerShotSeqs returns the render-shot sequence numbers the director
// marked trailer_worthy, recomputed deterministically from the cached
// script.json — the storyboard UI uses it for the trailer badge.
func (s *Studio) TrailerShotSeqs(id string) []int {
	script, err := s.loadCachedScript(filepath.Join(s.workRoot, id))
	if err != nil {
		return nil
	}
	var out []int
	for _, sh := range expandScriptShots(script) {
		if sh.TrailerWorthy {
			out = append(out, sh.Seq)
		}
	}
	return out
}

// RerenderShot quay lại đúng một shot rồi dựng lại phim (storyboard UI /
// P1-3). Chạy đồng bộ — handler web dùng QueueRerenderShot để chạy nền.
func (s *Studio) RerenderShot(id string, seq int) error {
	if err := s.validateRerender(id, seq); err != nil {
		return err
	}
	j, _ := s.GetJob(id)
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return fmt.Errorf("không đọc được kịch bản: %w", err)
	}
	shots := expandScriptShots(script)
	sh := shots[seq]
	scenes := sceneMap(script)
	var charRefs []ImageRef
	for i := range script.Characters {
		if pp := s.resumeAssetPath(id, portraitIdxBase+i, "portrait"); pp != "" {
			charRefs = append(charRefs, ImageRef{Path: pp})
		}
	}
	prevMp4 := ""
	if seq > 0 {
		prevMp4 = s.resumeAssetPath(id, seq-1, "shot")
	}
	budget := &veoBudget{rate: veoRateUSD()}
	if s.BudgetUSD != nil {
		budget.cap = s.BudgetUSD()
	}
	ctx := context.Background()
	s.appendLog(id, fmt.Sprintf("Quay lại shot %d…", seq+1))
	s.setStatus(id, StatusRunning, 50)
	renderMode := s.resolveRenderMode(p)
	if _, err := s.renderFilmShot(ctx, id, p, work, scenes[sh.SceneIdx], characterLocks(script), charRefs, sh, prevMp4, budget, renderMode); err != nil {
		s.setStatus(id, StatusFailed, 100)
		return err
	}
	return s.ReassembleFilm(id)
}

// validateRerender checks everything cheap before a shot re-render starts
// (job exists, is a film, idle, seq valid, script cached).
func (s *Studio) validateRerender(id string, seq int) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	if j.Kind != KindFilm {
		return fmt.Errorf("chỉ job phim mới quay lại shot được")
	}
	s.mu.Lock()
	_, already := s.running[id]
	s.mu.Unlock()
	if already {
		return fmt.Errorf("job đang chạy — đợi xong rồi quay lại")
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return fmt.Errorf("không đọc được kịch bản: %w", err)
	}
	if seq < 0 || seq >= len(expandScriptShots(script)) {
		return fmt.Errorf("shot %d không tồn tại", seq+1)
	}
	return nil
}

// QueueRerenderShot validates synchronously, then re-renders the shot and
// re-assembles the film in the background (a Veo shot takes minutes — the
// HTTP handler must not block).
func (s *Studio) QueueRerenderShot(id string, seq int) error {
	if err := s.validateRerender(id, seq); err != nil {
		return err
	}
	go func() {
		if err := s.RerenderShot(id, seq); err != nil {
			s.appendLog(id, "Quay lại shot lỗi: "+err.Error())
			s.setStatus(id, StatusFailed, 100)
		}
	}()
	return nil
}

// ReassembleFilm dựng lại phim từ các shot đã xong (sau "Quay lại shot này").
func (s *Studio) ReassembleFilm(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return err
	}
	shots := expandScriptShots(script)
	ctx := context.Background()
	var rendered []renderedShot
	var subTexts []string
	var subDurs []float64
	for _, sh := range shots {
		mp4 := s.resumeAssetPath(id, sh.Seq, "shot")
		if mp4 == "" {
			return fmt.Errorf("shot %d chưa dựng xong — không dựng lại được", sh.Seq+1)
		}
		d := ProbeDuration(ctx, mp4)
		if d <= 0 {
			d = float64(sh.Seconds)
		}
		rendered = append(rendered, renderedShot{Shot: sh, MP4: mp4, Dur: d})
		if t := shotSpokenText(sh); strings.TrimSpace(t) != "" {
			subTexts = append(subTexts, t)
			subDurs = append(subDurs, d)
		}
	}
	s.appendLog(id, "Dựng lại phim từ các shot…")
	final, err := s.assembleFilmFinal(ctx, id, p, work, rendered, subTexts, subDurs, s.resolveRenderMode(p))
	if err != nil {
		s.setStatus(id, StatusFailed, 100)
		return err
	}
	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Dựng lại xong: "+filepath.Base(final))
	s.buildTrailers(ctx, id, p, work, rendered)
	s.fireOnDone(id)
	return nil
}

// ---------------------------------------------------------------------------
// capability probe Veo (park cùng pipeline phim)
// ---------------------------------------------------------------------------

// VideoCapOK báo key có quay được video không (theo lần kiểm tra gần nhất).
// videoCapStatus đọc trạng thái video_gen trực tiếp từ DB (GetCapabilities
// ở bản thường đã bỏ hàng video — parked code không dùng nó nữa).
func (s *Studio) videoCapStatus() string {
	var st string
	if err := s.db.QueryRow(`SELECT status FROM capabilities WHERE key=?`,
		CapVideoGen).Scan(&st); err != nil {
		return CapUnknown
	}
	return st
}

func (s *Studio) VideoCapOK() bool {
	return s.videoCapStatus() == CapOK
}

// RefreshVideoStatusFromHistory suy trạng thái quay video từ N lần quay
// thật gần nhất (method từng shot — Wave 1 đã log): có shot nào "veo"
// thành công → ok; toàn "anh-tts" → fail (key không tạo được video);
// chưa quay lần nào → unknown. Chỉ ghi khi trạng thái hiện tại là
// unknown — probe tay luôn thắng.
func (s *Studio) RefreshVideoStatusFromHistory() {
	if s.videoCapStatus() != CapUnknown {
		return
	}
	rows, err := s.db.Query(
		`SELECT method FROM studio_assets
		 WHERE kind='shot' AND method != '' ORDER BY id DESC LIMIT 30`)
	if err != nil {
		return
	}
	defer rows.Close()
	n := 0
	veoOK := false
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			continue
		}
		n++
		if m == "veo" {
			veoOK = true
		}
	}
	switch {
	case veoOK:
		_ = s.SetCapability(CapVideoGen, CapOK, "shot gần nhất quay bằng Veo thành công")
	case n > 0:
		_ = s.SetCapability(CapVideoGen, CapFail,
			fmt.Sprintf("%d lần quay gần nhất đều rớt về ảnh + giọng đọc — key có thể không tạo được video", n))
	default:
		_ = s.SetCapability(CapVideoGen, CapUnknown, "chưa có lần quay nào")
	}
}

// ProbeVideoGen quay thử đúng 1 clip 8s bằng Veo — TỐN TIỀN THẬT (~8s
// billed). Chỉ gọi từ nút bấm tay "Kiểm tra quay video" (đã cảnh báo chi
// phí trên UI), không bao giờ tự chạy ngầm.
func (s *Studio) ProbeVideoGen(ctx context.Context) error {
	if s.mg == nil || !s.mg.Healthy(ctx) {
		_ = s.SetCapability(CapVideoGen, CapFail, "thiếu GEMINI_API_KEYS hoặc key lỗi")
		return fmt.Errorf("media-gen chưa sẵn sàng")
	}
	img := filepath.Join(os.TempDir(), "aicos-probe-key.png")
	defer os.Remove(img)
	if err := s.mg.GenerateImage(ctx,
		"a red sports car on an empty road at sunset, cinematic photo, no text",
		nil, img); err != nil {
		_ = s.SetCapability(CapVideoGen, CapFail, "không vẽ được keyframe kiểm tra: "+truncErr(err, 120))
		return fmt.Errorf("keyframe kiểm tra: %w", err)
	}
	out := filepath.Join(os.TempDir(), "aicos-probe-veo.mp4")
	defer os.Remove(out)
	err := s.mg.GenerateVideo(ctx,
		"the red sports car drives slowly forward on the road, cinematic, natural motion, no text",
		img, veoMaxSeconds, "16:9", out)
	if err != nil {
		_ = s.SetCapability(CapVideoGen, CapFail, "Veo lỗi: "+truncErr(err, 160))
		return err
	}
	_ = s.SetCapability(CapVideoGen, CapOK, "quay thử 8s thành công")
	return nil
}

// ---------------------------------------------------------------------------
// upscale video (park cùng pipeline phim — chỉ film dùng)
// ---------------------------------------------------------------------------

// UpscaleVideo masters a finished video to w×h with lanczos. This is an
// UPSCALE, not native 4K — the UI must label it honestly. Slow: only for
// finals the user explicitly asks to upscale.
func UpscaleVideo(ctx context.Context, src, dst string, w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("upscale: kích thước %dx%d không hợp lệ", w, h)
	}
	args := []string{"-i", src,
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h),
	}
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "copy", dst)
	return ffmpegRun(ctx, args...)
}
