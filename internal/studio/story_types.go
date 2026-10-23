package studio

const KindStory = "story"

// StoryParams mô tả 1 job kể chuyện.
type StoryParams struct {
	Topic           string `json:"topic"`
	CustomText      string `json:"custom_text,omitempty"`
	Genre           string `json:"genre"`
	DurationMinutes int    `json:"duration_minutes,omitempty"` // 30, 45, 60 phút
	TargetSeconds   int    `json:"target_seconds,omitempty"`   // Single-pass target for short-form jobs
	AspectRatio     string `json:"aspect_ratio,omitempty"`     // "9:16" or "16:9"
	Words           int    `json:"words"`                      // độ dài truyện mục tiêu (từ)
	Scenes          int    `json:"scenes"`                     // số cảnh mục tiêu (3–24)
	MusicOn         bool   `json:"music_on"`                   // lồng nhạc nền
	MusicPath       string `json:"music_path"`                 // file nhạc local ("" = không nhạc)
	AccountID       int64  `json:"account_id,omitempty"`
	GrowthItemID    int64  `json:"growth_item_id,omitempty"`
	// Voice: giọng đọc ("default", "male", "female"...). Đợt AC: user chọn ở
	// form tạo truyện thay vì hardcode.
	Voice string `json:"voice,omitempty"`
	// Attempt: lần thử thứ mấy (0 = lần đầu). Đợt P: retry tự động tăng
	// dần; đạt story.retry_max thì dừng để không đốt quota vô ích.
	Attempt int `json:"attempt,omitempty"`
	// ReuseFromJobID: job cũ để tái dùng ảnh đã vẽ xong (đợt Y — retry
	// không vẽ lại ảnh từ đầu, tiết kiệm image-gen).
	ReuseFromJobID string `json:"reuse_from_job,omitempty"`
	// AutoClipShorts: Tự động trích xuất các clip 3-5 phút (9:16) cho TikTok/Reels.
	AutoClipShorts bool `json:"auto_clip_shorts,omitempty"`
	// ClipDurationSec: Thời lượng mỗi clip cắt nhỏ (giây, mặc định 180s = 3 phút).
	ClipDurationSec int `json:"clip_duration_sec,omitempty"`
}

// storyScene là 1 cảnh: đoạn văn kể + prompt vẽ ảnh.
type storyScene struct {
	Text        string `json:"text"`
	ImagePrompt string `json:"image_prompt"`
}

// storyDoc là truyện đã viết.
type storyDoc struct {
	Title   string `json:"title"`
	Logline string `json:"logline"`
	Text    string `json:"text"`
}

// AudioStoryPlot cấu trúc 1 cốt truyện audio YouTube trường thiên chuẩn (30 - 60 phút).
type AudioStoryPlot struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Genre           string `json:"genre"`
	GenreLabel      string `json:"genre_label"`
	DurationMinutes int    `json:"duration_minutes"` // 30, 45, 60 phút
	WordsCount      int    `json:"words_count"`      // 4.500 - 9.000 từ
	Hook3s          string `json:"hook_3s"`          // Mở màn giật gân trong 3 giây
	Logline         string `json:"logline"`
	Act1            string `json:"act_1"` // Hồi 1: Khởi nguồn & Bí mật
	Act2            string `json:"act_2"` // Hồi 2: Dấn thân & Xung đột
	Act3            string `json:"act_3"` // Hồi 3: Cao trào kịch tính
	Act4            string `json:"act_4"` // Hồi 4: Bước ngoặt & Cú twist
	Act5            string `json:"act_5"` // Hồi 5: Lời giải & Dư âm sâu sắc
	Message         string `json:"message"`
	SuggestedVoice  string `json:"suggested_voice"`
	Soundscape      string `json:"soundscape"`
}
