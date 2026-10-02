package studio

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RenderShot là đơn vị render thật của pipeline phim: một shot dài tối đa
// veoMaxSeconds (8s) — đúng một lần gọi Veo. Cảnh (FilmScenePro) chỉ còn là
// nhóm shot để kịch bản dễ đọc; expandSceneShots chia mỗi cảnh thành các
// RenderShot, giữ tổng thời lượng cảnh không đổi.
type RenderShot struct {
	Seq           int            // thứ tự toàn phim (0-based)
	SceneIdx      int            // cảnh chứa shot
	Act           int            // hồi 1|2|3 (0 = phim ngắn)
	SubIdx        int            // lần chia nhỏ trong cùng một beat (0 = đầu)
	Seconds       int            // ≤ veoMaxSeconds
	ShotSize      string         // WS, CU, ECU …
	CameraMove    string         // dolly-in, tracking …
	LensLight     string         // 35mm f/1.8, golden hour …
	Action        string         // diễn xuất cụ thể — tiếng Việt
	ImagePrompt   string         // prompt keyframe — tiếng Anh
	VideoPrompt   string         // prompt video bổ sung — tiếng Anh
	Dialogue      []DialogueLine // thoại của shot
	Narration     string         // lời dẫn của shot
	TrailerWorthy bool           // shot đắt giá → cắt trailer
}

// expandSceneShots chia một cảnh thành các RenderShot ≤8s. Thời lượng cảnh
// được phân bổ theo trọng số seconds của từng beat (shot của đạo diễn);
// beat nào >8s thì chia nhỏ tiếp. Thoại và lời dẫn của CẢNH được chia đều
// vòng tròn cho các unit — để phụ đề khớp từng shot.
func expandSceneShots(sc FilmScenePro, seqBase int) []RenderShot {
	beats := sc.Shots
	if len(beats) == 0 {
		beats = []ProShot{{
			ShotSize: "WS", CameraMove: "static",
			Action: sc.ImagePrompt, ImagePrompt: sc.ImagePrompt,
			Seconds: sc.Seconds,
		}}
	}
	sceneSecs := sc.Seconds
	if sceneSecs <= 0 {
		sceneSecs = 16
	}
	weights := make([]int, len(beats))
	wTotal := 0
	for i, b := range beats {
		w := b.Seconds
		if w <= 0 {
			w = 8
		}
		weights[i] = w
		wTotal += w
	}
	var out []RenderShot
	assigned := 0
	for bi, b := range beats {
		beatSecs := sceneSecs * weights[bi] / wTotal
		if bi == len(beats)-1 {
			beatSecs = sceneSecs - assigned // hút phần dư làm tròn
		}
		if beatSecs < 1 {
			beatSecs = 1
		}
		assigned += beatSecs
		units := (beatSecs + veoMaxSeconds - 1) / veoMaxSeconds
		if units < 1 {
			units = 1
		}
		base := beatSecs / units
		rem := beatSecs % units
		for u := 0; u < units; u++ {
			secs := base
			if u < rem {
				secs++
			}
			rs := RenderShot{
				Seq: len(out) + seqBase, SceneIdx: sc.Index, Act: sc.Act,
				SubIdx: u, Seconds: secs,
				ShotSize: b.ShotSize, CameraMove: b.CameraMove,
				LensLight: b.LensLight, Action: b.Action,
				ImagePrompt: b.ImagePrompt, VideoPrompt: b.VideoPrompt,
				TrailerWorthy: b.TrailerWorthy,
			}
			if rs.ImagePrompt == "" {
				rs.ImagePrompt = sc.ImagePrompt
			}
			if u > 0 {
				rs.ImagePrompt += " (continued shot, same framing, same action)"
			}
			out = append(out, rs)
		}
	}
	// Thoại + lời dẫn của cảnh chia vòng tròn cho các unit.
	for di, d := range sc.Dialogue {
		out[di%len(out)].Dialogue = append(out[di%len(out)].Dialogue, d)
	}
	narrParts := splitTextChunks(sc.Narration, 400)
	for ni, np := range narrParts {
		tgt := &out[ni%len(out)]
		if tgt.Narration != "" {
			tgt.Narration += " "
		}
		tgt.Narration += np
	}
	return out
}

// shotSpokenText gộp thoại + lời dẫn của một shot thành văn bản phụ đề.
func shotSpokenText(sh RenderShot) string {
	var parts []string
	for _, d := range sh.Dialogue {
		if t := strings.TrimSpace(d.Text); t != "" {
			parts = append(parts, t)
		}
	}
	if t := strings.TrimSpace(sh.Narration); t != "" {
		parts = append(parts, t)
	}
	return strings.Join(parts, " ")
}

// performanceBlock dựng khối diễn xuất cho prompt Veo: biểu cảm khuôn mặt
// + hành động cụ thể + cảm xúc thoại lấy từ kịch bản. Veo KHÔNG được tự
// đoán diễn xuất — mọi chỉ dẫn phải có trong kịch bản (Ninh 2026-10-02).
func performanceBlock(sh RenderShot) string {
	var sb strings.Builder
	if a := strings.TrimSpace(sh.Action); a != "" {
		sb.WriteString("\nPERFORMANCE — exact acting direction, do not improvise: ")
		sb.WriteString(a)
	}
	var emos []string
	for _, d := range sh.Dialogue {
		e := strings.TrimSpace(d.Emotion)
		if e == "" {
			continue
		}
		who := strings.TrimSpace(d.Character)
		if who == "" {
			who = "character"
		}
		emos = append(emos, who+" shows "+e)
	}
	if len(emos) > 0 {
		sb.WriteString("\nFACIAL EXPRESSIONS / EMOTIONS: ")
		sb.WriteString(strings.Join(emos, "; "))
	}
	return sb.String()
}

// renderedShot là một RenderShot đã dựng xong, kèm clip trên đĩa.
type renderedShot struct {
	Shot RenderShot
	MP4  string
	Dur  float64
}

// extractLastFrame cắt frame cuối của clip để làm firstFrame cho Veo của
// shot tiếp theo — mắt xích giữ chuyển động liền mạch giữa các shot.
func extractLastFrame(ctx context.Context, mp4, outPng string) error {
	return ffmpegRun(ctx,
		"-sseof", "-0.5", "-i", mp4,
		"-frames:v", "1", "-q:v", "2", outPng)
}

// ProbeDuration trả về thời lượng giây của file media (0 khi lỗi).
func ProbeDuration(ctx context.Context, path string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var d float64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d)
	if d < 0 {
		return 0
	}
	return d
}

// ProbeHasAudio báo file có track audio không.
func ProbeHasAudio(ctx context.Context, path string) bool {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "a",
		"-show_entries", "stream=index", "-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// Trailer dài 45–60s, cắt từ các shot trailer_worthy.
const (
	trailerMinSecs = 45
	trailerMaxSecs = 60
)

// selectTrailerGroups chia các shot trailer_worthy thành tối đa 2 nhóm
// (nửa đầu / nửa sau phim → 2 trailer khác nhau). Mỗi nhóm lấy shot cho
// đến khi đủ 45s, cắt cứng ở 60s. Không có shot nào → nil (bỏ qua, không lỗi).
func selectTrailerGroups(marked []renderedShot) [][]renderedShot {
	if len(marked) == 0 {
		return nil
	}
	mid := (len(marked) + 1) / 2
	halves := [][]renderedShot{marked[:mid]}
	if mid < len(marked) {
		halves = append(halves, marked[mid:])
	}
	var groups [][]renderedShot
	for _, h := range halves {
		var g []renderedShot
		var total float64
		for _, m := range h {
			if total >= trailerMinSecs {
				break
			}
			g = append(g, m)
			total += m.Dur
		}
		for total > trailerMaxSecs && len(g) > 1 {
			total -= g[len(g)-1].Dur
			g = g[:len(g)-1]
		}
		if len(g) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}
