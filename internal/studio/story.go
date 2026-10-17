package studio

// ---------------------------------------------------------------------------
// Story pipeline (Đợt F — pillar 3 post-pivot): truyện ngôi thứ nhất +
// ảnh minh họa từng cảnh + giọng đọc, KHÔNG quay video.
//
//   - Viết truyện: LLM (tái dùng prompts/story.txt) → JSON {title, logline, text}.
//   - Chia cảnh: LLM → JSON {scenes:[{text, image_prompt}]} (3–12 cảnh).
//   - Ảnh minh họa: MediaGen (Gemini, xoay key) — 1 ảnh 16:9 / cảnh.
//   - Giọng đọc: Narrator (chuỗi TTS Gemini→VieNeu→Edge) — 1 wav / cảnh.
//   - Dựng: tái dùng AssemblePhotoList (Ken Burns) từng cảnh → ConcatClips →
//     mux audio TTS → MixMusicBed (tùy chọn) → 16:9 MP4. KHÔNG chèn
//     chữ/phụ đề vào video (Ninh chốt 2026-10-12).
//   - QC: 1920x1080 + có audio + dài > 30s. Rớt → failed + lý do tiếng Việt.
//
// Fail-closed: thiếu LLM / thiếu ảnh / thiếu giọng đọc → không dựng video
// câm hoặc video thiếu cảnh. Không đụng code cinematic/Veo (đã parked).
// ---------------------------------------------------------------------------

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// KindStory là job kind cho pipeline kể chuyện.

// StoryLogTail trả về dòng log cuối cùng của job — cho bảng /stories
// (ListJobs không select cột log để nhẹ).
func (s *Studio) StoryLogTail(id string) string {
	var lg string
	if err := s.db.QueryRow(`SELECT log FROM studio_jobs WHERE id=?`, id).Scan(&lg); err != nil {
		return ""
	}
	lg = strings.TrimSpace(lg)
	if lg == "" {
		return ""
	}
	lines := strings.Split(lg, "\n")
	return lines[len(lines)-1]
}

// StoryLogFull trả về toàn bộ log của job (cho trang Nhật ký, đợt Y).
func (s *Studio) StoryLogFull(id string) string {
	var lg string
	if err := s.db.QueryRow(`SELECT log FROM studio_jobs WHERE id=?`, id).Scan(&lg); err != nil {
		return ""
	}
	return lg
}

// DeleteStoryJob xoá 1 job kể chuyện: assets, file output, thư mục work,
// các row DB. Chỉ xoá job kind=story (không đụng job kind khác).
func (s *Studio) DeleteStoryJob(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("không tìm thấy job %s", id)
	}
	if j.Kind != KindStory {
		return fmt.Errorf("job %s không phải truyện kể", id)
	}
	return s.DeleteJob(id)
}

const KindStory = "story"

// StoryParams mô tả 1 job kể chuyện.
type StoryParams struct {
	Topic           string `json:"topic"`
	Genre           string `json:"genre"`
	DurationMinutes int    `json:"duration_minutes,omitempty"` // 30, 45, 60 phút
	Words           int    `json:"words"`                       // độ dài truyện mục tiêu (từ)
	Scenes          int    `json:"scenes"`                      // số cảnh mục tiêu (3–24)
	MusicOn         bool   `json:"music_on"`                    // lồng nhạc nền
	MusicPath       string `json:"music_path"`                  // file nhạc local ("" = không nhạc)
	AccountID       int64  `json:"account_id,omitempty"`
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
	Hook3s          string `json:"hook_3s"`           // Mở màn giật gân trong 3 giây
	Logline         string `json:"logline"`
	Act1            string `json:"act_1"`             // Hồi 1: Khởi nguồn & Bí mật
	Act2            string `json:"act_2"`             // Hồi 2: Dấn thân & Xung đột
	Act3            string `json:"act_3"`             // Hồi 3: Cao trào kịch tính
	Act4            string `json:"act_4"`             // Hồi 4: Bước ngoặt & Cú twist
	Act5            string `json:"act_5"`             // Hồi 5: Lời giải & Dư âm sâu sắc
	Message         string `json:"message"`
	SuggestedVoice  string `json:"suggested_voice"`
	Soundscape      string `json:"soundscape"`
}

// CuratedAudioStoryPlots chứa danh sách các cốt truyện audio YouTube kinh điển (hệ thống tự nghĩ sẵn 100%).
var CuratedAudioStoryPlots = []AudioStoryPlot{
	{
		ID:             "plot-dem-khuya-01",
		Title:          "Đêm Mưa Bên Quán Trọ Cũ",
		Genre:          "tâm lý",
		GenreLabel:     "Radio Kể Chuyện Đêm Khuya",
		Logline:        "Một lữ khách lỡ đường trong đêm bão táp tá túc tại quán trọ ven sông và nghe câu chuyện ân nghĩa cảm động.",
		Act1:           "Đêm mưa bão mù trời, người đàn ông ướt sũng bước vào quán trọ ven sông của bà cụ già và cô cháu gái ít nói.",
		Act2:           "Bên tách trà gừng nghi ngút khói, bà cụ kể về người khách quen 10 năm trước đã gửi lại chiếc hộp gỗ bí ẩn chưa từng quay lại.",
		Act3:           "Khi người đàn ông mở chiếc hộp, bí mật chấn động về tình cha con và lời hứa xưa được tháo gỡ trong nước mắt.",
		Message:        "Ở đời, ân nghĩa và tình thân lớn hơn mọi sóng gió thời gian.",
		SuggestedVoice: "default",
		Soundscape:     "Tiếng mưa rơi nhẹ + Piano lofi acoustic",
	},
	{
		ID:             "plot-dem-khuya-02",
		Title:          "Chuyến Xe Buýt Cuối Cùng Trở Về Nhà",
		Genre:          "tâm lý",
		GenreLabel:     "Radio Kể Chuyện Đêm Khuya",
		Logline:        "Những cuộc gặp gỡ định mệnh trên chuyến xe buýt đêm muộn đưa một chàng trai trẻ tìm lại ý nghĩa cuộc sống.",
		Act1:           "Chàng trai trẻ thất nghiệp ngồi gục đầu trên chuyến xe buýt tuyến cuối lúc 23h30 trong ngày đông lạnh giá.",
		Act2:           "Một ông lão bán vé số dạo bước lên xe, chia cho anh nửa củ khoai nướng nóng hổi và kể về thời thanh xuân vượt khó.",
		Act3:           "Ánh mắt ấm áp và lời động viên giản dị đã thắp sáng lại hy vọng sống cho chàng trai trước khi bước xuống trạm dừng.",
		Message:        "Đôi khi, một chút ấm áp từ người xa lạ lại đủ sức cứu rỗi cả một cuộc đời.",
		SuggestedVoice: "default",
		Soundscape:     "Tiếng còi xe xa xăm + Giai điệu mộc mạc",
	},
	{
		ID:             "plot-truyen-ma-01",
		Title:          "Tiếng Bước Chân Sau Bờ Tre Làng",
		Genre:          "kinh dị",
		GenreLabel:     "Truyện Ma Làng Quê",
		Logline:        "Bí mật rùng rợn về ngôi mộ vô danh dưới gốc đa đầu làng và món nợ nhân quả của phú hộ xưa.",
		Act1:           "Đêm rằm tháng bảy, chàng sinh viên về quê chịu tang ông nội thì nghe tiếng bước chân kỳ lạ dồn dập sau rặng tre làng.",
		Act2:           "Người làng cảnh báo không ai được ra gốc đa sau nửa đêm, nhưng cuốn nhật ký cổ của ông nội để lại hé lộ một bí mật đáng sợ.",
		Act3:           "Cuộc đối mặt rợn gáy trong đêm sương lạnh và sự thật về lời nguyền nhân quả ba đời được hóa giải khi ánh bình minh ló rạng.",
		Message:        "Gieo nhân nào gặt quả nấy, sống ngay thẳng thì quỷ thần cũng phải kính nhường.",
		SuggestedVoice: "default",
		Soundscape:     "Tiếng gió rít đêm sương + Tiếng côn trùng vắng",
	},
	{
		ID:             "plot-truyen-ma-02",
		Title:          "Căn Nhà Bỏ Hoang Bên Bến Đò Cũ",
		Genre:          "kinh dị",
		GenreLabel:     "Truyện Ma Làng Quê",
		Logline:        "Ngôi nhà gỗ mục nát ngã ba sông và bí ẩn về người lái đò không bao giờ nhận tiền của khách lỡ chuyến.",
		Act1:           "Đêm mưa gió mù mịt, một vị khách lỡ độ đường ghé vào bến đò vắng và được một người đàn ông chèo thuyền chở qua sông.",
		Act2:           "Người chèo đò kể về căn nhà bỏ hoang bên bờ sông từng là nơi ở của gia đình thầy lang cứu giúp dân làng trong nạn đói.",
		Act3:           "Sáng hôm sau hỏi người dân địa phương mới biết bến đò ấy đã không còn ai chèo từ 30 năm trước...",
		Message:        "Tấm lòng nhân ái thì dù qua bao kiếp luân hồi vẫn lưu lại phước báu nhân gian.",
		SuggestedVoice: "default",
		Soundscape:     "Tiếng nước róc rách + Tiếng chuông chùa vẳng xa",
	},
	{
		ID:             "plot-trinh-tham-01",
		Title:          "Bức Thư Bí Mật Trong Chiếc Đồng Hồ Cổ",
		Genre:          "trinh thám",
		GenreLabel:     "Kỳ Án Trinh Thám & Giải Mã",
		Logline:        "Vụ mất tích bí ẩn của một giáo sư già và cuộc đấu trí nghẹt thở giữa viên thám tử và kẻ giấu mặt.",
		Act1:           "Vị giáo sư qua đời để lại chiếc đồng hồ quả lắc đứng kim đúng 3 giờ 15 phút cùng bản di chúc kỳ lạ.",
		Act2:           "Những mảnh ghép manh mối đứt đoạn dẫn thám tử lần ra những phi vụ ngầm được che giấu dưới vỏ bọc danh giá.",
		Act3:           "Màn lật mặt thủ phạm nghẹt thở ngay trước khi chuyến tàu đêm khởi hành rời khỏi thành phố.",
		Message:        "Lưới trời lồng lộng, không tội ác nào có thể che giấu mãi mãi.",
		SuggestedVoice: "default",
		Soundscape:     "Dramatic Suspense Ambient + Tiếng tích tắc đồng hồ",
	},
	{
		ID:             "plot-nhan-sinh-01",
		Title:          "Bát Cơm Nguội Của Người Mẹ Già",
		Genre:          "triết lý",
		GenreLabel:     "Bài Học Nhân Sinh & Đạo Lý",
		Logline:        "Câu chuyện xúc động về sự hy sinh thầm lặng của người mẹ nghèo và bài học thức tỉnh người con thành đạt.",
		Act1:           "Sau 15 năm bôn ba thành đạt chốn phồn hoa, người con trai lần đầu trở về căn nhà ngói cũ ven đê thăm mẹ già.",
		Act2:           "Bữa cơm đạm bạc với bát cơm nguội mẹ nhường con hé mở cuốn sổ nợ mẹ đã âm thầm giấu kín suốt thời niên thiếu của anh.",
		Act3:           "Giọt nước mắt muộn màng trong vòng tay gầy guộc của mẹ khiến thính giả không cầm được xúc động.",
		Message:        "Đừng đợi đến khi cha mẹ khuất núi mới nhận ra điều quý giá nhất trên đời.",
		SuggestedVoice: "female",
		Soundscape:     "Acoustic Guitar mộc mạc + Tiếng sáo ru êm đềm",
	},
	{
		ID:             "plot-thuong-truong-01",
		Title:          "Cú Lật Kèo Phút 89",
		Genre:          "kinh doanh",
		GenreLabel:     "Thương Trường & Khởi Nghiệp",
		Logline:        "Từ bờ vực phá sản với khoản nợ hàng chục tỷ, chàng trai 28 tuổi lội ngược dòng ngoạn mục nhờ một nguyên tắc kinh doanh bất biến.",
		Act1:           "Công ty đứng trước nguy cơ phá sản vì đối tác lớn bất ngờ hủy hợp đồng ngay trước ngày thanh toán.",
		Act2:           "Quyết định táo bạo từ bỏ lợi nhuận ngắn hạn để bảo vệ quyền lợi của các khách hàng nhỏ lẻ và đối tác phụ trợ.",
		Act3:           "Hiệu ứng truyền miệng bùng nổ mang lại hợp đồng cứu cánh và bài học xương máu cho giới khởi nghiệp.",
		Message:        "Trong kinh doanh, chữ TÍN chính là tài sản quý giá nhất.",
		SuggestedVoice: "default",
		Soundscape:     "Inspiring Ambient Beat + Nhịp điệu dồn dập",
	},
	{
		ID:             "plot-da-su-01",
		Title:          "Huyền Thoại Thanh Gươm Dưới Lòng Hồ",
		Genre:          "lịch sử",
		GenreLabel:     "Dã Sử Huyền Huyễn",
		Logline:        "Câu chuyện dã sử hào hùng về người tráng sĩ vô danh đứng lên bảo vệ bờ cõi biên ải trước quân thù.",
		Act1:           "Đêm mùa đông buốt giá nơi ải bắc, tiếng tù và báo động giặc ngoại xâm áp sát cửa ải.",
		Act2:           "Hành trình tìm lại thanh thần kiếm cổ xưa và lời thề nguyền vì giang sơn xã tắc muôn đời.",
		Act3:           "Trận chiến oanh liệt dưới ánh trăng sáng rực, lưu danh thiên cổ như biểu tượng bất khuất.",
		Message:        "Tinh thần bảo vệ non sông ngàn đời là dòng máu chảy trong tim mỗi người con đất Việt.",
		SuggestedVoice: "default",
		Soundscape:     "Đàn tranh + Tiếng trống trận hào hùng vang dội",
	},
}

// GetCuratedAudioStories trả về danh sách cốt truyện audio mẫu theo thể loại.
func GetCuratedAudioStories(genre string) []AudioStoryPlot {
	genre = strings.ToLower(strings.TrimSpace(genre))
	if genre == "" || genre == "all" {
		return append([]AudioStoryPlot(nil), CuratedAudioStoryPlots...)
	}
	var out []AudioStoryPlot
	for _, p := range CuratedAudioStoryPlots {
		if strings.EqualFold(p.Genre, genre) || strings.Contains(strings.ToLower(p.GenreLabel), genre) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return append([]AudioStoryPlot(nil), CuratedAudioStoryPlots...)
	}
	return out
}

// DefaultAudioStoryTopic trả về 1 chủ đề cốt truyện audio hấp dẫn theo thể loại nếu người dùng để trống.
func DefaultAudioStoryTopic(genre string) string {
	plots := GetCuratedAudioStories(genre)
	if len(plots) > 0 {
		return plots[0].Title + " - " + plots[0].Logline
	}
	return "Đêm Mưa Bên Quán Trọ Cũ - Câu chuyện cảm động về ân nghĩa cuộc đời"
}

// GenerateUniqueAudioStory sinh ra 1 cốt truyện audio YouTube trường thiên (30 - 60 phút)
// theo đúng chủ đề người dùng cài đặt, với cấu trúc 5 hồi kịch tính, lôi cuốn và không bao giờ trùng lặp.
func GenerateUniqueAudioStory(customTopic, genre string, durationMinutes int) AudioStoryPlot {
	if durationMinutes <= 0 {
		durationMinutes = 30
	}
	if durationMinutes < 20 {
		durationMinutes = 30
	}
	if durationMinutes > 90 {
		durationMinutes = 60
	}

	genre = strings.ToLower(strings.TrimSpace(genre))
	if genre == "" {
		genre = "tâm lý"
	}

	words := durationMinutes * 150 // chuẩn 150 từ/phút đọc audio

	// Danh sách bối cảnh độc bản
	settings := []string{
		"Quán trọ ven sông bên bến đò xưa trong đêm mưa bão mù mịt",
		"Căn nhà cổ rêu phong ba gian nép mình sâu trong con ngõ nhỏ phố cổ",
		"Chuyến tàu đêm Thống Nhất chở những mảnh đời tha hương về quê đón Tết",
		"Trạm dừng chân heo hút trên đỉnh đèo mờ sương lạnh buốt",
		"Xóm trọ nghèo ngoại thành nơi cưu mang những số phận bươn chải nhọc nhằn",
		"Tiệm may áo dài gia truyền ba đời bên dòng kênh lịch sử",
		"Ngôi làng chài ven biển miền Trung kiên cường sau cơn bão lớn",
		"Căn biệt thự gỗ cổ kính ẩn mình giữa rừng thông sương giăng",
		"Ngôi chùa cổ thanh tịnh trên sườn núi nơi lữ khách tìm lại sự bình yên",
		"Khu chợ nổi miền sông nước với những nghĩa cử hào hiệp ấm lòng",
	}

	// Danh sách nhân vật chính
	protagonists := []string{
		"một người đàn ông trung niên mang vết sẹo ký ức và chiếc vali bí mật",
		"người thợ thủ công già câm lặng với đôi bàn tay chai sần tài hoa",
		"cô gái trẻ rời thành phố phồn hoa về lại quê xưa tìm câu trả lời cho biến cố cuộc đời",
		"bác tài xế xe tải đường dài chuyên chở những chuyến hàng đêm qua miền đất lạ",
		"cậu thanh niên nghèo tha hương mang theo lời dặn thiêng liêng của cha mẹ",
		"người thương gia từng trải qua bờ vực phá sản tìm về cội nguồn tạ lỗi",
		"cụ bà bán quán nước chè chén đầu đình lưu giữ bí mật 40 năm của cả làng",
		"người thợ làm gốm bền bỉ lưu giữ hồn đất qua bao thăng trầm thời cuộc",
	}

	// Danh sách cú twist bất ngờ (Plot Twist)
	twists := []string{
		"Người khách lạ tá túc năm xưa chính là người anh trai thất lạc hơn 20 năm của gia đình.",
		"Chiếc hộp gỗ cũ kỹ tưởng chừng rỗng không lại cất giữ cuốn sổ nợ đã được người cha trả hết bằng cả tính mạng.",
		"Ân nhân cứu mạng gia tộc năm xưa lại chính là người từng bị hàm oan mà dòng họ hắt hủi.",
		"Kẻ thù tưởng chừng tàn nhẫn thực chất lại âm thầm gánh chịu tiếng xấu để bảo vệ di sản của người mẹ.",
		"Toàn bộ chuỗi biến cố bí ẩn xảy ra chỉ để thử lòng trung hiếu và hoàn thành lời thề son sắt ba mươi năm trước.",
		"Bức thư giấu dưới đáy lư hương hé lộ bí mật chấn động về thân thế thực sự của người kế thừa.",
	}

	// Danh sách thông điệp nhân sinh sâu sắc
	messages := []string{
		"Ở đời, ân nghĩa và lòng hiếu thảo chính là ngọn đèn soi sáng qua mọi đêm đen giông bão.",
		"Tiền bạc danh vọng trôi qua như mây khói, chỉ có tình thân và sự bao dung mới ở lại muôn đời.",
		"Mỗi bước ngoặt định mệnh đều là bài học để con người ta học cách buông bỏ hận thù và trân trọng hiện tại.",
		"Sống ngay thẳng, thiện lương thì dẫu phong ba bão táp, quỷ thần cũng phải kiêng nhường.",
		"Một chút ấm áp sẻ chia giữa người với người có thể cứu rỗi cả một linh hồn lạc lối.",
	}

	// Danh sách câu Hook 3 giây giữ chân người nghe
	hooks := []string{
		"Nửa đêm hôm ấy, khi tiếng sấm vừa rạch ngang bầu trời, một tiếng gõ cửa dồn dập vang lên làm rung chuyển cả căn nhà cũ...",
		"Có những bí mật nếu không được phơi bày trong đêm nay, có lẽ người ta sẽ mang theo cả mối ân oán xuống mồ...",
		"Người đàn ông lạ mặt bước vào quán lúc 2 giờ sáng, đặt lên bàn một bọc vải đẫm nước mưa và run rẩy nói một câu...",
		"Trước khi nhắm mắt, ông lão chỉ thì thầm đúng một câu khiến tất cả con cháu trong nhà đều chết lặng...",
		"Đúng 12 giờ đêm, chiếc chuông đồng cổ xưa bỗng nhiên tự gióng lên 3 hồi chuông lạnh toát sống lưng...",
	}

	now := time.Now().UnixNano()
	sIdx := int(now % int64(len(settings)))
	pIdx := int((now / 7) % int64(len(protagonists)))
	tIdx := int((now / 13) % int64(len(twists)))
	mIdx := int((now / 19) % int64(len(messages)))
	hIdx := int((now / 23) % int64(len(hooks)))

	topicClean := strings.TrimSpace(customTopic)
	var title string
	if topicClean != "" {
		title = topicClean
	} else {
		switch genre {
		case "kinh dị":
			title = "Tiếng Bước Chân Sau Bờ Tre Làng - Bí Mật Ngôi Mộ Vô Danh"
		case "trinh thám":
			title = "Vụ Mất Tích Kỳ Bí Dưới Đêm Mưa Lạnh"
		case "kinh doanh":
			title = "Cuộc Lội Ngược Dòng Của Kẻ Phá Sản"
		default:
			title = fmt.Sprintf("Ký Ức Bên %s - Chuyện Tình & Nghĩa Cuộc Đời", settings[sIdx])
		}
	}

	genreLabel := "Radio Kể Chuyện Đêm Khuya"
	switch genre {
	case "kinh dị":
		genreLabel = "Truyện Kinh Dị & Tâm Linh Làng Quê"
	case "trinh thám":
		genreLabel = "Kỳ Án Trinh Thám & Giải Mã"
	case "kinh doanh":
		genreLabel = "Thương Trường & Khởi Nghiệp Bất Khả Thi"
	case "triết lý":
		genreLabel = "Bài Học Nhân Sinh & Đạo Lý Sâu Sắc"
	}

	plotID := fmt.Sprintf("story-%s-%d", genre, now%100000)

	return AudioStoryPlot{
		ID:              plotID,
		Title:           title,
		Genre:           genre,
		GenreLabel:      genreLabel,
		DurationMinutes: durationMinutes,
		WordsCount:      words,
		Hook3s:          hooks[hIdx],
		Logline:         fmt.Sprintf("Câu chuyện trường thiên %d phút: Cuộc hội ngộ định mệnh của %s tại %s, hé mở %s", durationMinutes, protagonists[pIdx], settings[sIdx], twists[tIdx]),
		Act1:            fmt.Sprintf("Mở màn (0-%d phút): %s bối cảnh được thiết lập tại %s. Sự xuất hiện của %s mở ra những uẩn khúc chưa từng được giải đáp.", durationMinutes/5, hooks[hIdx], settings[sIdx], protagonists[pIdx]),
		Act2:            fmt.Sprintf("Thắt nút (%d-%d phút): Những manh mối kỳ lạ dần hé lộ qua từng kỷ vật cũ. Mối xung đột ngấm ngầm giữa quá khứ và hiện tại bắt đầu đẩy các nhân vật vào thế khó.", durationMinutes/5, (durationMinutes*2)/5),
		Act3:            fmt.Sprintf("Cao trào (%d-%d phút): Đỉnh điểm kịch tính khi bí mật lớn nhất bị đặt trước ranh giới phơi bày. Mọi sự nghi ngờ và tổn thương dồn nén nhiều năm bùng nổ nghẹt thở.", (durationMinutes*2)/5, (durationMinutes*3)/5),
		Act4:            fmt.Sprintf("Bước ngoặt (%d-%d phút): Cú twist bất ngờ xoay chuyển hoàn toàn góc nhìn của người nghe: %s", (durationMinutes*3)/5, (durationMinutes*4)/5, twists[tIdx]),
		Act5:            fmt.Sprintf("Mở nút & Lắng đọng (%d-%d phút): Mọi ân oán được cởi bỏ bằng sự thấu hiểu và giọt nước mắt hối hận. Thông điệp đọng lại: %s", (durationMinutes*4)/5, durationMinutes, messages[mIdx]),
		Message:         messages[mIdx],
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng mưa rơi đêm khuya + Piano lofi acoustic mộc mạc",
	}
}

// CreateStoryJob kiểm tra tham số, xếp job kể chuyện và chạy nền.
func (s *Studio) CreateStoryJob(p StoryParams) (string, error) {
	if strings.TrimSpace(p.Topic) == "" {
		return "", fmt.Errorf("cần chủ đề truyện")
	}

	// Chuẩn hóa thời lượng 30 - 60 phút theo yêu cầu người dùng
	if p.DurationMinutes <= 0 {
		p.DurationMinutes = 30
	}
	if p.DurationMinutes < 15 {
		p.DurationMinutes = 15
	}
	if p.DurationMinutes > 120 {
		p.DurationMinutes = 60
	}

	// Tốc độ audio YouTube chuẩn: ~150 từ/phút
	calcWords := p.DurationMinutes * 150
	if p.Words < 200 || p.Words < calcWords {
		p.Words = calcWords
	}
	if p.Words > 15000 {
		p.Words = 15000
	}

	// Số cảnh minh họa 16:9 với chuyển cảnh Ken Burns
	if p.Scenes < 3 {
		p.Scenes = p.DurationMinutes / 3
		if p.Scenes < 6 {
			p.Scenes = 6
		}
	}
	if p.Scenes > 24 {
		p.Scenes = 24
	}

	title := fmt.Sprintf("Kể chuyện [%dp]: %s", p.DurationMinutes, strings.TrimSpace(p.Topic))
	if len(title) > 90 {
		title = string([]rune(title)[:90]) + "…"
	}
	id, err := s.insertJob(KindStory, title, p)
	if err != nil {
		return "", err
	}
	go s.runStory(id, p)
	return id, nil
}

// GenerateTopics nhờ LLM nghĩ ra n chủ đề truyện mới theo thể loại.
// Dùng để tự refill hàng đợi khi Ninh không nhập tay (zero-touch).
// Nil LLM → lỗi rõ ràng (caller giữ note trung thực, không bịa chủ đề).
func (s *Studio) GenerateTopics(ctx context.Context, genre string, n int) ([]string, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("cần LLM để nghĩ chủ đề (chưa cấu hình)")
	}
	if n < 1 {
		n = 5
	}
	if n > 20 {
		n = 20
	}
	genre = strings.TrimSpace(genre)
	if genre == "" {
		genre = "tâm lý"
	}
	text, err := s.llm.Complete(ctx,
		"Bạn là biên tập truyện Việt Nam. Chỉ trả lời JSON thuần, không giải thích.",
		fmt.Sprintf(`Nghĩ ra đúng %d chủ đề truyện ngắn ngôi thứ nhất, thể loại %s, `+
			`hợp gu khán giả Việt Nam xem YouTube (đời thường, cảm xúc, kịch tính vừa phải, `+
			`không kinh dị máu me, không chính trị). Mỗi chủ đề 1 dòng ngắn gọn, cụ thể, `+
			`khác nhau, không trùng lặp ý tưởng.\n\nChỉ trả JSON: {"topics": ["...", ...]}`,
			n, genre))
	if err != nil {
		return nil, fmt.Errorf("nghĩ chủ đề: %w", err)
	}
	var raw struct {
		Topics []string `json:"topics"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, fmt.Errorf("nghĩ chủ đề: %w", err)
	}
	var out []string
	for _, t := range raw.Topics {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nghĩ chủ đề: LLM không trả chủ đề nào")
	}
	return out, nil
}

// StoryJobs trả về các job kể chuyện mới nhất.
func (s *Studio) StoryJobs(limit int) []Job {
	var out []Job
	for _, j := range s.ListJobs(limit * 3) {
		if j.Kind == KindStory {
			out = append(out, j)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// writeStory dùng LLM viết truyện ngôi thứ nhất (tái dùng story.txt).
func (s *Studio) writeStory(ctx context.Context, p StoryParams) (*storyDoc, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("cần LLM để viết truyện (chưa cấu hình)")
	}
	prompt, err := directorPrompt("story.txt", promptData{
		Topic: p.Topic, Genre: p.Genre, StoryWords: p.Words,
		Orient: "Viết ở NGÔI THỨ NHẤT (xưng \"tôi\") từ đầu đến cuối",
		// Seconds chỉ để LLM ước lượng "độ thịt" truyện; thời lượng thật
		// đo từ audio TTS sau này.
		Seconds: p.Words / 3,
	})
	if err != nil {
		return nil, err
	}
	text, err := s.llm.Complete(ctx,
		"Bạn là nhà văn Việt Nam. Chỉ trả lời JSON thuần, không giải thích.", prompt)
	if err != nil {
		return nil, fmt.Errorf("viết truyện: %w", err)
	}
	var doc storyDoc
	if err := parseDirectorJSON(text, &doc); err != nil {
		return nil, fmt.Errorf("viết truyện: %w", err)
	}
	doc.Title = strings.TrimSpace(doc.Title)
	doc.Text = strings.TrimSpace(doc.Text)
	if doc.Text == "" {
		return nil, fmt.Errorf("viết truyện: truyện rỗng")
	}
	return &doc, nil
}

// splitScenes chia truyện thành các cảnh (đoạn văn + prompt ảnh).
func (s *Studio) splitScenes(ctx context.Context, p StoryParams, doc *storyDoc) ([]storyScene, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("cần LLM để chia cảnh (chưa cấu hình)")
	}
	sys := "Bạn là biên tập storyboard Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	prompt := fmt.Sprintf(`Chia truyện sau thành đúng %d cảnh theo trình tự thời gian.
Mỗi cảnh: 1 đoạn văn kể ở ngôi thứ nhất (lấy nguyên văn hoặc tóm sát từ truyện, 40–120 từ),
kèm 1 image_prompt tiếng Anh mô tả 1 khung hình minh họa điện ảnh cho cảnh đó
(ánh sáng, màu sắc, bố cục 16:9, nhân vật Việt Nam nếu có; không chữ trong ảnh).

Truyện: %s

Chỉ trả JSON: {"scenes": [{"text": "...", "image_prompt": "..."}, ...]}`,
		p.Scenes, doc.Text)
	text, err := s.llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("chia cảnh: %w", err)
	}
	var raw struct {
		Scenes []storyScene `json:"scenes"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, fmt.Errorf("chia cảnh: %w", err)
	}
	var scenes []storyScene
	for _, sc := range raw.Scenes {
		sc.Text = strings.TrimSpace(sc.Text)
		sc.ImagePrompt = strings.TrimSpace(sc.ImagePrompt)
		if sc.Text == "" || sc.ImagePrompt == "" {
			continue
		}
		scenes = append(scenes, sc)
	}
	if len(scenes) < 3 {
		return nil, fmt.Errorf("chia cảnh: chỉ được %d cảnh hợp lệ (cần ≥3)", len(scenes))
	}
	if len(scenes) > 12 {
		scenes = scenes[:12]
	}
	return scenes, nil
}

// concatWAVs nối các file wav thành 1 track audio m4a (aac).
func concatWAVs(ctx context.Context, wavs []string, outPath string) error {
	if len(wavs) == 0 {
		return fmt.Errorf("không có track giọng đọc")
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	var args []string
	var fc []string
	for i, w := range wavs {
		args = append(args, "-i", w)
		fc = append(fc, fmt.Sprintf("[%d:a]", i))
	}
	fc = append(fc, fmt.Sprintf("concat=n=%d:v=0:a=1[aout]", len(wavs)))
	args = append(args, "-filter_complex", strings.Join(fc, ""), "-map", "[aout]",
		"-c:a", "aac", "-b:a", "160k", outPath)
	return ffmpegRun(ctx, args...)
}

// findDonePhoto tìm ảnh đã vẽ xong của job cũ có cùng prompt — để job
// retry tái dùng, khỏi đốt image-gen vẽ lại (đợt Y). "" = không có.
func (s *Studio) findDonePhoto(jobID, prompt string) string {
	var path string
	err := s.db.QueryRow(`SELECT path FROM studio_assets
		WHERE job_id=? AND kind='photo' AND status=? AND prompt=? LIMIT 1`,
		jobID, StatusDone, prompt).Scan(&path)
	if err != nil || path == "" {
		return ""
	}
	if fi, statErr := os.Stat(path); statErr != nil || fi.IsDir() {
		return ""
	}
	return path
}

// findDoneAudio tìm file wav đã tổng hợp xong của job cũ — để job retry tái
// dùng, khỏi đốt quota TTS tổng hợp lại (đợt AC). Scene retry dùng cùng params
// nên scene thứ i có text giống nhau; chỉ cần file tồn tại và >0 byte.
func (s *Studio) findDoneAudio(jobID string, sceneIdx int) string {
	if jobID == "" {
		return ""
	}
	p := filepath.Join(s.workRoot, jobID, fmt.Sprintf("scene-%02d.wav", sceneIdx))
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return ""
	}
	return p
}
func copyStoryFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// runStory chạy pipeline kể chuyện trong goroutine nền.
func (s *Studio) runStory(id string, p StoryParams) {
	// Đợt AI 24/7: recover panic — một panic không được giết cả app
	// (pipeline chạy hàng chục phút, chạm nhiều dữ liệu ngoài).
	defer func() {
		if r := recover(); r != nil {
			s.setStatus(id, StatusFailed, 100)
			s.appendLog(id, fmt.Sprintf("LỖI: panic (%v) — job dừng, app vẫn chạy.", r))
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		return
	}

	s.setStatus(id, StatusRunning, 5)
	fail := func(err error) {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
	}

	if s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu GEMINI_API_KEYS hoặc key lỗi)"))
		return
	}
	if s.narrator == nil {
		fail(fmt.Errorf("TTS chưa sẵn sàng (không có chuỗi giọng đọc)"))
		return
	}

	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}

	// 1. Viết truyện.
	s.appendLog(id, "Nhà văn đang viết truyện…")
	doc, err := s.writeStory(ctx, p)
	if err != nil {
		fail(err)
		return
	}
	title := strings.TrimSpace(doc.Title)
	if title != "" {
		s.db.Exec(`UPDATE studio_jobs SET title=? WHERE id=?`, "Kể chuyện: "+title, id)
	}
	s.appendLog(id, fmt.Sprintf("Truyện: %s (%d ký tự)", firstLine(title, doc.Text), len(doc.Text)))
	s.setStatus(id, StatusRunning, 15)

	// 2. Chia cảnh.
	s.appendLog(id, "Chia cảnh + viết prompt ảnh…")
	scenes, err := s.splitScenes(ctx, p, doc)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, fmt.Sprintf("Storyboard: %d cảnh", len(scenes)))
	s.setStatus(id, StatusRunning, 25)

	// 3+4. Vẽ ảnh + đọc giọng từng cảnh (fail-closed từng cảnh).
	var clips []string
	var wavs []string
	for i, sc := range scenes {
		aid := s.addAsset(id, i, "photo", sc.ImagePrompt)
		s.setAsset(aid, StatusRunning, "")

		img := filepath.Join(work, fmt.Sprintf("scene-%02d.png", i))
		imgPrompt := sc.ImagePrompt + ". Cinematic illustration, 16:9 widescreen, " +
			"Vietnamese characters if any, no text, no watermark."
		// Đợt Y: job retry tái dùng ảnh đã vẽ xong ở job cũ (cùng prompt) —
		// khỏi đốt image-gen vẽ lại.
		reused := false
		if oldImg := s.findDonePhoto(p.ReuseFromJobID, sc.ImagePrompt); oldImg != "" {
			if cerr := copyStoryFile(oldImg, img); cerr == nil {
				s.appendLog(id, fmt.Sprintf("Dùng lại ảnh cảnh %d/%d từ lần trước (khỏi vẽ lại)", i+1, len(scenes)))
				reused = true
			}
		}
		if !reused {
			s.appendLog(id, fmt.Sprintf("Vẽ ảnh cảnh %d/%d…", i+1, len(scenes)))
			if err := s.mg.GenerateImage(ctx, imgPrompt, nil, img); err != nil {
				s.setAsset(aid, StatusFailed, "")
				fail(fmt.Errorf("cảnh %d: vẽ ảnh lỗi (%v) — dừng, không dựng thiếu cảnh", i+1, err))
				return
			}
		}

		s.appendLog(id, fmt.Sprintf("Đọc giọng cảnh %d/%d…", i+1, len(scenes)))
		wav := filepath.Join(work, fmt.Sprintf("scene-%02d.wav", i))
		// Đợt AC: job retry tái dùng wav đã tổng hợp ở job cũ — khỏi đốt
		// quota TTS (giống findDonePhoto cho ảnh ở đợt Y).
		// Không `continue` ở nhánh dùng lại: cảnh vẫn phải dựng clip, nếu
		// không video sẽ thiếu cảnh so với giọng đọc.
		reusedWav := false
		if oldWav := s.findDoneAudio(p.ReuseFromJobID, i); oldWav != "" {
			if cerr := copyStoryFile(oldWav, wav); cerr == nil {
				s.appendLog(id, fmt.Sprintf("Dùng lại giọng đọc cảnh %d/%d từ lần trước (khỏi tổng hợp lại)", i+1, len(scenes)))
				reusedWav = true
			}
		}
		if !reusedWav {
			wavBytes, err := s.narrator(ctx, sc.Text, p.Voice)
			if err != nil {
				s.setAsset(aid, StatusFailed, "")
				fail(fmt.Errorf("cảnh %d: TTS lỗi (%v) — dừng, không tạo video câm", i+1, err))
				return
			}
			if err := os.WriteFile(wav, wavBytes, 0o644); err != nil {
				s.setAsset(aid, StatusFailed, "")
				fail(err)
				return
			}
		}
		dur := storyProbeDuration(ctx, wav)
		if dur < 1 {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: audio giọng đọc quá ngắn (%.1fs)", i+1, dur))
			return
		}
		wavs = append(wavs, wav)

		// Clip Ken Burns cho cảnh (tái dùng AssemblePhotoList, khổ 16:9).
		clip := filepath.Join(work, fmt.Sprintf("scene-%02d.mp4", i))
		if err := AssemblePhotoList(ctx, []string{img}, dur, "", 0, clip, "16:9"); err != nil {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: dựng clip lỗi: %w", i+1, err))
			return
		}
		s.setAsset(aid, StatusDone, img)
		clips = append(clips, clip)
		s.setStatus(id, StatusRunning, 25+int(45*float64(i+1)/float64(len(scenes))))
	}

	// 5. Dựng: nối clip → mux audio TTS → nhạc nền (không chữ).
	s.appendLog(id, "Nối các cảnh…")
	videoOnly := filepath.Join(work, "video-only.mp4")
	if err := ConcatClips(ctx, clips, "16:9", videoOnly); err != nil {
		fail(fmt.Errorf("nối cảnh: %w", err))
		return
	}
	s.appendLog(id, "Ghép giọng đọc…")
	voice := filepath.Join(work, "voice.m4a")
	if err := concatWAVs(ctx, wavs, voice); err != nil {
		fail(fmt.Errorf("nối giọng đọc: %w", err))
		return
	}
	withAudio := filepath.Join(work, "with-audio.mp4")
	if err := ffmpegRun(ctx, "-i", videoOnly, "-i", voice,
		"-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac",
		"-shortest", withAudio); err != nil {
		fail(fmt.Errorf("ghép audio: %w", err))
		return
	}
	// Không chèn chữ vào video (Ninh chốt 2026-10-12): không phụ đề.
	subbed := withAudio
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	if p.MusicOn && strings.TrimSpace(p.MusicPath) != "" {
		s.appendLog(id, "Lồng nhạc nền…")
		if err := MixMusicBed(ctx, subbed, p.MusicPath, final); err != nil {
			s.appendLog(id, "Nhạc nền lỗi: "+err.Error()+" (giữ bản không nhạc)")
			if err := os.Rename(subbed, final); err != nil {
				fail(err)
				return
			}
		}
	} else {
		if err := os.Rename(subbed, final); err != nil {
			fail(err)
			return
		}
	}

	// 6. QC: 1920x1080 + có audio + dài > 30s (số thật đo từ file).
	s.appendLog(id, "Kiểm tra chất lượng…")
	if w, h := ProbeDims(ctx, final); w != 1920 || h != 1080 {
		fail(fmt.Errorf("QC: khổ video %dx%d, cần 1920x1080", w, h))
		return
	}
	if !ProbeHasAudio(ctx, final) {
		fail(fmt.Errorf("QC: video thiếu audio"))
		return
	}
	if d := storyProbeDuration(ctx, final); d < 30 {
		fail(fmt.Errorf("QC: video chỉ dài %.0fs (cần > 30s)", d))
		return
	}
	dur := storyProbeDuration(ctx, final)
	s.appendLog(id, fmt.Sprintf("QC đạt: 1920x1080, có audio, %.0fs, %d cảnh", dur, len(scenes)))

	s.setOutput(id, filepath.Base(final))

	// 7. Auto-Clipper: Tự động trích xuất các clip ngắn 3-5 phút (9:16) cho TikTok/Reels nếu được bật
	if p.AutoClipShorts && dur >= 30 {
		s.appendLog(id, "Bắt đầu tự động trích xuất các clip ngắn 3–5 phút (9:16) cho TikTok & Reels…")
		clipDur := 180.0 // 3 phút mặc định
		if p.ClipDurationSec >= 30 && p.ClipDurationSec <= 300 {
			clipDur = float64(p.ClipDurationSec)
		} else if dur > 360 {
			clipDur = 240.0 // 4 phút nếu truyện dài
		}
		part := 1
		for start := 0.0; start < dur; start += clipDur {
			length := clipDur
			if start+length > dur {
				length = dur - start
			}
			if length < 15 {
				break
			}
			clipName := fmt.Sprintf("studio-%s-clip%d.mp4", id, part)
			clipPath := filepath.Join(s.outDir, clipName)
			// Crop 16:9 chính tâm thành 9:16 vertical cho TikTok/Reels/Shorts
			args := []string{
				"-ss", fmt.Sprintf("%.2f", start),
				"-i", final,
				"-t", fmt.Sprintf("%.2f", length),
				"-vf", "crop=ih*9/16:ih:(iw-ih*9/16)/2:0,scale=1080:1920",
				"-c:a", "copy",
				clipPath,
			}
			if err := ffmpegRun(ctx, args...); err == nil {
				aid := s.addAsset(id, 100+part, "short_clip", fmt.Sprintf("Clip %d (%.0fs, 9:16 TikTok/Reels)", part, length))
				s.setAsset(aid, StatusDone, clipPath)
				s.appendLog(id, fmt.Sprintf("✓ Đã xuất Clip %d: %s (%.0fs, 9:16)", part, clipName, length))
			} else {
				s.appendLog(id, fmt.Sprintf("Trích xuất Clip %d: %v (bỏ qua)", part, err))
			}
			part++
		}
	}

	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, fmt.Sprintf("Xong: %s (%.0fs, %d cảnh)", filepath.Base(final), dur, len(scenes)))
	s.fireOnDone(id)
}

// probeDuration trả về thời lượng giây của file media (0 khi lỗi).
// (Bản unexported: ProbeDuration của film_shots.go nằm sau build tag
// parked — không đụng vào để tránh trùng symbol khi build -tags parked.)
func storyProbeDuration(ctx context.Context, path string) float64 {
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

func firstLine(title, text string) string {
	if title != "" {
		return title
	}
	if i := strings.Index(text, "\n"); i > 0 {
		return text[:i]
	}
	if len(text) > 60 {
		return string([]rune(text)[:60]) + "…"
	}
	return text
}
