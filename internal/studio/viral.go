package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// KindViral là loại job sản xuất video ngắn viral 9:16 (TikTok, Reels, Shorts) 100% gốc.
const KindViral = "viral"

// ViralParams chứa cấu hình đầu vào để tạo 1 video viral.
type ViralParams struct {
	Topic          string   `json:"topic"`
	Genre          string   `json:"genre"`       // mystery, lifehack, trendjack, tech_fact, koc_review
	VisualMood     string   `json:"visual_mood"` // cinematic, vibrant_3d, realistic, cyber_neon
	Voice          string   `json:"voice"`       // TTS provider voice identifier
	Account        string   `json:"account"`     // username kênh để đọc Brand DNA
	AccountID      int64    `json:"account_id,omitempty"`
	ModelPhoto     string   `json:"model_photo,omitempty"`  // local file path to character face reference image
	ModelPhotos    []string `json:"model_photos,omitempty"` // multi-angle face reference images for character locking
	TargetDuration int      `json:"target_duration"`        // minimum duration; raised to source duration when longer
	MusicURL       string   `json:"music_url"`              // Nhạc nền trending (tùy chọn)
	MusicFile      string   `json:"music_file"`
	SourceVideoURL string   `json:"source_video_url,omitempty"`
	SampleVideo    string   `json:"sample_video,omitempty"` // local file path to sample motion video
	AIMode         string   `json:"ai_mode,omitempty"`      // local or an explicitly selected video API such as veo3
}

// ViralScene mô tả 1 phân cảnh trong kịch bản video viral.
type ViralScene struct {
	Index       int     `json:"index"`
	Phase       string  `json:"phase"`        // hook, problem, insight, solution, cta
	VoiceText   string  `json:"voice_text"`   // Lời thoại lôi cuốn (tiếng Việt)
	ImagePrompt string  `json:"image_prompt"` // Prompt tiếng Anh cho AI MediaGen vẽ ảnh 9:16
	VisualHook  string  `json:"visual_hook"`  // Text giật tít hiển thị trên màn hình
	DurationSec float64 `json:"duration_sec"`
}

// ViralScript là kịch bản hoàn chỉnh của một video viral.
type ViralScript struct {
	Title        string       `json:"title"`
	Hook3s       string       `json:"hook_3s"`
	CoreAngle    string       `json:"core_angle"`
	Hashtags     []string     `json:"hashtags"`
	CallToAction string       `json:"call_to_action"`
	Scenes       []ViralScene `json:"scenes"`
}

// EnqueueViral đưa một tác vụ tạo video viral vào hàng đợi render của Studio.
func (s *Studio) EnqueueViral(ctx context.Context, p ViralParams) (string, error) {
	if strings.TrimSpace(p.Topic) == "" {
		return "", fmt.Errorf("chủ đề video không được để trống")
	}
	if p.Genre == "" {
		p.Genre = "trendjack"
	}
	if p.VisualMood == "" {
		p.VisualMood = "cinematic"
	}
	if p.TargetDuration < 60 {
		p.TargetDuration = 60 // Video viral tối thiểu từ 60s trở lên (chuẩn TikTok Creativity Program)
	}
	if strings.TrimSpace(p.AIMode) == "" {
		p.AIMode = "local"
	}
	if fileExists(p.SampleVideo) {
		if sourceDuration := probeDuration(ctx, p.SampleVideo); sourceDuration > 0 {
			if sourceTarget := int(math.Ceil(sourceDuration)); sourceTarget > p.TargetDuration {
				p.TargetDuration = sourceTarget
			}
		}
	}

	// Đảm bảo video của ai, kênh nào phải đúng kênh đó (không lấy C-Biz bỏ sang kênh VN hay ngược lại)
	topicLower := strings.ToLower(p.Topic)
	isCBiz := strings.Contains(topicLower, "c-biz") || strings.Contains(topicLower, "cbiz") ||
		strings.Contains(topicLower, "trung quốc") || strings.Contains(topicLower, "hoa ngữ") ||
		strings.Contains(topicLower, "douyin") || strings.Contains(topicLower, "weibo")
	if isCBiz && (p.Account == "kechuyen_ai" || p.Account == "") {
		p.Account = "douyin_reup"
	} else if !isCBiz && p.Account == "douyin_reup" {
		p.Account = "kechuyen_ai"
	}

	title := "Video Viral: " + p.Topic
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:77]) + "..."
	}

	jobID, err := s.insertJob(KindViral, title, p)
	if err != nil {
		return "", err
	}

	go s.runViral(jobID, p)
	return jobID, nil
}

// ListViralJobs trả về danh sách các job viral đã và đang sản xuất.
func (s *Studio) ListViralJobs(limit int) []Job {
	if limit <= 0 {
		limit = 30
	}
	jobs := s.ListJobs(limit)
	var out []Job
	for _, j := range jobs {
		if j.Kind == KindViral {
			out = append(out, j)
		}
	}
	return out
}

func defaultViralScript(p ViralParams) *ViralScript {
	if p.Genre == "dance" {
		return &ViralScript{
			Title:        p.Topic,
			Hook3s:       "Trending Dance Challenge TikTok",
			CoreAngle:    "Vũ đạo sexy cuốn hút",
			Hashtags:     []string{"#dance", "#trend", "#tiktokdance", "#xuhuong"},
			CallToAction: "Thả tim và follow để xem thêm vũ đạo mới!",
			Scenes: []ViralScene{
				{
					Index:       1,
					Phase:       "intro",
					VoiceText:   "",
					ImagePrompt: "A stunning beautiful Vietnamese female dancer striking an alluring opening pose in a modern aesthetic studio, warm dramatic cinematic lighting, 9:16 vertical, photorealistic, no text",
					VisualHook:  "HOT TREND DANCE",
					DurationSec: 15.0,
				},
				{
					Index:       2,
					Phase:       "dance",
					VoiceText:   "",
					ImagePrompt: "A stunning beautiful Vietnamese female dancer performing an energetic sexy dance choreography, fluid motion blur, glowing neon accents, 9:16 vertical, photorealistic, no text",
					VisualHook:  "VIRAL CHOREOGRAPHY",
					DurationSec: 15.0,
				},
				{
					Index:       3,
					Phase:       "rhythm",
					VoiceText:   "",
					ImagePrompt: "A stunning beautiful Vietnamese female dancer in the middle of a captivating dance motion, stylish trendy outfit, expressive dynamic movement, cinematic vertical 9:16, no text",
					VisualHook:  "RHYTHM DROP",
					DurationSec: 15.0,
				},
				{
					Index:       4,
					Phase:       "ending",
					VoiceText:   "",
					ImagePrompt: "A stunning beautiful Vietnamese female dancer striking an elegant closing dance pose with a radiant confident smile to camera, aesthetic stage background, 9:16 vertical, no text",
					VisualHook:  "FOLLOW FOR MORE",
					DurationSec: 15.0,
				},
			},
		}
	}
	// Rotate between 4 diverse fallback scripts for variety (5-Star Retention Architecture)
	variant := int(time.Now().UnixNano() % 4)
	switch variant {
	case 0:
		// Question & Curiosity Gap Hook Style
		return &ViralScript{
			Title:        "Bí Mật Triệu View: " + p.Topic,
			Hook3s:       "Dừng lại 3 giây! Bạn có dám tin rằng " + p.Topic + " đang âm thầm thay đổi mọi thứ?",
			CoreAngle:    "Góc nhìn tâm lý học hành vi và đòn bẩy đột phá",
			Hashtags:     []string{"#viral", "#xuhuong", "#shorts", "#kienthuc", "#bimat"},
			CallToAction: "Lưu lại video này kẻo trôi mất và follow kênh để nhận thêm bí mật mỗi ngày!",
			Scenes: []ViralScene{
				{Index: 1, Phase: "hook", VoiceText: "Dừng lại 3 giây! Bạn có dám tin rằng " + p.Topic + " đang âm thầm thay đổi cuộc sống của bạn mà bạn không hề hay biết?", ImagePrompt: "Cinematic close-up portrait of a person reacting with intense curiosity and amazement, dramatic rim lighting, moody dark background with warm accent glow, shot on 35mm lens, vertical 9:16, photorealistic, authentic skin texture", VisualHook: "DỪNG LẠI 3 GIÂY!", DurationSec: 3.5},
				{Index: 2, Phase: "setup", VoiceText: "Hãy nghe mình nói điều này... 90% mọi người ngoài kia đang hiểu sai hoàn toàn về bản chất của nó.", ImagePrompt: "Medium shot of an aesthetic modern workspace, floating holographic data nodes glowing softly, cinematic depth of field, warm golden ambient lights, vertical 9:16, 8k resolution", VisualHook: "90% ĐANG HIỂU SAI", DurationSec: 4.0},
				{Index: 3, Phase: "problem", VoiceText: "Sai lầm chí mạng là họ chỉ nhìn vào bề nổi — mà bỏ qua quy luật vận hành ngầm bên dưới.", ImagePrompt: "Moody atmospheric shot of an iceberg metaphor or intricate clockwork mechanism turning in dramatic shadows, volumetric light beams, cinematic 9:16", VisualHook: "SAI LẦM CHÍ MẠNG", DurationSec: 4.0},
				{Index: 4, Phase: "insight", VoiceText: "Nhưng khi bạn đổi góc nhìn sang hướng này... mọi nút thắt sẽ tự động được gỡ bỏ hoàn toàn.", ImagePrompt: "Extreme close-up of a sharp focused eye reflecting a golden revelation spark, macro photography, pristine iris details, dramatic chiaroscuro, vertical 9:16", VisualHook: "BƯỚC NGOẶT ĐỘT PHÁ!", DurationSec: 4.5},
				{Index: 5, Phase: "transformation", VoiceText: "Những người nắm bắt được chìa khóa này đã bứt phá ngoạn mục chỉ sau một thời gian rất ngắn.", ImagePrompt: "Wide cinematic low-angle hero shot of a confident figure standing at the precipice of a modern skyscraper at twilight, city lights bokeh, inspirational golden hour, vertical 9:16", VisualHook: "BỨT PHÁ NGOẠN MỤC", DurationSec: 4.0},
				{Index: 6, Phase: "cta", VoiceText: "Lưu ngay video này trước khi bị trôi và follow kênh — ngày mai mình sẽ tiết lộ phần tiếp theo!", ImagePrompt: "Modern aesthetic graphic of a hand tapping a luminous bookmark button, soft pastel particle bursts, clean luxury studio background, vertical 9:16", VisualHook: "LƯU LẠI KẺO MẤT!", DurationSec: 3.5},
			},
		}
	case 1:
		// Shocking Statistic Hook Style
		return &ViralScript{
			Title:        "97% Người Hiểu Sai Về " + p.Topic,
			Hook3s:       "97% người ngoài kia đang làm sai điều này về " + p.Topic + "!",
			CoreAngle:    "Dữ liệu phản trực giác phá vỡ tư duy cũ",
			Hashtags:     []string{"#viral", "#facts", "#trending", "#suthat", "#tiktok"},
			CallToAction: "Chia sẻ cho người thân cùng biết và follow kênh để không bỏ lỡ phần 2!",
			Scenes: []ViralScene{
				{Index: 1, Phase: "hook", VoiceText: "97% người ngoài kia đang làm sai điều này về " + p.Topic + "... và cái giá phải trả cực kỳ đắt!", ImagePrompt: "Extreme close-up expressive shot of eyes widening in dramatic realization, sharp cinematic lighting, high contrast shadows, shot on 85mm portrait lens, vertical 9:16, photorealistic", VisualHook: "97% LÀM SAI ĐIỀU NÀY!", DurationSec: 3.2},
				{Index: 2, Phase: "setup", VoiceText: "Một nghiên cứu thực tế vừa công bố số liệu khiến cả giới chuyên môn phải giật mình.", ImagePrompt: "Cinematic wide shot of a futuristic research library with glowing data panels, deep blue and amber dual lighting, vertical 9:16, architectural depth", VisualHook: "SỐ LIỆU GÂY SỐC", DurationSec: 4.0},
				{Index: 3, Phase: "problem", VoiceText: "Vấn đề là chúng ta vẫn đang áp dụng những thói quen cũ kỹ từ 10 năm trước — mà không hề hay biết.", ImagePrompt: "Overhead top-down geometric shot of a complex maze with solitary figure finding a way out, dramatic moody lighting, vertical 9:16", VisualHook: "CÁCH CŨ = BẾ TẮC", DurationSec: 4.0},
				{Index: 4, Phase: "insight", VoiceText: "Chỉ cần thay đổi đúng một chi tiết cốt lõi này, hiệu quả sẽ nhảy vọt gấp nhiều lần.", ImagePrompt: "Macro close-up shot of a golden key turning inside an intricate mechanical lock, volumetric light rays beaming through, cinematic vertical 9:16", VisualHook: "MỘT THAY ĐỔI CỐT LÕI", DurationSec: 4.5},
				{Index: 5, Phase: "transformation", VoiceText: "Và đây là kết quả không thể chối cãi khi bạn làm chủ được phương pháp này.", ImagePrompt: "Split comparison visual showing dramatic before-and-after transformation, cinematic lighting balance, modern aesthetic, vertical 9:16", VisualHook: "KẾT QUẢ ĐỈNH CAO", DurationSec: 4.0},
				{Index: 6, Phase: "cta", VoiceText: "Đừng quên lưu lại và bấm follow — phần tiếp theo sẽ giải thích chi tiết từng bước!", ImagePrompt: "Clean aesthetic minimalist composition of a phone screen glowing with a red notification heart and follow button, subtle neon lighting, vertical 9:16", VisualHook: "FOLLOW XEM PHẦN 2!", DurationSec: 3.5},
			},
		}
	case 2:
		// High-Stakes Challenge Hook Style
		return &ViralScript{
			Title:        "Thử Thách: Làm Chủ " + p.Topic + " Trong 30 Giây",
			Hook3s:       "Thử thách bản thân: Bạn có thể hiểu trọn vẹn " + p.Topic + " chỉ trong 30 giây?",
			CoreAngle:    "Đúc kết tinh hoa tốc độ cao dành cho người bận rộn",
			Hashtags:     []string{"#challenge", "#viral", "#30giay", "#trending", "#hocnhanh"},
			CallToAction: "Bạn có làm được không? Comment ngay bên dưới và bấm follow nhé!",
			Scenes: []ViralScene{
				{Index: 1, Phase: "hook", VoiceText: "Thử thách cho bạn: Hiểu trọn vẹn " + p.Topic + " chỉ trong đúng 30 giây. Bắt đầu đếm ngược!", ImagePrompt: "Dynamic close-up shot of a glowing analog stopwatch timer freezing at zero, sparks and volumetric smoke, high adrenaline cinematic lighting, vertical 9:16", VisualHook: "THỬ THÁCH 30 GIÂY!", DurationSec: 3.0},
				{Index: 2, Phase: "setup", VoiceText: "Bước 1: Hãy gạt bỏ ngay lập tức những định kiến rườm rà không cần thiết.", ImagePrompt: "Cinematic shot of hands sweeping away dusty vintage blueprints to reveal a pristine glowing tablet, soft golden window light, vertical 9:16", VisualHook: "BƯỚC 1: BỎ RƯỜM RÀ", DurationSec: 3.8},
				{Index: 3, Phase: "problem", VoiceText: "Cái bẫy lớn nhất là bị ngợp trong ma trận thông tin mà không có lộ trình rõ ràng.", ImagePrompt: "Dramatic shot of a labyrinth made of towering glowing bookshelves, dark moody atmospheric fog, vertical 9:16 cinematic", VisualHook: "BẪY QUÁ TẢI", DurationSec: 4.0},
				{Index: 4, Phase: "insight", VoiceText: "Quy tắc 80/20 ở đây rất đơn giản: 20% hành động đúng này sẽ mang lại 80% kết quả.", ImagePrompt: "Artistic conceptual shot of a shining golden ratio spiral illuminating the darkness, elegant aesthetic, vertical 9:16", VisualHook: "QUY TẮC 80/20", DurationSec: 4.5},
				{Index: 5, Phase: "transformation", VoiceText: "Bây giờ bạn đã nắm trong tay công thức then chốt — hãy áp dụng ngay hôm nay.", ImagePrompt: "Hero perspective shot of a determined person walking forward into a bright futuristic doorway, confident stride, lens flare, vertical 9:16", VisualHook: "ÁP DỤNG NGAY!", DurationSec: 4.0},
				{Index: 6, Phase: "cta", VoiceText: "Bạn đã vượt qua thử thách chưa? Hãy comment góc nhìn của bạn và follow kênh ngay!", ImagePrompt: "Close-up cinematic shot of an elegant trophy reflecting warm stage lights, sparkling confetti, vertical 9:16", VisualHook: "COMMENT GÓC NHÌN!", DurationSec: 3.5},
			},
		}
	default:
		// Industry Revelation & Insider Secret Style
		return &ViralScript{
			Title:        "Sự Thật Không Ai Nói Về " + p.Topic,
			Hook3s:       "Đây là điều mà những người thành công về " + p.Topic + " không bao giờ muốn bạn biết!",
			CoreAngle:    "Bóc tách sự thật ngầm và chiến lược thực chiến",
			Hashtags:     []string{"#bimat", "#viral", "#suthat", "#chuyendoi", "#xuhuong"},
			CallToAction: "Follow kênh ngay hôm nay để trở thành người đi trước đón đầu!",
			Scenes: []ViralScene{
				{Index: 1, Phase: "hook", VoiceText: "Đây là điều mà những người đứng đầu về " + p.Topic + " không bao giờ muốn bạn biết!", ImagePrompt: "Cinematic close-up of a silhouette whispering in shadows, warm tungsten backlight cutting through dark room, intense mystery mood, 35mm lens, vertical 9:16", VisualHook: "ĐIỀU HỌ GIẤU BẠN!", DurationSec: 3.5},
				{Index: 2, Phase: "setup", VoiceText: "Sau nhiều năm quan sát và đúc kết, mình nhận ra một mô thức chung mà ai cũng che giấu.", ImagePrompt: "Wide atmospheric shot of a master strategist desk with maps, compass, and chess board, warm desk lamp, film noir style, vertical 9:16", VisualHook: "MÔ THỨC BÍ MẬT", DurationSec: 4.0},
				{Index: 3, Phase: "problem", VoiceText: "Họ luôn kể cho bạn nghe phần kết quả rực rỡ, nhưng cố tình giấu đi cái giá phải trả phía sau.", ImagePrompt: "Dramatic shot of a fractured mirror reflecting two opposing emotional expressions, chiaroscuro lighting, deep shadows, vertical 9:16", VisualHook: "SỰ THẬT PHÍA SAU", DurationSec: 4.0},
				{Index: 4, Phase: "insight", VoiceText: "Bí mật thực sự nằm ở việc kiểm soát biến số này trước khi quá muộn.", ImagePrompt: "Close-up shot of hands deftly manipulating an antique brass astrolabe pointing at a glowing celestial star, cinematic vertical 9:16", VisualHook: "BIẾN SỐ THEN CHỐT", DurationSec: 4.5},
				{Index: 5, Phase: "transformation", VoiceText: "Một khi thấu suốt được chân lý này, bạn sẽ làm chủ hoàn toàn cuộc chơi.", ImagePrompt: "Inspiring medium shot of a person smiling with serene confidence, overlooking sunrise horizon from an observation deck, golden hour light, vertical 9:16", VisualHook: "LÀM CHỦ CUỘC CHƠI", DurationSec: 4.0},
				{Index: 6, Phase: "cta", VoiceText: "Follow kênh ngay hôm nay để đón xem những phân tích chuyên sâu tiếp theo!", ImagePrompt: "Clean aesthetic shot of a smartphone held in hand with animated follow notification and sleek ambient lighting, vertical 9:16", VisualHook: "FOLLOW NGAY HÔM NAY!", DurationSec: 3.5},
			},
		}
	}
}

// GenerateViralScript uses LLM or curated high-retention fallback templates.
func (s *Studio) GenerateViralScript(ctx context.Context, p ViralParams) (*ViralScript, error) {
	if s.llm == nil || !s.llm.Healthy(ctx) || p.Genre == "dance" {
		return defaultViralScript(p), nil
	}

	prompt := fmt.Sprintf(`Bạn là Đạo diễn kiêm Nhà thiết kế kịch bản Viral ngắn (TikTok/Reels/Shorts 9:16) đẳng cấp hàng đầu thế giới.
Nhiệm vụ: Tạo kịch bản video ngắn giữ chân người xem 100%% trong 3 giây đầu (Stop-the-Scroll) và duy trì tỷ lệ xem hết (Completion Rate) trên 70%%.

CHỦ ĐỀ: "%s"
THỂ LOẠI: %s
MOOD HÌNH ẢNH: %s
KÊNH ĐẠI DIỆN: %s

── CÔNG THỨC GIỮ CHÂN 5 SAO BẮT BUỘC (RETENTION FORMULA) ──
1. HOOK 3 GIÂY ĐẦU (Scene 1):
   - Mở đầu bằng một câu hỏi phản trực giác, một con số giật mình hoặc câu khẳng định phá vỡ niềm tin cũ.
   - VoiceText: Ngắn, đanh, năng lượng cao, từ ngữ gây kích thích trí tò mò cực độ (Curiosity Gap).
   - VisualHook: Chữ in hoa to, giật tít 2-4 từ nổi bật nhất.
2. VĂN PHONG VOICE-OVER:
   - Viết bằng tiếng Việt văn nói tự nhiên, gần gũi, dứt khoát theo nhịp thở (dùng dấu chấm, phẩy, ba chấm... để tạo nhịp điệu).
   - Tuyệt đối KHÔNG hành văn như dịch sách hay văn xuôi khô khan.
3. CHỈ ĐẠO HÌNH ẢNH (image_prompt):
   - Viết bằng tiếng Anh chi tiết cho AI Image Generation (MediaGen / Imagen 3).
   - BẮT BUỘC có chi tiết máy quay & ánh sáng: "Cinematic vertical 9:16 portrait, shot on 35mm lens, f/1.8 shallow depth of field, dramatic cinematic lighting (golden hour / rim light / volumetric beams), authentic natural skin texture, realistic human eyes, masterclass composition, photorealistic 8k, no text, no watermark".
4. THỜI LƯỢNG: Tối thiểu %d giây. Tạo đủ cảnh để tổng thời lượng phủ hết mức này, thường 4-6 giây/cảnh; không kéo dài một ảnh để bù thời lượng.

Yêu cầu trả về DUY NHẤT một chuỗi JSON thuần, đúng định dạng sau:
{
  "title": "Tiêu đề video ngắn giật gân, cuốn hút",
  "hook_3s": "Câu mở đầu gây tò mò cực độ trong 3 giây đầu",
  "core_angle": "Góc nhìn độc đáo hoặc giá trị cốt lõi mang lại",
  "hashtags": ["#tag1", "#tag2", "#tag3", "#xuhuong", "#viral"],
  "call_to_action": "Kêu gọi lưu video hoặc follow tự nhiên",
  "scenes": [
    {
      "index": 1,
      "phase": "hook",
      "voice_text": "Lời thoại ngắn gọn, súc tích (3-4 giây)",
      "image_prompt": "Chi tiết prompt tiếng Anh 9:16 cinematic photorealistic",
      "visual_hook": "CHỮ TO GIẬT TÍT 3S ĐẦU",
      "duration_sec": 3.5
    }
  ]
}`, p.Topic, p.Genre, p.VisualMood, p.Account, p.TargetDuration)

	if p.ModelPhoto != "" {
		prompt += "\n- KHÓA KHUÔN MẶT NHÂN VẬT: Video này có nhân vật đại diện. Hãy miêu tả nhân vật chính xuất hiện rõ nét."
	}
	if s.PromptEnhancer != nil {
		if enhancement := s.PromptEnhancer(ctx, p.Genre, p.Account); enhancement != "" {
			prompt += "\n" + enhancement
		}
	}

	resp, err := s.llm.Complete(ctx, "Bạn là Viral Content Architect. Trả về duy nhất JSON hợp lệ, không bọc markdown.", prompt)
	if err != nil {
		return defaultViralScript(p), nil
	}

	var script ViralScript
	cleaned := extractJSON(resp)
	if err := json.Unmarshal([]byte(cleaned), &script); err != nil {
		if err2 := json.Unmarshal([]byte(resp), &script); err2 != nil {
			return defaultViralScript(p), nil
		}
	}

	if len(script.Scenes) == 0 {
		return defaultViralScript(p), nil
	}

	return &script, nil
}

// runViral là goroutine chạy nền sản xuất video viral hoàn chỉnh 100% gốc.
func (s *Studio) runViral(jobID string, p ViralParams) {
	defer func() {
		if r := recover(); r != nil {
			s.setStatus(jobID, StatusFailed, 100)
			s.appendLog(jobID, fmt.Sprintf("LỖI panic (%v) — job dừng an toàn.", r))
		}
	}()

	localMode := strings.EqualFold(strings.TrimSpace(p.AIMode), "local") || strings.TrimSpace(p.AIMode) == ""
	requiredDuration := p.TargetDuration
	if requiredDuration < 60 {
		requiredDuration = 60
	}
	p.TargetDuration = requiredDuration
	renderTimeout := 20 * time.Minute
	if requiredDuration > 0 {
		const maxRenderTimeout = 24 * time.Hour
		maxDurationSeconds := int64((maxRenderTimeout - 20*time.Minute) / (3 * time.Second))
		if int64(requiredDuration) >= maxDurationSeconds {
			renderTimeout = maxRenderTimeout
		} else {
			durationBudget := time.Duration(requiredDuration)*3*time.Second + 20*time.Minute
			if durationBudget > renderTimeout {
				renderTimeout = durationBudget
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	s.mu.Lock()
	s.running[jobID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, jobID)
		s.mu.Unlock()
	}()

	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(jobID, StatusFailed, 100)
		s.appendLog(jobID, "LỖI: job bị hủy khi đang chờ hàng đợi render")
		return
	}

	s.setStatus(jobID, StatusRunning, 5)
	s.appendLog(jobID, "Khởi động xưởng dựng video viral...")

	fail := func(err error) {
		s.appendLog(jobID, "LỖI: "+err.Error())
		s.setStatus(jobID, StatusFailed, 100)
	}

	var script *ViralScript
	if localMode {
		script = defaultViralScript(p)
		s.appendLog(jobID, "Chế độ cục bộ: không gọi API LLM, tạo ảnh hoặc video.")
	} else {
		var err error
		script, err = s.GenerateViralScript(ctx, p)
		if err != nil {
			fail(fmt.Errorf("soạn kịch bản thất bại: %w", err))
			return
		}
	}

	title := script.Title
	if title != "" {
		s.db.Exec(`UPDATE studio_jobs SET title=? WHERE id=?`, "Video Viral: "+title, jobID)
	}
	if localMode {
		s.appendLog(jobID, fmt.Sprintf("Kịch bản '%s' sẵn sàng (%d cảnh); dựng từ ảnh mẫu đã tải lên.", script.Title, len(script.Scenes)))
	} else {
		s.appendLog(jobID, fmt.Sprintf("Kịch bản '%s' sẵn sàng (%d cảnh). Đang sinh hình ảnh 9:16...", script.Title, len(script.Scenes)))
	}
	s.setStatus(jobID, StatusRunning, 20)

	jobDir := filepath.Join(s.workRoot, jobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		fail(fmt.Errorf("tạo thư mục làm việc thất bại: %w", err))
		return
	}

	// 1. Chuẩn bị ảnh nhân vật (Character Face Lock) nếu có
	var refs []ImageRef
	modelPhoto := p.ModelPhoto
	if modelPhoto == "" || !fileExists(modelPhoto) {
		mps := s.queryAccountModelPhotos(p.AccountID)
		if len(mps) > 0 {
			modelPhoto = mps[0]
		}
	}
	anchor := NewVisualAnchor(p.Topic, p.Genre, "", modelPhoto)
	if modelPhoto != "" && fileExists(modelPhoto) {
		refs = append(refs, ImageRef{Path: modelPhoto})
		s.appendLog(jobID, fmt.Sprintf("Khóa nhận diện nhân vật (Character Face Lock): sử dụng ảnh mẫu %s", filepath.Base(modelPhoto)))
	} else {
		s.appendLog(jobID, fmt.Sprintf("Kích hoạt Visual Anchor Matrix: Khóa đặc điểm nhân vật (%s, %s, %s) và bối cảnh (%s) xuyên suốt các phân cảnh", anchor.ActorName, anchor.FaceTraits, anchor.Wardrobe, anchor.Environment))
	}

	var silentVideo string
	var preparedMotion bool
	var photoPaths []string

	isVideoFile := func(path string) bool {
		if path == "" || !fileExists(path) {
			return false
		}
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".mp4" || ext == ".mov" || ext == ".webm" || ext == ".mkv" || ext == ".avi"
	}

	// Read reference duration; output uses newly generated visuals and audio.
	if isVideoFile(p.SampleVideo) {
		profile := AnalyzeViralOriginal(ctx, p.SampleVideo, "")
		if profile.Duration > 0 {
			s.appendLog(jobID, fmt.Sprintf("✓ Video tham chiếu: %.1fs. Dựng nội dung mới đủ thời lượng, không dùng lại footage/audio nguồn.", profile.Duration))
		}
	}
	if p.SourceVideoURL != "" {
		s.appendLog(jobID, fmt.Sprintf("✓ Source reference: %s. Create original visuals; source footage and audio are not reused.", p.SourceVideoURL))
	}
	if localMode {
		modelPhotos := p.ModelPhotos
		if len(modelPhotos) == 0 {
			modelPhotos = s.uploadedModelPhotos(p.AccountID)
		}
		for _, path := range modelPhotos {
			if !fileExists(path) {
				continue
			}
			crops := 2
			if len(modelPhotos) == 1 {
				crops = 4
			}
			variants := derivePhotoVariants(ctx, path, jobDir, fmt.Sprintf("viral-model-%d", len(photoPaths)), crops)
			if len(variants) == 0 {
				variants = []string{path}
			}
			for _, variant := range variants {
				if len(photoPaths) >= 8 {
					break
				}
				photoPaths = append(photoPaths, variant)
			}
			if len(photoPaths) >= 8 {
				break
			}
		}
		if len(photoPaths) == 0 && p.ModelPhoto != "" && fileExists(p.ModelPhoto) {
			photoPaths = derivePhotoVariants(ctx, p.ModelPhoto, jobDir, "viral-model-upload", 4)
			if len(photoPaths) == 0 {
				photoPaths = []string{p.ModelPhoto}
			}
		}
		if len(photoPaths) == 0 {
			fail(fmt.Errorf("chế độ cục bộ cần ảnh mẫu đã tải lên; thêm ảnh ở trang /viral rồi thử lại"))
			return
		}
		for i, path := range photoPaths {
			aid := s.addAsset(jobID, i+1, "photo", fmt.Sprintf("Ảnh mẫu đã tải lên %d", i+1))
			s.setAsset(aid, StatusDone, path)
		}
		s.appendLog(jobID, fmt.Sprintf("Dùng %d khung từ ảnh mẫu đã tải lên; render cục bộ đủ %.0fs.", len(photoPaths), float64(requiredDuration)))
	}

	// 0B. Nếu không có video vũ đạo sẵn có nhưng có người mẫu và là chủ đề dance:
	// Tự động dựng video CapCut Beat Bounce "giật giật" theo beat nhạc TikTok 9:16 với người mẫu!
	if !preparedMotion && isDanceOrPerformance(p.Topic, p.Genre) && modelPhoto != "" && fileExists(modelPhoto) {
		dancePhotos := p.ModelPhotos
		if len(dancePhotos) == 0 {
			dancePhotos = []string{modelPhoto}
		}
		for len(dancePhotos) < 6 {
			dancePhotos = append(dancePhotos, dancePhotos...)
		}
		if len(dancePhotos) > 8 {
			dancePhotos = dancePhotos[:8]
		}
		danceMusic := p.MusicFile
		if danceMusic == "" || !fileExists(danceMusic) {
			danceMusic = FindTrendingMusic("nhay")
		}
		if danceMusic == "" || !fileExists(danceMusic) {
			danceMusic = FindDefaultMusic()
		}

		danceOut := filepath.Join(jobDir, "dance_beatbounce_916.mp4")
		s.appendLog(jobID, "Khởi tạo video Sexy Dance Beat Bounce 9:16 (CapCut giật giật theo nhịp nhạc trending TikTok)...")
		if err := AssembleBeatBounce(ctx, dancePhotos, 2.5, 126, danceMusic, 0.0, danceOut, "9:16"); err == nil && fileExists(danceOut) {
			silentVideo = danceOut
			preparedMotion = true
			s.appendLog(jobID, "✓ Hoàn thành dựng video Sexy Dance Beat Bounce 9:16 chuẩn TikTok!")
		}
	}

	var motionClips []string
	var wavPaths []string
	var secsPerPhoto float64 = 4.0

	if !preparedMotion {
		if !localMode {
			for i, sc := range script.Scenes {
				s.setStatus(jobID, StatusRunning, 20+int(float64(i)/float64(len(script.Scenes))*35))
				s.appendLog(jobID, fmt.Sprintf("Xử lý cảnh %d/%d (%s)...", i+1, len(script.Scenes), sc.Phase))

				aid := s.addAsset(jobID, i+1, "photo", sc.ImagePrompt)
				s.setAsset(aid, StatusRunning, "")

				outImg := filepath.Join(jobDir, fmt.Sprintf("scene_%02d.png", i+1))
				imgPrompt := anchor.EnrichScenePrompt(sc.ImagePrompt, "cinematic smooth tracking", i+1, len(script.Scenes))
				if len(refs) > 0 {
					imgPrompt += ", featuring the consistent character from the reference image, exact same face, hair, and appearance"
				}

				if s.mg != nil && s.mg.Healthy(ctx) {
					s.appendLog(jobID, fmt.Sprintf("Tạo keyframe AI cảnh %d/%d (khóa đặc tính nhân vật)...", i+1, len(script.Scenes)))
					if err := s.mg.GenerateImage(ctx, imgPrompt, refs, outImg); err != nil {
						s.appendLog(jobID, fmt.Sprintf("Cảnh %d: lỗi tạo ảnh MediaGen (%v), tạo ảnh mẫu dự phòng", i+1, err))
						_ = os.Remove(outImg)
					}
				}

				st, stErr := os.Stat(outImg)
				if stErr != nil || st.Size() < 100 {
					if modelPhoto != "" && fileExists(modelPhoto) {
						_ = copyViralFile(modelPhoto, outImg)
					} else {
						_ = GenerateCinematicStoryScene(i, len(script.Scenes), sc.ImagePrompt, outImg)
					}
				}

				if fileExists(outImg) {
					photoPaths = append(photoPaths, outImg)
					// Khóa ảnh đầu tiên làm Master Anchor Photo cho các cảnh tiếp theo nếu chưa có ảnh mẫu
					if anchor.MasterPhotoPath == "" {
						anchor.MasterPhotoPath = outImg
						refs = append(refs, ImageRef{Path: outImg})
						s.appendLog(jobID, fmt.Sprintf("✓ Đã khóa khung hình Cảnh 1 làm mốc nhân vật chuẩn (Master Keyframe) cho các cảnh sau"))
					}
				}

				// KÍCH HOẠT GENERATIVE AI VIDEO (Google Flow / Veo 3 / Muse AI): Sinh video chuyển động thật cho từng phân cảnh
				clipSec := int(sc.DurationSec)
				if clipSec <= 0 {
					clipSec = 5
				}
				sceneVideo := filepath.Join(jobDir, fmt.Sprintf("motion_%02d.mp4", i+1))
				fallbackUsed := false
				vErr := s.GenerateMotionVideoClip(ctx, MotionVideoOptions{
					Prompt:       imgPrompt,
					FirstFrame:   outImg,
					Seconds:      clipSec,
					AspectRatio:  "9:16",
					Engine:       p.AIMode,
					Anchor:       anchor,
					AutoFaceLock: true,
					SceneIndex:   i,
					FallbackUsed: &fallbackUsed,
					OutputPath:   sceneVideo,
					LogFn:        func(msg string) { s.appendLog(jobID, msg) },
				})

				if fileExists(outImg) {
					s.setAsset(aid, StatusDone, outImg)
				}
				if vErr == nil && fileExists(sceneVideo) && !fallbackUsed {
					motionClips = append(motionClips, sceneVideo)
					clipAid := s.addAsset(jobID, 100+i+1, "clip", fmt.Sprintf("Cảnh chuyển động AI %d (%ds)", i+1, clipSec))
					s.setAsset(clipAid, StatusDone, sceneVideo)
					s.appendLog(jobID, fmt.Sprintf("✓ Cảnh %d/%d: Sinh video chuyển động AI thành công (%ds, 9:16)!", i+1, len(script.Scenes), clipSec))
				} else if vErr == nil && fallbackUsed {
					s.appendLog(jobID, fmt.Sprintf("Cảnh %d/%d: engine chưa tạo clip; dùng ảnh tĩnh làm video pan/zoom.", i+1, len(script.Scenes)))
				} else if !fileExists(outImg) {
					s.setAsset(aid, StatusFailed, "")
				}

				// Tạo âm thanh giọng đọc nếu narrator có sẵn (và KHÔNG phải video vũ đạo/nhảy)
				if !localMode && !isDanceOrPerformance(p.Topic, p.Genre) && s.narrator != nil && sc.VoiceText != "" {
					wavFile := filepath.Join(jobDir, fmt.Sprintf("scene_%02d.wav", i+1))
					wavBytes, err := s.narrator(ctx, sc.VoiceText, p.Voice)
					if err == nil && len(wavBytes) > 0 {
						if err := os.WriteFile(wavFile, wavBytes, 0o644); err == nil {
							wavPaths = append(wavPaths, wavFile)
						}
					}
				}
			}
		}

		s.setStatus(jobID, StatusRunning, 65)
		silentVideo = filepath.Join(jobDir, "silent_916.mp4")

		targetSec := float64(requiredDuration)

		// Nếu đã sinh được các video clip chuyển động AI (Google Veo 3 / Muse AI): Ghép nối thành chuỗi phim hoàn chỉnh!
		if len(motionClips) > 0 {
			s.appendLog(jobID, fmt.Sprintf("Nối %d phân cảnh video chuyển động AI (Google Veo 3 / Muse AI) thành phim 9:16...", len(motionClips)))
			if err := ConcatClips(ctx, motionClips, "9:16", silentVideo); err != nil {
				s.appendLog(jobID, fmt.Sprintf("Cảnh báo ghép clip (%v), chuyển sang ghép ảnh phân cảnh...", err))
			}
		}

		// Nếu chưa có silentVideo (hoặc ConcatClips thất bại): Dùng chuỗi ảnh với CapCut Ken Burns
		if !fileExists(silentVideo) {
			if len(photoPaths) == 0 {
				fail(fmt.Errorf("không tạo được hình ảnh hoặc video chuyển động nào cho các phân cảnh"))
				return
			}
			secsPerPhoto = targetSec / float64(len(photoPaths))
			if secsPerPhoto < 2.5 {
				secsPerPhoto = 2.5
			}
			s.appendLog(jobID, fmt.Sprintf("AI đang kết xuất chuỗi phân cảnh điện ảnh 9:16 (%.1fs/cảnh, %d cảnh AI 100%% gốc)...", secsPerPhoto, len(photoPaths)))
			if err := AssemblePhotoList(ctx, photoPaths, secsPerPhoto, "", 0, silentVideo, "9:16"); err != nil {
				fail(fmt.Errorf("dựng video AI 9:16 thất bại: %w", err))
				return
			}
		}

	}
	if err := ensureVideoDuration(ctx, silentVideo, float64(requiredDuration), "9:16"); err != nil {
		fail(fmt.Errorf("video nền chưa đạt %.0fs: %w", float64(requiredDuration), err))
		return
	}

	s.setStatus(jobID, StatusRunning, 85)
	s.appendLog(jobID, "Đang hoàn tất đóng gói và tối ưu video...")

	finalVideo := filepath.Join(s.outDir, fmt.Sprintf("viral_%s.mp4", jobID))

	// 3. Mux voiceover & nhạc nền
	musicPath := p.MusicFile
	if musicPath == "" && p.MusicURL != "" && fileExists(p.MusicURL) {
		musicPath = p.MusicURL
	}
	if musicPath == "" || !fileExists(musicPath) {
		musicGenre := p.Genre
		if musicGenre == "" {
			musicGenre = "viral"
		}
		musicPath = FindTrendingMusic(musicGenre)
		if musicPath != "" {
			s.appendLog(jobID, fmt.Sprintf("✓ Tự động chọn nhạc TikTok Trending (%s): %s", musicGenre, filepath.Base(musicPath)))
		}
	}

	var voiceFull string
	if !localMode && !isDanceOrPerformance(p.Topic, p.Genre) && len(wavPaths) > 0 {
		voiceFull = filepath.Join(jobDir, "voice_full.m4a")
		if err := concatWAVs(ctx, wavPaths, voiceFull); err != nil {
			s.appendLog(jobID, "Nối voiceover lỗi: "+err.Error())
			voiceFull = ""
		}
	} else if isDanceOrPerformance(p.Topic, p.Genre) {
		s.appendLog(jobID, "✓ Video vũ đạo/nhảy: Tắt hoàn toàn giọng đọc voiceover, chỉ phát nhạc nền trending sôi động theo chuẩn TikTok Dance.")
	}

	if voiceFull != "" && musicPath != "" {
		s.appendLog(jobID, "Lồng tiếng voiceover và hòa âm nhạc nền (ducking -8dB)…")
		voiceVideo := filepath.Join(jobDir, "voice_video.mp4")
		if err := ffmpegRun(ctx, "-i", silentVideo, "-i", voiceFull, "-map", "0:v", "-map", "1:a", "-af", "apad", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", voiceVideo); err == nil {
			if err := MixMusicBed(ctx, voiceVideo, musicPath, finalVideo); err != nil {
				s.appendLog(jobID, "Hòa âm lỗi, dùng bản voiceover: "+err.Error())
				_ = os.Rename(voiceVideo, finalVideo)
			}
		} else {
			_ = MuxMusic(ctx, silentVideo, musicPath, 0, finalVideo)
		}
	} else if voiceFull != "" {
		s.appendLog(jobID, "Ghép voiceover vào video…")
		_ = ffmpegRun(ctx, "-i", silentVideo, "-i", voiceFull, "-map", "0:v", "-map", "1:a", "-af", "apad", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", finalVideo)
	} else if musicPath != "" {
		s.appendLog(jobID, "Ghép nhạc nền trending sôi động…")
		if err := MuxMusic(ctx, silentVideo, musicPath, 0, finalVideo); err != nil {
			_ = copyViralFile(silentVideo, finalVideo)
		}
	} else {
		_ = copyViralFile(silentVideo, finalVideo)
	}

	caption := fmt.Sprintf("%s\n\n%s\n\n%s", script.Title, script.CallToAction, strings.Join(script.Hashtags, " "))
	s.setCaption(jobID, caption)

	// Master 4K Delivery: Nâng cấp video cuối lên chuẩn 4K UHD nếu bật MASTER_QUALITY_4K hoặc render chất lượng cao
	if IsMasterQuality4K() && fileExists(finalVideo) {
		s.appendLog(jobID, "✨ Nâng cấp Master 4K UHD (2160x3840 VideoToolbox 35 Mbps)...")
		video4k := filepath.Join(jobDir, "master_4k.mp4")
		if err := UpscaleVideoTo4K(ctx, finalVideo, video4k, "9:16"); err == nil && fileExists(video4k) {
			_ = os.Rename(video4k, finalVideo)
			s.appendLog(jobID, "✓ Video hoàn thiện đạt chuẩn điện ảnh 4K Ultra HD sắc nét tuyệt đối!")
		}
	}

	if !fileExists(finalVideo) {
		fail(fmt.Errorf("không tạo được file video đầu ra"))
		return
	}
	if err := ensureVideoDuration(ctx, finalVideo, float64(requiredDuration), "9:16"); err != nil {
		fail(fmt.Errorf("video đầu ra chưa đạt %.0fs: %w", float64(requiredDuration), err))
		return
	}
	finalDuration := probeDuration(ctx, finalVideo)
	if finalDuration < float64(requiredDuration)-0.05 {
		fail(fmt.Errorf("video đầu ra chỉ dài %.1fs; yêu cầu %ds", finalDuration, requiredDuration))
		return
	}
	s.setOutput(jobID, finalVideo)
	s.setStatus(jobID, StatusDone, 100)
	s.appendLog(jobID, fmt.Sprintf("Hoàn thành video: %s (%0.1fs)", script.Title, finalDuration))
	s.fireOnDone(jobID)
}

// isDanceOrPerformance kiểm tra chủ đề hoặc thể loại có phải là vũ đạo/nhảy/dance không.
func isDanceOrPerformance(topic, genre string) bool {
	t := strings.ToLower(topic)
	g := strings.ToLower(genre)
	return g == "dance" ||
		strings.Contains(t, "nhảy") ||
		strings.Contains(t, "dance") ||
		strings.Contains(t, "vũ đạo") ||
		strings.Contains(t, "sexy") ||
		strings.Contains(t, "choreo")
}

// createSolidColorPhoto tạo 1 ảnh nền mẫu chuẩn 9:16 hợp lệ (PNG) cho trường hợp offline, test hoặc quota hết.
func (s *Studio) createSolidColorPhoto(outPath string, w, h int, text string) error {
	_ = text
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 1920
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Đổ nền gradient dọc tinh tế từ xanh đậm sang đen tạo bối cảnh điện ảnh
	for y := 0; y < h; y++ {
		factor := float64(y) / float64(h)
		r := uint8(15 + factor*10)
		g := uint8(23 + factor*15)
		b := uint8(42 + factor*20)
		col := color.RGBA{R: r, G: g, B: b, A: 255}
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, col)
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func copyViralFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// ViralOriginalProfile lưu trữ đặc tính kỹ thuật & phong cách của video viral gốc để phản chiếu sang video AI.
type ViralOriginalProfile struct {
	Path           string  `json:"path"`
	Duration       float64 `json:"duration"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	HasAudio       bool    `json:"has_audio"`
	AudioExtracted string  `json:"audio_extracted,omitempty"`
	MotionVibe     string  `json:"motion_vibe"` // "energetic_beat", "smooth_sensual", "dynamic_street", "natural_lookbook"
	VisualMood     string  `json:"visual_mood"` // "neon_aesthetic", "warm_studio", "natural_daylight", "cinematic_dramatic"
	PacingSec      float64 `json:"pacing_sec"`
}

// AnalyzeViralOriginal bóc tách các thuộc tính của video viral gốc (thời lượng, âm thanh, nhịp điệu, visual mood).
func AnalyzeViralOriginal(ctx context.Context, samplePath, outDir string) *ViralOriginalProfile {
	prof := &ViralOriginalProfile{
		Path:       samplePath,
		Duration:   probeDuration(ctx, samplePath),
		HasAudio:   ProbeHasAudio(ctx, samplePath),
		MotionVibe: "energetic_beat",
		VisualMood: "neon_aesthetic",
		PacingSec:  3.0,
	}

	w, h := ProbeDims(ctx, samplePath)
	prof.Width = w
	prof.Height = h

	// Trích xuất âm thanh gốc nếu video có audio stream
	if prof.HasAudio && outDir != "" {
		audioFile := filepath.Join(outDir, "viral_orig_audio.m4a")
		if err := ffmpegRun(ctx, "-i", samplePath, "-vn", "-c:a", "aac", "-b:a", "192k", audioFile); err == nil && fileExists(audioFile) {
			prof.AudioExtracted = audioFile
		}
	}

	// Nhận diện phong cách chuyển động & visual mood từ tên file hoặc ngữ cảnh clip
	base := strings.ToLower(filepath.Base(samplePath))
	switch {
	case strings.Contains(base, "douyin") || strings.Contains(base, "hot") || strings.Contains(base, "beat") || strings.Contains(base, "cac"):
		prof.MotionVibe = "energetic_beat"
		prof.VisualMood = "neon_aesthetic"
		prof.PacingSec = 2.0
	case strings.Contains(base, "sexy") || strings.Contains(base, "bedroom") || strings.Contains(base, "phuonghoa"):
		prof.MotionVibe = "smooth_sensual"
		prof.VisualMood = "warm_studio"
		prof.PacingSec = 3.5
	case strings.Contains(base, "street") || strings.Contains(base, "duo"):
		prof.MotionVibe = "dynamic_street"
		prof.VisualMood = "natural_daylight"
		prof.PacingSec = 3.0
	case strings.Contains(base, "lookbook") || strings.Contains(base, "fashion") || strings.Contains(base, "dress"):
		prof.MotionVibe = "natural_lookbook"
		prof.VisualMood = "luxury_boutique"
		prof.PacingSec = 4.0
	default:
		prof.MotionVibe = "natural_cinematic"
		prof.VisualMood = "cinematic_dramatic"
		prof.PacingSec = 3.5
	}

	return prof
}
