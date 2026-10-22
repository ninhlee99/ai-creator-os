//go:build parked

package studio

// film_director.go — director phim ngắn/điện ảnh (PIVOT 2026-10-02: đã park).
// Chuyển từ director.go: character bible, director 3 pha (writeStory →
// writeScreenplay → breakdownScreenplay → assembleScript), FilmScriptPro,
// FilmScriptBundle. Chỉ biên dịch với -tags parked. WriteProShotList
// (affiliate) ở lại director.go.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
