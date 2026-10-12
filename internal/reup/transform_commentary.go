package reup

import (
	"context"
	"fmt"
	"strings"
)

func (t *Transformer) llmCommentary(ctx context.Context, vs []Video) string {
	var sb strings.Builder
	for i, v := range vs {
		fmt.Fprintf(&sb, "- Video %d: tiêu đề %q, tác giả %q", i+1, v.Title, v.Author)
		if v.PlayCount > 0 {
			fmt.Fprintf(&sb, ", %d lượt xem", v.PlayCount)
		}
		sb.WriteString("\n")
	}
	// Văn nói thân thiện với TTS: câu ngắn, từ dễ đọc, có móc tò mò,
	// đúng chất tiên hiệp/thần tiên; tuyệt đối không bịa tình tiết.
	system := "Bạn là người dẫn chuyện tiên hiệp, đang kể cho bạn bè nghe " +
		"bằng giọng tự nhiên, hào hứng. Yêu cầu: văn NÓI (câu ngắn, từ ngữ " +
		"đời thường dễ đọc thành tiếng, không dùng ký tự đặc biệt/ký hiệu); " +
		"câu đầu phải có móc tò mò khiến người nghe muốn xem tiếp; " +
		"CHỈ dùng thông tin đã cho, tuyệt đối không bịa tình tiết, không " +
		"khẳng định điều không có trong dữ liệu; tối đa 300 ký tự."
	n := len(vs)
	kind := "một video"
	if n > 1 {
		kind = fmt.Sprintf("tuyển tập %d video", n)
	}
	out, err := t.LLM.Complete(ctx, system,
		fmt.Sprintf("Viết lời dẫn cho %s:\n%s", kind, sb.String()))
	if err != nil {
		return ""
	}
	return sanitizeCommentary(out)
}

// templateCommentary: dự phòng trung thực khi không có LLM.
// 3 biến thể xoay theo seed để các video không đọc cùng một câu.
func templateCommentary(vs []Video, seed int64) string {
	if len(vs) > 1 {
		variants := []string{
			fmt.Sprintf("Tuyển tập %d khoảnh khắc tiên hiệp đang gây sốt trên Douyin — mở đầu đã cuốn, xem tiếp nhé.", len(vs)),
			fmt.Sprintf("Gom %d cảnh đẹp nhất dòng tiên hiệp Douyin tuần này. Cảnh nào cũng đáng xem.", len(vs)),
			fmt.Sprintf("%d video tiên hiệp hot nhất Douyin trong một clip — bạn đoán được cảnh cuối không?", len(vs)),
		}
		return variants[int(seed%3+3)%3]
	}
	v := vs[0]
	var core strings.Builder
	if strings.TrimSpace(v.Title) != "" {
		fmt.Fprintf(&core, "Video %q", strings.TrimSpace(v.Title))
	} else {
		core.WriteString("Video này")
	}
	if strings.TrimSpace(v.Author) != "" {
		fmt.Fprintf(&core, " của %s", strings.TrimSpace(v.Author))
	}
	if v.PlayCount > 0 {
		fmt.Fprintf(&core, ", đang viral trên Douyin với %d lượt xem", v.PlayCount)
	} else {
		core.WriteString(", đang được chú ý trên Douyin")
	}
	variants := []string{
		core.String() + ". Mở đầu đã gay cấn — xem tiếp nhé.",
		"Bạn phải xem " + core.String() + ". Càng xem càng cuốn.",
		core.String() + ". Tin tôi đi, đoạn cuối mới là đỉnh nhất.",
	}
	return variants[int(seed%3+3)%3]
}

// sanitizeCommentary làm sạch text LLM: gọn whitespace, cắt 300 ký tự.
func sanitizeCommentary(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, "\"“”")
	if len([]rune(s)) > 300 {
		r := []rune(s)
		s = string(r[:297]) + "…"
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// Finalize: trộn voiceover/nhạc lên hình đã biến đổi.
//
// Một pass ffmpeg: giữ nguyên hình (copy stream) + voiceover delay nhẹ
// + nhạc nền nhỏ + loudnorm. KHÔNG chèn chữ vào video — không card
// intro/outro, không caption (Ninh chốt 2026-10-12). Lời bình luận chỉ
// tồn tại dưới dạng giọng đọc.
// ---------------------------------------------------------------------------

const voiceLead = 0.3 // giây — voiceover vào gần như ngay để giữ hook giây đầu

// finalFilter dựng filter_complex audio cho pass finalize.
// voiceIdx/musicIdx: -1 = không có input tương ứng (cả hai -1 → "").
func finalFilter(mainDur float64, voiceIdx, musicIdx int) string {
	var sb strings.Builder
	if voiceIdx >= 0 {
		fmt.Fprintf(&sb, "[%d:a]aresample=44100,adelay=%d|%d,apad[vo];",
			voiceIdx, int(voiceLead*1000), int(voiceLead*1000))
	}
	if musicIdx >= 0 {
		fmt.Fprintf(&sb, "[%d:a]atrim=0:%.2f,afade=t=in:st=0:d=1,afade=t=out:st=%.2f:d=1,volume=0.35[mu];",
			musicIdx, mainDur, mainDur-1)
	}
	switch {
	case voiceIdx >= 0 && musicIdx >= 0:
		sb.WriteString("[vo][mu]amix=inputs=2:duration=first:dropout_transition=0," +
			"loudnorm=I=-14:TP=-1.5:LRA=11[aout]")
	case voiceIdx >= 0:
		sb.WriteString("[vo]loudnorm=I=-14:TP=-1.5:LRA=11[aout]")
	case musicIdx >= 0:
		sb.WriteString("[mu]loudnorm=I=-14:TP=-1.5:LRA=11[aout]")
	}
	return strings.TrimSuffix(sb.String(), ";")
}

// finalizeTransform pass cuối: card + audio mix → outPath, rồi caption.
