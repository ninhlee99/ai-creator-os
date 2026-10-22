//go:build parked

package studio

import (
	"context"
	"fmt"
	"strings"
)

func WriteFilmScript(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string) (FilmScriptPro, error) {
	b, err := WriteFilmScriptBundle(ctx, llm, topic, genre, seconds, aspect, nil)
	if err != nil {
		return FilmScriptPro{}, err
	}
	return b.Script, nil
}

// WriteFilmScriptBundle là WriteFilmScript nhưng trả cả bundle từng pha —
// studio lưu vào script.json để storyboard UI xem lại; log callback nhận
// tiến trình từng pha để hiện lên UI job.
func WriteFilmScriptBundle(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string, log func(string)) (FilmScriptBundle, error) {
	var bundle FilmScriptBundle
	if strings.TrimSpace(genre) == "" {
		genre = defaultFilmGenre
	}
	say := func(s string) {
		if log != nil {
			log(s)
		}
	}
	// Pha 0 — Truyện: linh hồn của phim.
	say("Pha 0/3 — nhà văn viết truyện…")
	story, err := writeStory(ctx, llm, topic, genre, seconds, aspect)
	if err != nil {
		return bundle, err
	}
	bundle.Story = story
	// Pha 1 — Screenplay: chuyển thể truyện thành kịch bản điện ảnh.
	say("Pha 0 xong — pha 1/3 chuyển thể kịch bản từ truyện…")
	var sp Screenplay
	if seconds <= 180 {
		sp, err = writeScreenplayShort(ctx, llm, topic, genre, seconds, aspect, story)
	} else {
		sp, err = writeScreenplayThreeActs(ctx, llm, topic, genre, seconds, aspect, story)
	}
	if err != nil {
		return bundle, err
	}
	bundle.Screenplay = sp
	// Pha 2 — Breakdown: một lần gọi trên toàn bộ kịch bản.
	say(fmt.Sprintf("Pha 1 xong (%d cảnh) — pha 2/3 breakdown nhân vật + shot…", len(sp.Scenes)))
	bd, err := breakdownScreenplay(ctx, llm, sp, aspect)
	if err != nil {
		return bundle, err
	}
	bundle.Breakdown = bd
	script, err := assembleScript(sp, bd)
	if err != nil {
		return bundle, err
	}
	script.Genre = genre
	bundle.Script = script
	say(fmt.Sprintf("Breakdown xong: %d nhân vật, %d cảnh, %d shot.",
		len(script.Characters), len(script.Scenes), countShots(script)))
	return bundle, nil
}

// storyWords ước lượng độ dài truyện theo thời lượng phim: ~2 từ/giây phim,
// kẹp 300–6000 từ (tiểu thuyết mini cho phim dài).
func storyWords(seconds int) int {
	w := seconds * 2
	if w < 300 {
		w = 300
	}
	if w > 6000 {
		w = 6000
	}
	return w
}

// writeStory — pha 0: một lần gọi LLM viết truyện ngắn/tiểu thuyết mini hoàn
// chỉnh (văn xuôi) — "linh hồn" để pha 1 chuyển thể.
func writeStory(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string) (Story, error) {
	var st Story
	prompt, err := directorPrompt("story.txt", promptData{
		Topic: topic, Genre: genre, Seconds: seconds,
		StoryWords: storyWords(seconds), Orient: orientationWord(aspect),
	})
	if err != nil {
		return st, err
	}
	text, err := llm.Complete(ctx,
		"Bạn là nhà văn Việt Nam. Chỉ trả lời JSON thuần, không giải thích.",
		prompt)
	if err != nil {
		return st, fmt.Errorf("director pha 0: %w", err)
	}
	if err := parseDirectorJSON(text, &st); err != nil {
		return st, err
	}
	st.Genre = genre
	if strings.TrimSpace(st.Text) == "" {
		return st, fmt.Errorf("director pha 0: truyện rỗng")
	}
	return st, nil
}

// writeScreenplayShort — pha 1 cho phim ngắn: một lần gọi LLM chuyển thể
// truyện pha 0 thành kịch bản đầy đủ (3 hồi thu nhỏ), chưa cần character
// sheet hay shot list.
func writeScreenplayShort(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string, story Story) (Screenplay, error) {
	var sp Screenplay
	n := seconds / 20
	if n < 2 {
		n = 2
	}
	if n > 6 {
		n = 6
	}
	prompt, err := directorPrompt("screenplay.txt", promptData{
		Topic: topic, Genre: genre, Seconds: seconds, N: n,
		Orient: orientationWord(aspect), Story: story.Text,
	})
	if err != nil {
		return sp, err
	}
	text, err := llm.Complete(ctx,
		"Bạn là biên kịch chuyển thể phim Việt Nam. Chỉ trả lời JSON thuần, không giải thích.",
		prompt)
	if err != nil {
		return sp, fmt.Errorf("director pha 1: %w", err)
	}
	if err := parseDirectorJSON(text, &sp); err != nil {
		return sp, err
	}
	sp.Genre = genre
	if len(sp.Scenes) == 0 {
		return sp, fmt.Errorf("director pha 1: kịch bản rỗng (0 cảnh)")
	}
	return sp, nil
}

// writeScreenplayThreeActs — pha 1 cho phim dài: chuyển thể truyện pha 0
// thành kịch bản theo 3 hồi, mỗi hồi một lần gọi. Không dựng character bible
// ở hồi 1 (việc của pha 2); dàn nhân vật được trích XÁC ĐỊNH từ tên trong
// thoại các hồi trước để hồi sau giữ đúng tên mà không phụ thuộc LLM.
func writeScreenplayThreeActs(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string, story Story) (Screenplay, error) {
	var sp Screenplay
	sp.Genre = genre
	acts := []actDef{
		{1, 0.20, "Mở đầu", "film_act1.txt"},
		{2, 0.55, "Diễn biến", "film_act2.txt"},
		{3, 0.25, "Kết", "film_act3.txt"},
	}
	sys := "Bạn là biên kịch chuyển thể phim Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	var recap1, recap2 string
	sceneBase := 0
	for _, a := range acts {
		actSecs := int(float64(seconds) * a.share)
		n := actSecs / 120
		if n < 3 {
			n = 3
		}
		if n > 10 {
			n = 10
		}
		prompt, err := directorPrompt(a.template, promptData{
			Topic: topic, Genre: genre, TotalSeconds: seconds,
			ActSeconds: actSecs, N: n, Orient: orientationWord(aspect),
			Story: story.Text,
			Cast:  screenplayCast(sp), Recap1: recap1, Recap2: recap2,
		})
		if err != nil {
			return sp, err
		}
		text, err := llm.Complete(ctx, sys, prompt)
		if err != nil {
			return sp, fmt.Errorf("director pha 1 hồi %d (%s): %w", a.num, a.role, err)
		}
		var act struct {
			Title   string            `json:"title"`
			Logline string            `json:"logline"`
			Scenes  []ScreenplayScene `json:"scenes"`
			Recap   string            `json:"recap"`
		}
		if err := parseDirectorJSON(text, &act); err != nil {
			return sp, fmt.Errorf("director pha 1 hồi %d: %w", a.num, err)
		}
		if a.num == 1 {
			sp.Title = act.Title
			sp.Logline = act.Logline
		}
		if len(act.Scenes) == 0 {
			return sp, fmt.Errorf("director pha 1 hồi %d (%s): 0 cảnh", a.num, a.role)
		}
		for i := range act.Scenes {
			act.Scenes[i].Act = a.num
			act.Scenes[i].Index = sceneBase + i + 1
		}
		sceneBase += len(act.Scenes)
		sp.Scenes = append(sp.Scenes, act.Scenes...)
		if a.num == 1 {
			recap1 = act.Recap
		} else if a.num == 2 {
			recap2 = act.Recap
		}
	}
	if len(sp.Scenes) == 0 {
		return sp, fmt.Errorf("director pha 1: kịch bản rỗng (0 cảnh)")
	}
	return sp, nil
}

// screenplayCast trích dàn nhân vật XÁC ĐỊNH từ tên trong thoại các cảnh đã
// viết (theo thứ tự xuất hiện) — hồi sau giữ đúng tên, không phụ thuộc LLM
// diễn đạt lại.
func screenplayCast(sp Screenplay) string {
	var names []string
	seen := map[string]bool{}
	for _, sc := range sp.Scenes {
		for _, d := range sc.Dialogue {
			nm := strings.TrimSpace(d.Character)
			if nm == "" || seen[nm] {
				continue
			}
			seen[nm] = true
			names = append(names, "- "+nm)
		}
	}
	if len(names) == 0 {
		return "(chưa có nhân vật thoại — hồi này tự đặt tên)"
	}
	return strings.Join(names, "\n")
}

// breakdownScreenplay — pha 2: MỘT lần gọi LLM đọc toàn bộ kịch bản pha 1,
// trích xuất character bible (thiết kế PHẢI lý giải từ nhu cầu kịch bản) +
// khóa bối cảnh + continuity + shot list từng cảnh.
