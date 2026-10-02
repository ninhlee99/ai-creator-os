package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Director chuyên nghiệp (yêu cầu Ninh 2026-10-01):
//   - Video affiliate 30s: list ảnh 5–10 ảnh.
//   - Video từ 1 phút: shot list điện ảnh như có cameraman/photographer/
//     editor thật — cỡ cảnh, chuyển động máy, ống kính, ánh sáng, story arc.
//   - Phim ngắn: kịch bản + character bible khóa nhân vật, mỗi cảnh khóa
//     địa điểm/thời gian/không gian/ánh sáng để đồng nhất giữa các phân cảnh.

// ---------------------------------------------------------------------------
// shot điện ảnh
// ---------------------------------------------------------------------------

// ProShot là một shot quay theo ngôn ngữ điện ảnh.
type ProShot struct {
	Index         int    `json:"index"`
	Purpose       string `json:"purpose"`     // hook | build | payoff …
	ShotSize      string `json:"shot_size"`   // ECU, CU, MCU, MS, WS, EWS, drone …
	CameraMove    string `json:"camera_move"` // dolly-in, tracking, handheld, static …
	LensLight     string `json:"lens_light"`  // 35mm f/1.8, golden hour …
	Seconds       int    `json:"seconds"`
	Action        string `json:"action"`         // diễn xuất/hành động (tiếng Việt)
	ImagePrompt   string `json:"image_prompt"`   // prompt ảnh (tiếng Anh)
	VideoPrompt   string `json:"video_prompt"`   // prompt video (tiếng Anh)
	TrailerWorthy bool   `json:"trailer_worthy"` // shot đắt giá → cắt trailer 9:16
}

// ---------------------------------------------------------------------------
// character bible — khóa nhân vật cho phim
// ---------------------------------------------------------------------------

// Character là khóa ngoại hình bất biến của một nhân vật trong phim.
// Mọi prompt sinh ảnh/video của mọi cảnh đều gắn nguyên khối này.
type Character struct {
	Name       string `json:"name"`
	Appearance string `json:"appearance"` // tuổi, khuôn mặt, tóc, da, dáng — tiếng Anh
	Wardrobe   string `json:"wardrobe"`   // trang phục theo cảnh — tiếng Anh
	Portrait   string `json:"-"`          // path ảnh chân dung đã sinh (làm ref)
}

// LockBlock trả về khối khóa nhân vật để gắn vào mọi prompt.
func (c Character) LockBlock() string {
	return fmt.Sprintf("CHARACTER LOCK — %s: %s. Wardrobe: %s. "+
		"Keep the EXACT same face, hairstyle, age and identity in every shot.",
		c.Name, c.Appearance, c.Wardrobe)
}

// DialogueLine là một câu thoại có cảm xúc.
type DialogueLine struct {
	Character string `json:"character"`
	Text      string `json:"text"`    // tiếng Việt
	Emotion   string `json:"emotion"` // vui, buồn, căng thẳng …
}

// FilmScenePro là một cảnh phim với đầy đủ khóa bối cảnh.
type FilmScenePro struct {
	Index       int            `json:"index"`
	Act         int            `json:"act,omitempty"` // hồi 1|2|3 (phim dài), 0 = phim ngắn
	Location    string         `json:"location"`      // khóa địa điểm — tiếng Anh
	TimeOfDay   string         `json:"time_of_day"`   // khóa thời gian — tiếng Anh
	Atmosphere  string         `json:"atmosphere"`    // khóa không gian/ánh sáng — tiếng Anh
	Seconds     int            `json:"seconds"`
	Shots       []ProShot      `json:"shots"`
	Dialogue    []DialogueLine `json:"dialogue"`
	ImagePrompt string         `json:"image_prompt"` // keyframe của cảnh — tiếng Anh
	Narration   string         `json:"narration"`    // lời dẫn (nếu có) — tiếng Việt
	// Continuity là khối bất di bất dịch của cảnh: trang phục chi tiết từng
	// nhân vật, tóc, đạo cụ, hướng ánh sáng, thời tiết. MỌI shot trong cảnh
	// kế thừa NGUYÊN VĂN vào prompt (không diễn đạt lại) — continuity lock
	// từng khung hình (Ninh 2026-10-02).
	Continuity string `json:"continuity,omitempty"`
}

// SceneLockBlock khóa bối cảnh của cảnh để gắn vào prompt từng shot.
func (s FilmScenePro) SceneLockBlock() string {
	return fmt.Sprintf("SCENE LOCK — location: %s; time: %s; atmosphere/lighting: %s. "+
		"Every shot of this scene must show the SAME place, time and lighting.",
		s.Location, s.TimeOfDay, s.Atmosphere)
}

// FilmScriptPro là kịch bản phim ngắn chuyên nghiệp.
type FilmScriptPro struct {
	Title      string         `json:"title"`
	Logline    string         `json:"logline"`
	Genre      string         `json:"genre,omitempty"`
	Characters []Character    `json:"characters"`
	Scenes     []FilmScenePro `json:"scenes"`
}

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

func parseDirectorJSON(text string, v any) error {
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "```"))
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("director JSON: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 60s affiliate: shot list điện ảnh
// ---------------------------------------------------------------------------

// WriteProShotList viết shot list điện ảnh cho video affiliate dài (>= 60s):
// story arc hook -> build -> payoff, mỗi shot có cỡ cảnh, chuyển động máy,
// ống kính/ánh sáng như cameraman thật. Không chữ, không voiceover.
func WriteProShotList(ctx context.Context, llm LLM, productName, niche string, seconds int) ([]ProShot, error) {
	n := seconds / 7
	if n < 6 {
		n = 6
	}
	if n > 10 {
		n = 10
	}
	sys := "Bạn là đạo diễn quảng cáo TikTok Việt Nam đẳng cấp quốc tế. Chỉ trả lời JSON thuần."
	prompt, err := directorPrompt("affiliate_shotlist.txt", promptData{
		Topic: productName, Niche: niche, Seconds: seconds, N: n,
	})
	if err != nil {
		return nil, err
	}
	text, err := llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("director: %w", err)
	}
	var plan struct {
		Shots []ProShot `json:"shots"`
	}
	if err := parseDirectorJSON(text, &plan); err != nil {
		return nil, err
	}
	var out []ProShot
	for _, sh := range plan.Shots {
		if strings.TrimSpace(sh.ImagePrompt) == "" {
			continue
		}
		if sh.Seconds < 3 {
			sh.Seconds = 5
		}
		out = append(out, sh)
	}
	if len(out) < 4 {
		return nil, fmt.Errorf("director: chỉ có %d shot, cần >= 4", len(out))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// phim ngắn: kịch bản chuyên nghiệp
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// phim: kịch bản chuyên nghiệp (chuẩn điện ảnh — luật ở docs/FILM_RULES.md)
// ---------------------------------------------------------------------------

// defaultFilmGenre là thể loại khi người dùng không chọn.
const defaultFilmGenre = "Tâm lý"

// WriteFilmScript viết kịch bản phim chuyên nghiệp: character bible khóa
// nhân vật + từng cảnh khóa địa điểm/thời gian/không gian + shot điện ảnh
// + thoại có cảm xúc. aspect ("9:16"|"16:9") tells the director the
// delivery frame so the shot list is composed for the real frame.
//
// Phim ≤180s: một lần gọi LLM (cấu trúc 3 hồi thu nhỏ). Phim dài hơn:
// viết theo 3 hồi, mỗi hồi một lần gọi — vừa ép đúng cấu trúc điện ảnh
// (hook 60s đầu → bước ngoặt giữa → cao trào + kết), vừa không vượt giới
// hạn độ dài một response của LLM.
func WriteFilmScript(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string) (FilmScriptPro, error) {
	if strings.TrimSpace(genre) == "" {
		genre = defaultFilmGenre
	}
	if seconds <= 180 {
		return writeFilmScriptShort(ctx, llm, topic, genre, seconds, aspect)
	}
	return writeFilmScriptThreeActs(ctx, llm, topic, genre, seconds, aspect)
}

// writeFilmScriptShort viết kịch bản phim ngắn trong một lần gọi LLM.
func writeFilmScriptShort(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string) (FilmScriptPro, error) {
	var script FilmScriptPro
	n := seconds / 20
	if n < 2 {
		n = 2
	}
	if n > 6 {
		n = 6
	}
	prompt, err := directorPrompt("film_short.txt", promptData{
		Topic: topic, Genre: genre, Seconds: seconds, N: n,
		Orient: orientationWord(aspect),
	})
	if err != nil {
		return script, err
	}
	text, err := llm.Complete(ctx,
		"Bạn là biên kịch phim Việt Nam. Chỉ trả lời JSON thuần, không giải thích.",
		prompt)
	if err != nil {
		return script, fmt.Errorf("director: %w", err)
	}
	if err := parseDirectorJSON(text, &script); err != nil {
		return script, err
	}
	script.Genre = genre
	if len(script.Characters) == 0 || len(script.Scenes) == 0 {
		return script, fmt.Errorf("director: kịch bản rỗng (0 nhân vật / 0 cảnh)")
	}
	return script, nil
}

// actDef mô tả một hồi của phim dài: tỉ trọng thời lượng + vai trò kịch bản.
type actDef struct {
	num      int
	share    float64
	role     string
	template string
}

// writeFilmScriptThreeActs viết kịch bản phim dài theo đúng 3 hồi điện ảnh.
// Hồi 1 dựng character bible + gieo hook; hồi 2/3 nhận bible + recap các hồi
// trước để viết tiếp mà không lặp hay lệch nhân vật.
func writeFilmScriptThreeActs(ctx context.Context, llm LLM, topic, genre string, seconds int, aspect string) (FilmScriptPro, error) {
	var script FilmScriptPro
	script.Genre = genre
	acts := []actDef{
		{1, 0.20, "Mở đầu", "film_act1.txt"},
		{2, 0.55, "Diễn biến", "film_act2.txt"},
		{3, 0.25, "Kết", "film_act3.txt"},
	}
	sys := "Bạn là biên kịch phim Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	var recap1, recap2, charJSON string
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
			Characters: charJSON, Recap1: recap1, Recap2: recap2,
		})
		if err != nil {
			return script, err
		}
		text, err := llm.Complete(ctx, sys, prompt)
		if err != nil {
			return script, fmt.Errorf("director hồi %d (%s): %w", a.num, a.role, err)
		}
		var act struct {
			Title      string         `json:"title"`
			Logline    string         `json:"logline"`
			Characters []Character    `json:"characters"`
			Scenes     []FilmScenePro `json:"scenes"`
			Recap      string         `json:"recap"`
		}
		if err := parseDirectorJSON(text, &act); err != nil {
			return script, fmt.Errorf("director hồi %d: %w", a.num, err)
		}
		if a.num == 1 {
			script.Title = act.Title
			script.Logline = act.Logline
			script.Characters = act.Characters
			if len(script.Characters) == 0 {
				return script, fmt.Errorf("director hồi 1: thiếu character bible")
			}
			var cb strings.Builder
			for _, c := range script.Characters {
				fmt.Fprintf(&cb, "- %s: %s | Trang phục: %s\n", c.Name, c.Appearance, c.Wardrobe)
			}
			charJSON = cb.String()
		}
		if len(act.Scenes) == 0 {
			return script, fmt.Errorf("director hồi %d (%s): 0 cảnh", a.num, a.role)
		}
		for i := range act.Scenes {
			act.Scenes[i].Act = a.num
			act.Scenes[i].Index = sceneBase + i + 1
		}
		sceneBase += len(act.Scenes)
		script.Scenes = append(script.Scenes, act.Scenes...)
		if a.num == 1 {
			recap1 = act.Recap
		} else if a.num == 2 {
			recap2 = act.Recap
		}
	}
	if len(script.Scenes) == 0 {
		return script, fmt.Errorf("director: kịch bản rỗng (0 cảnh)")
	}
	return script, nil
}

// CharacterRefs trả về ImageRef của các chân dung nhân vật đã sinh (để khóa
// identity cho mọi cảnh).
func (s FilmScriptPro) CharacterRefs() []ImageRef {
	var out []ImageRef
	for _, c := range s.Characters {
		if c.Portrait != "" {
			out = append(out, ImageRef{Path: c.Portrait})
		}
	}
	return out
}
