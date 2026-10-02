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
	Index       int    `json:"index"`
	Purpose     string `json:"purpose"`     // hook | build | payoff …
	ShotSize    string `json:"shot_size"`   // ECU, CU, MCU, MS, WS, EWS, drone …
	CameraMove  string `json:"camera_move"` // dolly-in, tracking, handheld, static …
	LensLight   string `json:"lens_light"`  // 35mm f/1.8, golden hour …
	Seconds     int    `json:"seconds"`
	Action      string `json:"action"`       // diễn xuất/hành động (tiếng Việt)
	ImagePrompt string `json:"image_prompt"` // prompt ảnh (tiếng Anh)
	VideoPrompt string `json:"video_prompt"` // prompt video (tiếng Anh)
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
	Location    string         `json:"location"`    // khóa địa điểm — tiếng Anh
	TimeOfDay   string         `json:"time_of_day"` // khóa thời gian — tiếng Anh
	Atmosphere  string         `json:"atmosphere"`  // khóa không gian/ánh sáng — tiếng Anh
	Seconds     int            `json:"seconds"`
	Shots       []ProShot      `json:"shots"`
	Dialogue    []DialogueLine `json:"dialogue"`
	ImagePrompt string         `json:"image_prompt"` // keyframe của cảnh — tiếng Anh
	Narration   string         `json:"narration"`    // lời dẫn (nếu có) — tiếng Việt
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
	prompt := fmt.Sprintf(`Sản phẩm: %s. Niche: %s. Video dài %ds, KHÔNG chữ, KHÔNG voiceover, chỉ hình + nhạc.

Viết %d shot theo story arc: shot 1 là HOOK (3s đầu giữ chân), giữa BUILD (lifestyle, macro chất liệu, khoảnh khắc dùng sản phẩm thật), cuối PAYOFF ấm áp ôm sản phẩm.

Mỗi shot gồm:
- purpose: hook | build | payoff
- shot_size: ECU/CU/MCU/MS/WS/EWS/aerial — chọn như cameraman thật
- camera_move: dolly-in, dolly-out, tracking, pan, handheld nhẹ, static…
- lens_light: ví dụ "35mm f/1.8, golden hour" / "macro 100mm, softbox"
- seconds: 4-9
- action: mẫu nữ Việt Nam diễn gì với sản phẩm (tiếng Việt, chi tiết, hành động hợp lý)
- image_prompt, video_prompt: tiếng Anh, photorealistic, vertical 9:16, KHÔNG text/watermark

Chỉ trả JSON: {"shots": [{"index":1,"purpose":"hook","shot_size":"CU","camera_move":"dolly-in","lens_light":"35mm f/1.8, golden hour","seconds":5,"action":"...","image_prompt":"...","video_prompt":"..."}]}`,
		productName, niche, seconds, n)
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

// WriteFilmScript viết kịch bản phim ngắn chuyên nghiệp: character bible
// khóa nhân vật + từng cảnh khóa địa điểm/thời gian/không gian + shot điện
// ảnh + thoại có cảm xúc. aspect ("9:16"|"16:9") tells the director the
// delivery frame so the shot list is composed for the real format.
func WriteFilmScript(ctx context.Context, llm LLM, topic string, seconds int, aspect string) (FilmScriptPro, error) {
	var script FilmScriptPro
	n := seconds / 20
	if n < 2 {
		n = 2
	}
	if n > 6 {
		n = 6
	}
	sys := "Bạn là biên kịch phim ngắn Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	prompt := fmt.Sprintf(`Chủ đề phim: %s. Phim dài khoảng %ds, gồm %d cảnh, %s.

Viết kịch bản NHƯ PHIM THẬT:
1. characters: 1-3 nhân vật, mỗi người có name + appearance (tuổi, khuôn mặt, tóc, da, dáng người — tiếng Anh, CHI TIẾT để khóa identity) + wardrobe (trang phục — tiếng Anh).
2. Mỗi scene có:
   - location: địa điểm CỤ THỂ khóa cứng (tiếng Anh)
   - time_of_day: thời gian khóa cứng (tiếng Anh, vd "late afternoon golden hour")
   - atmosphere: không gian/ánh sáng/không khí khóa cứng (tiếng Anh)
   - seconds: 12-25
   - shots: 2-4 shot, mỗi shot có shot_size, camera_move, lens_light, action (tiếng Việt), image_prompt (tiếng Anh, photorealistic cinematic, KHÔNG text)
   - dialogue: 1-4 câu thoại tiếng Việt, mỗi câu có character, text, emotion (vui/buồn/căng thẳng/dịu dàng…)
   - image_prompt: keyframe đại diện cảnh (tiếng Anh)
   - narration: lời dẫn ngắn (tiếng Việt) hoặc "" nếu thoại đã đủ

Yêu cầu: nhân vật NHẤT QUÁN mọi cảnh, bối cảnh mỗi cảnh NHẤT QUÁN mọi shot, thoại rõ ràng mạch lạc có cảm xúc, có mở-thân-kết.

Chỉ trả JSON: {"title":"...","logline":"...","characters":[{"name":"...","appearance":"...","wardrobe":"..."}],"scenes":[{"index":1,"location":"...","time_of_day":"...","atmosphere":"...","seconds":18,"shots":[{"shot_size":"WS","camera_move":"slow dolly-in","lens_light":"35mm, golden hour","action":"..."}],"dialogue":[{"character":"...","text":"...","emotion":"..."}],"image_prompt":"...","narration":"..."}]}`,
		topic, seconds, n, orientationWord(aspect))
	text, err := llm.Complete(ctx, sys, prompt)
	if err != nil {
		return script, fmt.Errorf("director: %w", err)
	}
	if err := parseDirectorJSON(text, &script); err != nil {
		return script, err
	}
	if len(script.Characters) == 0 || len(script.Scenes) == 0 {
		return script, fmt.Errorf("director: kịch bản rỗng (0 nhân vật / 0 cảnh)")
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
