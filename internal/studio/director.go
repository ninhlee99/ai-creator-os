package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
// DesignNote lý giải thiết kế từ nhu cầu kịch bản (pha 2 breakdown viết) —
// vd "váy vàng cảnh 4 = sự hồi sinh". Không gắn vào prompt khóa, chỉ để
// đọc trong bible.
type Character struct {
	Name       string `json:"name"`
	Appearance string `json:"appearance"` // tuổi, khuôn mặt, tóc, da, dáng — tiếng Anh
	Wardrobe   string `json:"wardrobe"`   // trang phục theo cảnh — tiếng Anh
	DesignNote string `json:"design_note,omitempty"`
	Portrait   string `json:"-"` // path ảnh chân dung đã sinh (làm ref)
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
	Action      string         `json:"action,omitempty"` // diễn biến đầy đủ từ kịch bản pha 1 — tiếng Việt
	Shots       []ProShot      `json:"shots"`
	Dialogue    []DialogueLine `json:"dialogue"`
	ImagePrompt string         `json:"image_prompt"` // keyframe của cảnh — tiếng Anh
	Narration   string         `json:"narration"`    // lời dẫn (nếu có) — tiếng Việt
	// Props là đạo cụ quan trọng của cảnh (pha 2 breakdown trích xuất).
	Props []string `json:"props,omitempty"`
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

// ---------------------------------------------------------------------------
// DIRECTOR 3 PHA — quy trình chuyển thể điện ảnh chuẩn (Ninh 2026-10-02).
//
// QUY TẮC BẤT DI BẤT DỊCH: "truyện → kịch bản → breakdown → storyboard →
// quay → dựng".
//   Pha 0 — Truyện: LLM viết truyện ngắn/tiểu thuyết mini HOÀN CHỈNH (văn
//     xuôi, giàu chi tiết, cảm xúc, đối thoại tự nhiên) — đây là "linh hồn".
//   Pha 1 — Screenplay: chuyển thể truyện thành kịch bản điện ảnh (cảnh,
//     action, thoại, chỉ dẫn quay), CHƯA cần character sheet hay shot list.
//   Pha 2 — Breakdown: LLM đọc kịch bản pha 1 và trích xuất character bible
//     (ngoại hình/trang phục PHẢI lý giải từ nhu cầu kịch bản — vd "váy vàng
//     cảnh 4 = sự hồi sinh") + khóa bối cảnh + continuity + shot list từng cảnh.
// Sau đó mới expand shot (expandSceneShots) → quay (cinematic/Veo) → dựng.
// Output từng pha được lưu vào script.json (FilmScriptBundle) để storyboard
// UI xem lại khi cần.
// (docs/FILM_RULES.md do agent docs quản — rule này ghi trong báo cáo Wave 3
// để parent cập nhật docs.)

// Story là truyện pha 0 — "linh hồn" của phim, đầu vào cho pha 1 chuyển thể.
type Story struct {
	Title   string `json:"title"`
	Logline string `json:"logline"`
	Genre   string `json:"genre,omitempty"`
	Text    string `json:"text"` // văn xuôi hoàn chỉnh
}

// FilmScriptBundle lưu output từng pha director vào script.json — storyboard
// UI đọc để xem lại truyện/kịch bản/breakdown khi cần. Script là bản ráp cuối
// (quay + dựng dùng bản này). Job cũ (Wave 1–2) chỉ có FilmScriptPro trong
// script.json — loadScriptBundle tự tương thích ngược.
type FilmScriptBundle struct {
	Story      Story         `json:"story"`
	Screenplay Screenplay    `json:"screenplay"`
	Breakdown  Breakdown     `json:"breakdown"`
	Script     FilmScriptPro `json:"script"`
}

// loadScriptBundle đọc script.json (bundle mới hoặc FilmScriptPro cũ).
func loadScriptBundle(work string) (FilmScriptBundle, error) {
	var b FilmScriptBundle
	raw, err := os.ReadFile(filepath.Join(work, "script.json"))
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return b, err
	}
	if len(b.Script.Scenes) == 0 {
		var legacy FilmScriptPro
		if jerr := json.Unmarshal(raw, &legacy); jerr == nil && len(legacy.Scenes) > 0 {
			b.Script = legacy
		}
	}
	return b, nil
}

// ScreenplayScene là một cảnh trong kịch bản pha 1: action + thoại hoàn
// chỉnh, chưa có character sheet hay shot list (việc của pha 2).
type ScreenplayScene struct {
	Index      int            `json:"index"`
	Act        int            `json:"act,omitempty"` // hồi 1|2|3 (phim dài)
	Location   string         `json:"location"`
	TimeOfDay  string         `json:"time_of_day"`
	Atmosphere string         `json:"atmosphere"`
	Seconds    int            `json:"seconds"`
	Action     string         `json:"action"` // diễn biến đầy đủ — tiếng Việt
	Dialogue   []DialogueLine `json:"dialogue"`
	Narration  string         `json:"narration"`
}

// Screenplay là kịch bản pha 1 — đầu ra của biên kịch, đầu vào của breakdown.
type Screenplay struct {
	Title   string            `json:"title"`
	Logline string            `json:"logline"`
	Genre   string            `json:"genre,omitempty"`
	Scenes  []ScreenplayScene `json:"scenes"`
}

// BreakdownScene là kết quả breakdown pha 2 cho một cảnh: khóa sản xuất +
// shot list. Action/thoại/lời dẫn KHÔNG nằm ở đây — lấy từ kịch bản pha 1.
type BreakdownScene struct {
	Index       int       `json:"index"`
	Location    string    `json:"location"`    // khóa cứng (có thể chau chuốt từ pha 1)
	TimeOfDay   string    `json:"time_of_day"` // khóa cứng
	Atmosphere  string    `json:"atmosphere"`  // khóa cứng
	Continuity  string    `json:"continuity"`  // khối bất di bất dịch
	Props       []string  `json:"props"`       // đạo cụ quan trọng
	Shots       []ProShot `json:"shots"`
	ImagePrompt string    `json:"image_prompt"` // keyframe đại diện cảnh
}

// Breakdown là đầu ra pha 2: character bible + breakdown từng cảnh.
type Breakdown struct {
	Characters []Character      `json:"characters"`
	Scenes     []BreakdownScene `json:"scenes"`
}

// actDef mô tả một hồi của phim dài: tỉ trọng thời lượng + vai trò kịch bản.
type actDef struct {
	num      int
	share    float64
	role     string
	template string
}

// WriteFilmScript viết kịch bản phim theo Director 3 pha ("truyện → kịch bản
// → breakdown"). aspect ("9:16"|"16:9") tells the director the delivery
// frame so the shot list is composed for the real frame.
//
// Pha 0 luôn là một lần gọi viết truyện. Pha 1: phim ≤180s một lần gọi
// chuyển thể (cấu trúc 3 hồi thu nhỏ); phim dài hơn chuyển thể theo 3 hồi,
// mỗi hồi một lần gọi — vừa ép đúng cấu trúc điện ảnh (hook 60s đầu → bước
// ngoặt giữa → cao trào + kết), vừa không vượt giới hạn độ dài một response
// của LLM. Pha 2 luôn là một lần gọi breakdown trên toàn kịch bản.
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
func breakdownScreenplay(ctx context.Context, llm LLM, sp Screenplay, aspect string) (Breakdown, error) {
	var bd Breakdown
	raw, err := json.Marshal(sp)
	if err != nil {
		return bd, fmt.Errorf("breakdown: %w", err)
	}
	prompt, err := directorPrompt("breakdown.txt", promptData{
		Screenplay: string(raw),
		Orient:     orientationWord(aspect),
		N:          len(sp.Scenes),
	})
	if err != nil {
		return bd, err
	}
	text, err := llm.Complete(ctx,
		"Bạn là script supervisor + production designer điện ảnh. Chỉ trả lời JSON thuần, không giải thích.",
		prompt)
	if err != nil {
		return bd, fmt.Errorf("director pha 2: %w", err)
	}
	if err := parseDirectorJSON(text, &bd); err != nil {
		return bd, err
	}
	return bd, nil
}

// assembleScript ráp kịch bản pha 1 với breakdown pha 2 thành FilmScriptPro
// hoàn chỉnh. Quy tắc "kịch bản trước, breakdown sau":
//   - action / thoại / lời dẫn của pha 1 là CHUẨN (pha 2 không được viết lại);
//   - character bible + khóa bối cảnh + continuity + shot list lấy từ pha 2.
//
// Thiếu breakdown của bất kỳ cảnh nào, hoặc cảnh không có shot → lỗi
// (fail-closed: không quay cảnh không có shot).
func assembleScript(sp Screenplay, bd Breakdown) (FilmScriptPro, error) {
	var script FilmScriptPro
	if len(bd.Characters) == 0 {
		return script, fmt.Errorf("breakdown: thiếu character bible")
	}
	bdMap := make(map[int]BreakdownScene, len(bd.Scenes))
	for _, s := range bd.Scenes {
		bdMap[s.Index] = s
	}
	script.Title = sp.Title
	script.Logline = sp.Logline
	script.Genre = sp.Genre
	script.Characters = bd.Characters
	for _, sps := range sp.Scenes {
		bds, ok := bdMap[sps.Index]
		if !ok {
			return script, fmt.Errorf("breakdown: thiếu cảnh %d", sps.Index)
		}
		shots := sanitizeShots(bds.Shots)
		if len(shots) == 0 {
			return script, fmt.Errorf("breakdown: cảnh %d không có shot", sps.Index)
		}
		script.Scenes = append(script.Scenes, FilmScenePro{
			Index:       sps.Index,
			Act:         sps.Act,
			Location:    firstNonEmpty(bds.Location, sps.Location),
			TimeOfDay:   firstNonEmpty(bds.TimeOfDay, sps.TimeOfDay),
			Atmosphere:  firstNonEmpty(bds.Atmosphere, sps.Atmosphere),
			Seconds:     sps.Seconds,
			Action:      sps.Action,
			Shots:       shots,
			Dialogue:    sps.Dialogue,
			ImagePrompt: bds.ImagePrompt,
			Narration:   sps.Narration,
			Continuity:  bds.Continuity,
			Props:       bds.Props,
		})
	}
	return script, nil
}

// sanitizeShots loại shot rỗng prompt ảnh, kẹp thời lượng shot 2–10s.
func sanitizeShots(shots []ProShot) []ProShot {
	var out []ProShot
	for _, sh := range shots {
		if strings.TrimSpace(sh.ImagePrompt) == "" {
			continue
		}
		if sh.Seconds < 2 {
			sh.Seconds = 4
		} else if sh.Seconds > 10 {
			sh.Seconds = 8
		}
		out = append(out, sh)
	}
	return out
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func countShots(script FilmScriptPro) int {
	n := 0
	for _, sc := range script.Scenes {
		n += len(sc.Shots)
	}
	return n
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
