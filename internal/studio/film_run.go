//go:build parked

package studio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	s.setOutput(id, filepath.Base(final))
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
