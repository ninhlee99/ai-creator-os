//go:build parked

package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

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
