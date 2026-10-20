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
	"strconv"
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
		ID:              "plot-dem-khuya-01",
		Title:           "Đêm Mưa Bên Quán Trọ Cũ",
		Genre:           "tâm lý",
		GenreLabel:      "Radio Kể Chuyện Đêm Khuya",
		DurationMinutes: 45,
		Logline:         "Một lữ khách lỡ đường trong đêm bão táp tá túc tại quán trọ ven sông và nghe câu chuyện ân nghĩa cảm động.",
		Act1:            "Đêm mưa bão mù trời, người đàn ông ướt sũng bước vào quán trọ ven sông của bà cụ già và cô cháu gái ít nói.",
		Act2:            "Bên tách trà gừng nghi ngút khói, bà cụ kể về người khách quen 10 năm trước đã gửi lại chiếc hộp gỗ bí ẩn chưa từng quay lại.",
		Act3:            "Khi người đàn ông mở chiếc hộp, bức thư cũ và chiếc vòng bạc tiết lộ sự thật: ông lão gửi hộp chính là người cha bặt vô âm tín của anh.",
		Act4:            "Bà cụ run rẩy rút ra lá thư người cha gửi gắm trước lúc lâm chung, tha thứ mọi lỗi lầm giận dỗi tuổi trẻ của đứa con thơ dại.",
		Act5:            "Trong tiếng mưa rả rích đầu hiên vắng, giọt nước mắt hối hận hòa tan gánh nặng mười năm, khép lại câu chuyện tình phụ tử thiêng liêng.",
		Message:         "Ở đời, ân nghĩa và tình thân lớn hơn mọi sóng gió thời gian.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng mưa rơi nhẹ + Piano lofi acoustic",
	},
	{
		ID:              "plot-dem-khuya-02",
		Title:           "Chuyến Xe Buýt Cuối Cùng Trở Về Nhà",
		Genre:           "tâm lý",
		GenreLabel:      "Radio Kể Chuyện Đêm Khuya",
		DurationMinutes: 30,
		Logline:         "Những cuộc gặp gỡ định mệnh trên chuyến xe buýt đêm muộn đưa một chàng trai trẻ tìm lại ý nghĩa cuộc sống.",
		Act1:            "Chàng trai trẻ thất nghiệp ngồi gục đầu trên chuyến xe buýt tuyến cuối lúc 23h30 trong ngày đông giá rét.",
		Act2:            "Một ông lão bán vé số dạo bước lên xe, chia cho anh nửa củ khoai nướng nóng hổi và kể về chuỗi ngày cơ cực thời trẻ.",
		Act3:            "Người phụ nữ bế con nhỏ ngồi ghế sau cất tiếng hát ru ngọt ngào, làm dịu đi cái lạnh cắt da và không khí ngột ngạt trong khoang xe.",
		Act4:            "Khi xe ghé trạm cuối ngoại thành, người lái xe mỉm cười tặng anh tấm danh thiếp tuyển dụng của một xưởng mộc đang cần người học việc.",
		Act5:            "Bước xuống xe trong sương đêm, chàng trai nhận ra giữa giông bão cuộc đời, lòng tốt của những người xa lạ luôn là ngọn đèn soi lối.",
		Message:         "Đôi khi, một chút ấm áp từ người xa lạ lại đủ sức cứu rỗi cả một cuộc đời.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng còi xe xa xăm + Giai điệu mộc mạc",
	},
	{
		ID:              "plot-truyen-ma-01",
		Title:           "Tiếng Bước Chân Sau Bờ Tre Làng",
		Genre:           "kinh dị",
		GenreLabel:      "Truyện Ma Làng Quê",
		DurationMinutes: 45,
		Logline:         "Bí mật rùng rợn về ngôi mộ vô danh dưới gốc đa đầu làng và món nợ nhân quả của phú hộ xưa.",
		Act1:            "Đêm rằm tháng bảy, chàng sinh viên về quê chịu tang ông nội thì nghe tiếng bước chân dồn dập sau rặng tre làng lúc canh ba.",
		Act2:            "Người làng cảnh báo tuyệt đối không ai được bén mảng ra gốc đa cổ thụ sau nửa đêm, nơi từng có một nấm mồ vô danh.",
		Act3:            "Cuốn nhật ký cổ bằng chữ Nôm của ông nội để lại hé lộ bí mật kinh hoàng về món nợ máu của gia tộc phú hộ xưa cách đây một thế kỷ.",
		Act4:            "Cuộc đối mặt rợn gáy dưới ánh trăng mờ nhân ảnh và lễ tạ lỗi chân thành trước vong linh oan khuất bị chôn vùi trong bóng tối lịch sử.",
		Act5:            "Khi ánh bình minh ló rạng trên rặng tre ngả bóng vàng, oán khí tiêu tan, bài học nhân quả luân hồi để lại dư âm sâu sắc ngàn đời.",
		Message:         "Gieo nhân nào gặt quả nấy, sống ngay thẳng thì quỷ thần cũng phải kính nhường.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng gió rít đêm sương + Tiếng côn trùng vắng",
	},
	{
		ID:              "plot-truyen-ma-02",
		Title:           "Căn Nhà Bỏ Hoang Bên Bến Đò Cũ",
		Genre:           "kinh dị",
		GenreLabel:      "Truyện Ma Làng Quê",
		DurationMinutes: 30,
		Logline:         "Ngôi nhà gỗ mục nát ngã ba sông và bí ẩn về người lái đò không bao giờ nhận tiền của khách lỡ chuyến.",
		Act1:            "Đêm mưa gió mịt mùng, một vị khách lỡ độ đường ghé vào bến đò vắng và được người đàn ông bí ẩn lặng lẽ chèo thuyền đưa sang sông.",
		Act2:            "Người chèo đò đưa khách vào căn nhà gỗ mục nát sưởi ấm, kể về người thầy lang từng dốc hết gia tài cứu cả làng trong nạn đói năm xưa.",
		Act3:            "Nửa đêm tỉnh giấc, người khách nhận ra căn nhà hoàn toàn hoang phế, chiếc bếp lò đã nguội lạnh từ hàng chục năm.",
		Act4:            "Sáng hôm sau hỏi han các vị cao niên, người khách bàng hoàng khi biết người chèo đò và căn nhà ấy đã chìm vào dòng nước từ trận lũ 30 năm trước.",
		Act5:            "Tấm bia đá tri ân bên bờ sông khẳng định đạo lý: Tấm lòng nhân đức cứu người dẫu qua bao kiếp luân hồi vẫn tỏa sáng giữa nhân gian.",
		Message:         "Tấm lòng nhân ái thì dù qua bao kiếp luân hồi vẫn lưu lại phước báu nhân gian.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng nước róc rách + Tiếng chuông chùa vẳng xa",
	},
	{
		ID:              "plot-trinh-tham-01",
		Title:           "Bức Thư Bí Mật Trong Chiếc Đồng Hồ Cổ",
		Genre:           "trinh thám",
		GenreLabel:      "Kỳ Án Trinh Thám & Giải Mã",
		DurationMinutes: 60,
		Logline:         "Vụ mất tích bí ẩn của một giáo sư già và cuộc đấu trí nghẹt thở giữa viên thám tử và kẻ giấu mặt.",
		Act1:            "Giáo sư sử học qua đời bí ẩn tại biệt thự cổ kính, để lại chiếc đồng hồ quả lắc Thụy Sĩ đứng kim đúng 3 giờ 15 phút.",
		Act2:            "Thám tử tư lần theo các ký hiệu mật mã khắc trên bánh răng đồng hồ, phát hiện bản đồ dẫn đến bộ sưu tập cổ vật bị thất lạc thời chiến.",
		Act3:            "Những cái bẫy nguy hiểm trong hầm ngầm và cuộc rượt đuổi nghẹt thở với tổ chức buôn lậu đồ cổ xuyên quốc gia.",
		Act4:            "Cú twist lật màn khi hung thủ thực sự lại chính là người học trò xuất sắc nhất mà giáo sư từng hết lòng yêu quý và tin tưởng.",
		Act5:            "Bản di chúc thực sự được công bố trước tòa án, toàn bộ bảo vật được hiến tặng cho bảo tàng quốc gia, kết thúc vụ án kinh điển.",
		Message:         "Lưới trời lồng lộng, không tội ác nào có thể che giấu mãi mãi.",
		SuggestedVoice:  "default",
		Soundscape:      "Dramatic Suspense Ambient + Tiếng tích tắc đồng hồ",
	},
	{
		ID:              "plot-nhan-sinh-01",
		Title:           "Bát Cơm Nguội Của Người Mẹ Già",
		Genre:           "triết lý",
		GenreLabel:      "Bài Học Nhân Sinh & Đạo Lý",
		DurationMinutes: 30,
		Logline:         "Câu chuyện xúc động về sự hy sinh thầm lặng của người mẹ nghèo và bài học thức tỉnh người con thành đạt.",
		Act1:            "Sau 15 năm bôn ba làm ăn thành đạt nơi phố thị, người con trai lần đầu trở về căn nhà tranh ngói cũ ven đê thăm mẹ già tóc bạc.",
		Act2:            "Bữa cơm đạm bạc với bát cơm nguội mẹ nhường con ăn trước, còn mình lặng lẽ ăn khoai sắn bên góc bếp un khói.",
		Act3:            "Người con vô tình tìm thấy chiếc túi vải cũ kỹ cất dưới đáy rương, bên trong là từng đồng tiền lẻ thấm mồ hôi mẹ dành dụm gửi anh học đại học.",
		Act4:            "Cơn đột quỵ bất ngờ của mẹ trong đêm khiến người con hoảng loạn đưa mẹ đi cấp cứu, đối mặt với sự mong manh của kiếp người.",
		Act5:            "Giọt nước mắt muộn màng bên giường bệnh và lời thì thầm của mẹ nhắc nhở: Tiền tài danh vọng không bằng một bữa cơm đoàn viên bên gia đình.",
		Message:         "Đừng đợi đến khi cha mẹ khuất núi mới nhận ra điều quý giá nhất trên đời.",
		SuggestedVoice:  "female",
		Soundscape:      "Acoustic Guitar mộc mạc + Tiếng sáo ru êm đềm",
	},
	{
		ID:              "plot-thuong-truong-01",
		Title:           "Cú Lật Kèo Phút 89",
		Genre:           "kinh doanh",
		GenreLabel:      "Thương Trường & Khởi Nghiệp",
		DurationMinutes: 45,
		Logline:         "Từ bờ vực phá sản với khoản nợ hàng chục tỷ, chàng trai 28 tuổi lội ngược dòng ngoạn mục nhờ một nguyên tắc kinh doanh bất biến.",
		Act1:            "Công ty công nghệ đứng bên bờ vực phá sản khi đối tác chiến lược đột ngột đơn phương hủy hợp đồng trước hạn chót thanh toán 48 giờ.",
		Act2:            "Áp lực khủng khiếp từ ngân hàng và nhà cung ứng, đội ngũ sáng lập chia rẽ gay gắt giữa việc tuyên bố phá sản hay cầm cố nhà cửa.",
		Act3:            "Quyết định sống còn: CEO trẻ tuổi mở toàn bộ mã nguồn và công khai minh bạch tài chính để kêu gọi sự chung tay từ cộng đồng người dùng.",
		Act4:            "Sự ủng hộ bùng nổ từ hàng ngàn khách hàng trung thành thu hút sự chú ý của quỹ đầu tư mạo hiểm hàng đầu khu vực.",
		Act5:            "Thương vụ sáp nhập lịch sử được ký kết tại phút 89, khẳng định chân lý: Chữ Tín và giá trị thực sự cho cộng đồng là vũ khí mạnh nhất trong kinh doanh.",
		Message:         "Trong kinh doanh, chữ TÍN chính là tài sản quý giá nhất.",
		SuggestedVoice:  "default",
		Soundscape:      "Inspiring Ambient Beat + Nhịp điệu dồn dập",
	},
	{
		ID:              "plot-da-su-01",
		Title:           "Huyền Thoại Thanh Gươm Dưới Lòng Hồ",
		Genre:           "lịch sử",
		GenreLabel:      "Dã Sử Huyền Huyễn",
		DurationMinutes: 60,
		Logline:         "Câu chuyện dã sử hào hùng về người tráng sĩ vô danh đứng lên bảo vệ bờ cõi biên ải trước quân thù.",
		Act1:            "Đêm mùa đông buốt giá nơi ải bắc, tiếng tù và báo động giặc dữ tràn qua biên ải, đốt phá thôn làng bình yên.",
		Act2:            "Người tráng sĩ thợ rèn nhận lời ủy thác tìm lại thanh bảo kiếm gia truyền ngâm sâu dưới đáy hồ nước thần giữa rừng thiêng.",
		Act3:            "Vượt qua thử thách sinh tử trong hang đá ngầm và trận giao tranh khốc liệt chống lại toán thích khách tinh nhuệ.",
		Act4:            "Bảo kiếm xuất thế tỏa ánh hàn quang lẫm liệt, vị tráng sĩ hiệu triệu hào kiệt bốn phương tập hợp dưới cờ khởi nghĩa cứu dân.",
		Act5:            "Trận phục kích quyết định tại hẻm núi bẻ gãy ý chí quân xâm lược, thanh gươm được quy phục về lòng hồ, ngàn năm giữ yên cõi bờ.",
		Message:         "Tinh thần bảo vệ non sông ngàn đời là dòng máu chảy trong tim mỗi người con đất Việt.",
		SuggestedVoice:  "default",
		Soundscape:      "Đàn tranh + Tiếng trống trận hào hùng vang dội",
	},
	{
		ID:              "plot-tinh-cam-01",
		Title:           "Hẹn Ước Dưới Mưa Năm 17 Tuổi",
		Genre:           "tình cảm",
		GenreLabel:      "Tình Cảm Nam Nữ & Ngôn Tình Đô Thị",
		DurationMinutes: 45,
		Logline:         "Sau 8 năm hiểu lầm và xa cách, định mệnh sắp đặt cuộc hội ngộ cay đắng nhưng vỡ òa giữa giám đốc tập đoàn và cô thư ký từng là mối tình đầu thời cấp ba.",
		Act1:            "Chàng trai nghèo và cô tiểu thư từng có mối tình học trò đẹp như tranh vẽ dưới gốc phượng già trường huyện, trước khi biến cố gia đình khiến cô đột ngột biến mất không một lời từ biệt.",
		Act2:            "8 năm sau, anh trở thành tổng tài lạnh lùng quyền lực, còn cô xuất hiện trong buổi phỏng vấn thư ký với bộ dạng tiều tụy gánh trên vai món nợ viện phí cho mẹ.",
		Act3:            "Những lạnh lùng xa cách nơi công sở không che giấu được những quan tâm vụng về và ký ức thanh xuân chưa từng phai nhạt.",
		Act4:            "Bức thư tình bị thất lạc 8 năm trước được tìm thấy trong tập tài liệu cũ, hóa giải mọi hiểu lầm và tổn thương quá khứ.",
		Act5:            "Dưới cơn mưa đầu mùa bên hiên quán cũ, hai bàn tay lại tìm về nhau, viết tiếp cái kết viên mãn cho câu chuyện tình thanh xuân dang dở.",
		Message:         "Chân tình nếu đủ sâu đậm, dù đi một vòng bão giông rồi cũng sẽ tìm về đúng người.",
		SuggestedVoice:  "warm-female",
		Soundscape:      "Piano ballad lãng mạn + Tiếng mưa rơi êm dịu",
	},
	{
		ID:              "plot-tinh-cam-02",
		Title:           "Bức Tranh Chưa Kịp Đặt Tên",
		Genre:           "tình cảm",
		GenreLabel:      "Tình Cảm Nam Nữ & Ngôn Tình Đô Thị",
		DurationMinutes: 30,
		Logline:         "Mối tình ngọt ngào nhưng đầy trắc trở giữa chàng họa sĩ khiếm thị và cô gái làm việc tại phòng triển lãm tranh nghệ thuật.",
		Act1:            "Tại góc quán cà phê sách ven hồ Tây, cô gái tình cờ quen một chàng trai mù có tài vẽ tranh sơn dầu tuyệt mỹ qua cảm nhận xúc giác và mùi hương.",
		Act2:            "Những ngày tháng cùng nhau rong ruổi qua từng con phố Hà Nội đã nhen nhóm ngọn lửa tình yêu sâu sắc, nhưng căn bệnh hiểm nghèo khiến cô gái chọn cách âm thầm rút lui.",
		Act3:            "Khi anh được chỉ định phẫu thuật mắt có cơ hội phục hồi thị lực, cô gái phát hiện căn bệnh nan y của mình và lặng lẽ rời xa.",
		Act4:            "Ngày tháo băng mắt nhìn thấy ánh sáng, chàng trai bàng hoàng nhận ra bức tranh tuyệt mỹ cô gửi lại chính là chân dung của chính cô.",
		Act5:            "Cuộc hội ngộ nghẹn ngào tại vườn hoa cúc họa mi năm nào, tình yêu đích thực đã vượt qua mọi rào cản số phận để ở bên nhau trọn đời.",
		Message:         "Tình yêu đích thực không nhìn bằng mắt, mà thấu hiểu bằng cả trái tim.",
		SuggestedVoice:  "default",
		Soundscape:      "Acoustic Guitar + Violin da diết sâu lắng",
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

	// Danh sách câu Hook 3 giây giữ chân người nghe (25 mẫu, 5 nhóm thể loại)
	hooks := []string{
		// --- Mystery/Suspense (Bí ẩn / Hồi hộp) ---
		"Nửa đêm hôm ấy, khi tiếng sấm vừa rạch ngang bầu trời, một tiếng gõ cửa dồn dập vang lên làm rung chuyển cả căn nhà cũ...",
		"Có những bí mật nếu không được phơi bày trong đêm nay, có lẽ người ta sẽ mang theo cả mối ân oán xuống mồ...",
		"Người đàn ông lạ mặt bước vào quán lúc 2 giờ sáng, đặt lên bàn một bọc vải đẫm nước mưa và run rẩy nói một câu...",
		"Trước khi nhắm mắt, ông lão chỉ thì thầm đúng một câu khiến tất cả con cháu trong nhà đều chết lặng...",
		"Đúng 12 giờ đêm, chiếc chuông đồng cổ xưa bỗng nhiên tự gióng lên 3 hồi chuông lạnh toát sống lưng...",
		// --- Emotional/Touching (Cảm xúc / Chạm lòng) ---
		"Bà tôi nắm chặt tay tôi lần cuối và nói: 'Cháu à, có một điều bà giấu cháu suốt 30 năm nay...'",
		"Khi tôi mở chiếc hộp thiếc cũ kỹ mẹ để lại, bên trong là một bức thư chưa bao giờ được gửi đi...",
		"Người đàn ông ngồi khóc một mình trên ghế đá công viên lúc 4 giờ sáng. Tôi không biết rằng cuộc đời tôi sẽ thay đổi hoàn toàn kể từ khoảnh khắc ấy...",
		"Mẹ tôi chưa bao giờ kể về cha. Cho đến ngày hôm ấy, khi tôi tìm thấy tấm ảnh cũ nhàu nát trong túi áo bà...",
		"Tiếng khóc của đứa trẻ sơ sinh vang lên giữa đêm đông giá buốt ngoài cổng chùa. Sư thầy mở cửa và nhìn thấy một thứ khiến ngài chấn động...",
		// --- Horror/Creepy (Kinh dị / Rùng rợn) ---
		"Từ đêm đó, cứ đúng 3 giờ sáng, tiếng bước chân lại vang lên trên tầng áp mái. Vấn đề là... căn nhà này chỉ có một tầng...",
		"Người ta nói rằng ai đi ngang qua cây cầu đá cũ sau nửa đêm sẽ nghe thấy tiếng ru con. Tôi không tin, cho đến khi chính tôi nghe thấy...",
		"Con bé hàng xóm chỉ tay về góc tối và nói: 'Chú ơi, sao bà kia cứ đứng nhìn chú hoài vậy?' Tôi quay lại. Không có ai cả...",
		"Chiếc gương cổ trong căn biệt thự bỏ hoang không phản chiếu bóng tôi. Nó phản chiếu một người đàn bà mặc áo trắng đang đứng ngay sau lưng tôi...",
		"Đêm ấy gió rít từng cơn ngoài bờ tre, ông nội bảo tôi: 'Dù nghe thấy gì, tuyệt đối không được mở cửa.' Rồi tiếng gọi tên tôi vang lên, bằng chính giọng ông nội...",
		// --- Business/Success (Kinh doanh / Thành công) ---
		"Hôm ấy tôi đứng trước tòa nhà công ty với đúng 50 ngàn trong túi và một khoản nợ 3 tỷ đồng. Không ai tin rằng 2 năm sau, mọi thứ sẽ lật ngược hoàn toàn...",
		"Đối tác lớn nhất hủy hợp đồng ngay trước ngày ký kết. Cả công ty nhìn tôi chờ đợi. Và tôi đã đưa ra quyết định điên rồ nhất đời mình...",
		"Người đàn ông bán vé số dạo năm ấy đã dạy tôi một bài học mà cả Harvard cũng không có trong giáo trình...",
		"Tôi bị sa thải đúng ngày sinh nhật tuổi 30. Nhưng đó lại là ngày đẹp nhất cuộc đời tôi. Để tôi kể cho bạn nghe vì sao...",
		"Khi cả thế giới quay lưng, chính kẻ thù cũ lại chìa tay ra cứu tôi. Và cái giá phải trả cho lòng tốt ấy... không hề rẻ...",
		// --- Romance/Love (Tình cảm / Ngôn tình) ---
		"Anh ấy bước vào đời tôi đúng lúc tôi đã quyết định không bao giờ yêu ai nữa. Và mọi thứ bắt đầu từ một chiếc ô trong cơn mưa tầm tã...",
		"Tôi và anh hẹn gặp lại sau 5 năm tại quán cà phê góc phố cũ. Anh đến đúng giờ. Nhưng bên cạnh anh là một người phụ nữ khác...",
		"Bức thư tình cuối cùng anh viết cho tôi chỉ có đúng 8 chữ. Nhưng 8 chữ ấy đã khiến tôi khóc suốt cả đêm...",
		"Cô ấy là người yêu cũ của anh trai tôi. Và tôi... đã yêu cô ấy từ cái nhìn đầu tiên mà không dám nói...",
		"Đám cưới sắp diễn ra trong 3 ngày nữa. Nhưng tối hôm đó, tôi nhận được tin nhắn từ một số lạ: 'Em xin lỗi, chú rể không phải là người anh nghĩ đâu...'",
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
		case "tình cảm", "tinh_cam", "ngôn tình":
			title = "Hẹn Ước Dưới Mưa Năm 17 Tuổi - Mối Tình Đầu Định Mệnh"
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
	case "tình cảm", "tinh_cam", "ngôn tình":
		genreLabel = "Tình Cảm Nam Nữ & Ngôn Tình Đô Thị"
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

	if p.DurationMinutes > 0 {
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
	} else {
		if p.Words < 200 {
			p.Words = 200
		}
		if p.Words > 1200 {
			p.Words = 1200
		}
		if p.Scenes < 3 {
			p.Scenes = 3
		}
		if p.Scenes > 12 {
			p.Scenes = 12
		}
	}

	title := "Kể chuyện: " + strings.TrimSpace(p.Topic)
	if p.DurationMinutes > 0 {
		title = fmt.Sprintf("Kể chuyện [%dp]: %s", p.DurationMinutes, strings.TrimSpace(p.Topic))
	}
	if r := []rune(title); len(r) > 85 {
		title = string(r[:85]) + "…"
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

// defaultDramaticStory tạo cốt truyện tình cảm kịch tính chuẩn cho trường hợp LLM timeout hoặc quota bận.
// Xoay vòng 3 template khác nhau theo genre: kinh dị, trinh thám, hoặc tình cảm mặc định.
func defaultDramaticStory(topic, genre string) *storyDoc {
	title := strings.TrimSpace(topic)
	genreLower := strings.ToLower(strings.TrimSpace(genre))

	// Template 1: Kinh dị — truyện ma làng quê
	if strings.Contains(genreLower, "kinh dị") {
		if title == "" {
			title = "Tiếng Gọi Từ Giếng Cổ: Bí Mật Rùng Rợn Của Ngôi Làng Bỏ Hoang"
		}
		text := `Tôi trở về ngôi làng cũ sau mười năm xa cách. Con đường đất đỏ vẫn vậy, nhưng không khí hoang vắng đến lạ thường. Không một bóng người, không tiếng chó sủa, chỉ có tiếng gió rít qua kẽ lá tre nghe như tiếng thở dài ai oán.

Bà ngoại tôi từng dặn: "Đừng bao giờ lại gần giếng cổ sau nửa đêm." Tôi không hiểu vì sao, cho đến khi đêm ấy, lúc 2 giờ sáng, tôi bị đánh thức bởi tiếng gọi tên mình vọng lên từ lòng giếng. Giọng nói ấy quen thuộc đến rợn người — nó giống hệt giọng bà ngoại tôi, người đã mất cách đây 3 năm.

Tôi run rẩy bước ra sân, ánh trăng rọi xuống miệng giếng cổ phủ rêu xanh. Nhìn xuống, tôi thấy mặt nước đen ngòm phản chiếu khuôn mặt tôi. Nhưng bóng phản chiếu ấy không cử động theo tôi — nó mỉm cười. Chính lúc đó, bàn tay lạnh buốt từ phía sau nắm chặt vai tôi. Sáng hôm sau, người ta tìm thấy cuốn nhật ký cũ của bà ngoại trong đáy giếng, hé lộ bí mật về lời nguyền ba đời và cách hóa giải duy nhất trước khi bình minh ló rạng.`

		return &storyDoc{Title: title, Text: text}
	}

	// Template 2: Trinh thám — vụ án bí ẩn
	if strings.Contains(genreLower, "trinh thám") {
		if title == "" {
			title = "Bức Di Thư Mật Mã: Vụ Án 20 Năm Chưa Có Lời Giải"
		}
		text := `Tôi nhận được cuộc gọi lúc 3 giờ sáng. Giọng nói bên kia đường dây lạnh như thép: "Anh có đúng 72 giờ để giải mã bức di thư này, nếu không... người tiếp theo biến mất sẽ là người anh yêu thương nhất."

Tôi là thám tử tư, đã nghỉ hưu non sau vụ án ám ảnh nhất sự nghiệp: cô giáo Hạnh mất tích không dấu vết 20 năm trước. Không xác, không manh mối, không nghi phạm. Vụ án bị xếp vào hồ sơ lạnh. Cho đến khi bức di thư của ông Trần — người hiệu trưởng vừa qua đời — được tìm thấy trong ngăn kéo bí mật, với những dòng mật mã mà chỉ tôi mới có chìa khóa giải.

Từng mảnh ghép dần lộ ra: nhân chứng giả, bằng chứng bị tiêu hủy có hệ thống, và một mạng lưới che đậy tinh vi đến mức rùng mình. Kẻ thủ ác không phải người xa lạ — hắn ngồi ngay bên cạnh tôi suốt 20 năm, mỉm cười và gọi tôi là bạn. Cuộc đấu trí nghẹt thở trên chuyến tàu đêm cuối cùng đã phơi bày tất cả, nhưng cái giá phải trả cho sự thật... đắt hơn tôi tưởng rất nhiều.`

		return &storyDoc{Title: title, Text: text}
	}

	// Template 3: Tình yêu / Thanh xuân / Ngôn tình (Anime thẩm mỹ cao)
	if strings.Contains(genreLower, "tình yêu") || strings.Contains(genreLower, "ngôn tình") || strings.Contains(genreLower, "thanh xuân") || strings.Contains(strings.ToLower(title), "tình yêu") || strings.Contains(strings.ToLower(title), "hẹn ước") {
		if title == "" {
			title = "Hẹn Ước Dưới Mưa: Mối Tình Thanh Xuân Vườn Trường Không Thể Quên"
		}
		text := `Cơn mưa rào đầu hạ bất chợt đổ xuống sân trường cấp ba rực rỡ hoa phượng đỏ. Tôi và cậu ấy cùng đứng trú mưa dưới mái hiên dãy hành lang quen thuộc. Cậu ấy khẽ nghiêng chiếc ô về phía tôi, bờ vai ướt đẫm nước mưa nhưng nụ cười vẫn ấm áp như ánh mặt trời ban mai.

Những năm tháng thanh xuân cùng nhau học bài, cùng đạp xe qua từng con phố rợp bóng cây xanh đã khắc sâu vào ký ức không thể nào quên. Cậu ấy luôn âm thầm ghi nhớ từng thói quen nhỏ của tôi, lặng lẽ đi bên cạnh bảo vệ tôi qua những ngày giông bão tuổi trẻ.

Ngày bế giảng chia tay, dưới tán bằng lăng tím ngát trong ánh hoàng hôn rực rỡ, cậu ấy trao cho tôi lá thư hẹn ước và thì thầm lời tỏ tình ngọt ngào nhất cuộc đời: "Dù mai này trưởng thành đi xa đến đâu, vị trí bên cạnh cậu mãi mãi chỉ dành cho tớ." Khoảnh khắc ấy, tôi biết rằng thanh xuân này đã trọn vẹn vì có cậu.`
		return &storyDoc{Title: title, Text: text}
	}

	// Template 4 (mặc định): Tình cảm — hôn nhân hợp đồng
	if title == "" {
		title = "Hôn Nhân Hợp Đồng: Sự Thật Đằng Sau Người Chồng Hoàn Hảo"
	}
	text := fmt.Sprintf(`Tôi và anh ký vào bản hợp đồng hôn nhân thời hạn hai năm trong một buổi chiều mưa tầm tã tại văn phòng luật sư. Với mọi người ngoài kia, anh là người chồng hoàn hảo: giàu có, lịch thiệp, yêu thương vợ hết mực. Nhưng chỉ tôi mới biết, đằng sau cánh cửa căn penthouse xa hoa, chúng tôi chỉ là hai người xa lạ cùng diễn một vở kịch hoàn hảo.

Thời gian trôi qua, những cử chỉ quan tâm bất ngờ của anh bắt đầu làm trái tim tôi rung động. Anh nhớ từng sở thích nhỏ, luôn xuất hiện mỗi khi tôi gặp khó khăn. Cho đến một ngày, tôi tình cờ phát hiện ra căn phòng bí mật ở tầng áp mái. Trong đó là tất cả tài liệu về gia đình tôi năm xưa — vụ tai nạn đã cướp đi cha mẹ tôi, và anh chính là người âm thầm bảo vệ tôi suốt mười năm qua mà không một lời oán thán.

Khoảnh khắc kẻ thù năm xưa tìm đến đe dọa mạng sống của tôi, anh đã không ngần ngại lấy thân mình che chắn. Giữa lằn ranh sinh tử trong cơn mưa xối xả, anh nắm chặt tay tôi và thì thầm: "Hợp đồng đã kết thúc từ lâu, nhưng tình yêu của anh dành cho em là cả đời này." Giây phút ấy, tôi nhận ra bản hợp đồng hôn nhân không phải là cái bẫy, mà là cách duy nhất anh có thể đường hoàng bước vào và che chở cho cuộc đời tôi.`)

	return &storyDoc{
		Title: title,
		Text:  text,
	}
}

// defaultDramaticScenes tạo danh sách cảnh điện ảnh kịch tính chuẩn 9:16 vertical theo thể loại.
func defaultDramaticScenes(doc *storyDoc) []storyScene {
	titleLower := strings.ToLower(doc.Title)
	if strings.Contains(titleLower, "tình yêu") || strings.Contains(titleLower, "hẹn ước") || strings.Contains(titleLower, "thanh xuân") || strings.Contains(titleLower, "ngôn tình") || strings.Contains(titleLower, "học đường") {
		return []storyScene{
			{
				Text: "Cơn mưa rào đầu hạ bất chợt đổ xuống sân trường cấp ba rực rỡ hoa phượng đỏ. Tôi và cậu ấy cùng đứng trú mưa dưới mái hiên dãy hành lang quen thuộc.",
				ImagePrompt: "Breathtaking Makoto Shinkai anime style vertical illustration 9:16, vibrant high school campus hallway in summer rain, a handsome anime boy with dark hair in crisp white school uniform holding a clear umbrella, standing next to a beautiful cute anime girl with long silky hair blushing softly, sparkling rain droplets falling, lush green trees and pink blossoms in background, emotional nostalgic anime masterpiece, vibrant colors, 8k",
			},
			{
				Text: "Cậu ấy khẽ nghiêng chiếc ô về phía tôi, bờ vai ướt đẫm nước mưa nhưng nụ cười vẫn ấm áp như ánh mặt trời ban mai.",
				ImagePrompt: "Makoto Shinkai anime style vertical 9:16, cinematic close-up of the handsome anime boy gently tilting his umbrella over the cute anime girl, gentle affectionate smile on his face, rain mist glowing in golden ambient light, high school romance aesthetic, hyper-detailed anime art",
			},
			{
				Text: "Những năm tháng thanh xuân cùng nhau học bài, cùng đạp xe qua từng con phố rợp bóng cây xanh đã khắc sâu vào ký ức không thể nào quên.",
				ImagePrompt: "Stunning anime style vertical 9:16, late afternoon golden hour sunset, the anime boy riding a bicycle with the cute anime girl sitting in the back holding his waist, joyful smiles, wind blowing through hair, scenic riverside road lined with blossoming trees, beautiful anime aesthetic",
			},
			{
				Text: "Ngày bế giảng chia tay, dưới tán bằng lăng tím ngát, cậu ấy trao cho tôi lá thư hẹn ước và lời tỏ tình ngọt ngào nhất cuộc đời.",
				ImagePrompt: "Emotional Makoto Shinkai anime climax scene vertical 9:16, graduation day under blooming purple royal poinciana trees, sunset sky with pastel clouds, the anime boy confessing his love to the anime girl, tears of joy, holding hands, romantic anime masterpiece, 8k",
			},
		}
	}
	return []storyScene{
		{
			Text: "Tôi và anh ký vào bản hợp đồng hôn nhân thời hạn hai năm trong một buổi chiều mưa tầm tã tại văn phòng luật sư xa hoa.",
			ImagePrompt: "Cinematic vertical portrait illustration 9:16 of a luxurious law office interior, rain pouring outside the floor-to-ceiling windows, a young beautiful Vietnamese woman in elegant white blazer and a handsome businessman in dark navy suit signing legal documents on a mahogany desk, dramatic moody lighting, 8k masterpiece",
		},
		{
			Text: "Với mọi người ngoài kia, anh là người chồng hoàn hảo, nhưng đằng sau cánh cửa căn penthouse xa hoa, chúng tôi chỉ là hai người xa lạ cùng diễn một vở kịch.",
			ImagePrompt: "Modern high-end penthouse living room overlooking city skyline at dusk, vertical 9:16 composition, cold emotional distance between the beautiful Vietnamese woman and handsome businessman standing apart, warm ambient interior light contrasting cold twilight outside",
		},
		{
			Text: "Thời gian trôi qua, những cử chỉ chăm sóc thầm lặng và ánh mắt ấm áp của anh bắt đầu khiến trái tim tôi rung động.",
			ImagePrompt: "A cozy modern kitchen at night, vertical 9:16 framing, soft warm lamplight, the same handsome Vietnamese businessman gently offering a warm cup of tea to the pensive beautiful woman, tender and emotional atmosphere",
		},
		{
			Text: "Một đêm nọ, tôi phát hiện căn phòng bí mật ở tầng áp mái chứa toàn bộ sự thật về vụ tai nạn năm xưa và sự bảo vệ âm thầm suốt mười năm qua của anh.",
			ImagePrompt: "A secret attic room lit by a desk lamp, vertical 9:16 framing, old photo albums and archived documents on a wooden table, the same woman discovering heartfelt secrets in astonishment and tearful eyes, dramatic cinematic composition",
		},
		{
			Text: "Kẻ thù tìm đến đe dọa, giữa lằn ranh sinh tử trong đêm mưa bão, anh đã lao ra lấy thân mình che chở cho tôi.",
			ImagePrompt: "Dramatic intense confrontation under pouring heavy rain at night on an empty city street, vertical 9:16 composition, headlights casting sharp shadows, the handsome man bravely shielding the woman, high tension cinematic drama",
		},
		{
			Text: "Anh nắm chặt tay tôi và nói: Bản hợp đồng đã kết thúc từ lâu, nhưng tình yêu của anh dành cho em là mãi mãi.",
			ImagePrompt: "Heartfelt romantic emotional climax in the aftermath of rain, vertical 9:16 framing, wet asphalt reflecting streetlights, the couple embracing closely with tearful loving expressions, breathtaking cinematic lighting",
		},
	}
}

// generateStorySilenceWav tạo track audio chuyển cảnh dự phòng chuẩn PCM 24kHz mono.
func generateStorySilenceWav(ctx context.Context, dst string, durationSec float64) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=24000:cl=mono:d=%.2f", durationSec),
		"-acodec", "pcm_s16le", dst)
	return cmd.Run()
}

// writeStory dùng LLM viết truyện ngôi thứ nhất (tái dùng story.txt), có cơ chế phục hồi tự động.
func (s *Studio) writeStory(ctx context.Context, p StoryParams) (*storyDoc, error) {
	storyWords := p.Words
	if storyWords <= 0 {
		storyWords = 1500
	}
	if storyWords > 1800 {
		storyWords = 1800
	}
	if s.llm != nil {
		prompt, err := directorPrompt("story.txt", promptData{
			Topic: p.Topic, Genre: p.Genre, StoryWords: storyWords,
			Orient: "Viết ở NGÔI THỨ NHẤT (xưng \"tôi\") từ đầu đến cuối, tình tiết lôi cuốn, cao trào cảm xúc",
			Seconds: storyWords / 3,
		})
		if err == nil {
			cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
			text, err := s.llm.Complete(cctx,
				"Bạn là nhà văn Việt Nam. Chỉ trả lời JSON thuần, không giải thích.", prompt)
			ccancel()
			if err == nil {
				var doc storyDoc
				if perr := parseDirectorJSON(text, &doc); perr == nil {
					doc.Title = strings.TrimSpace(doc.Title)
					doc.Text = strings.TrimSpace(doc.Text)
					if doc.Text != "" {
						return &doc, nil
					}
				}
			}
		}
	}
	return defaultDramaticStory(p.Topic, p.Genre), nil
}

// splitScenes chia truyện thành các cảnh (đoạn văn + prompt ảnh).
func (s *Studio) splitScenes(ctx context.Context, p StoryParams, doc *storyDoc) ([]storyScene, error) {
	if s.llm != nil {
		visualStyle := "Masterclass cinematic digital painting, highly detailed 8k, photorealistic, Vietnamese context if applicable, no text, no watermark."
		if strings.Contains(strings.ToLower(p.Genre), "tình yêu") || strings.Contains(strings.ToLower(p.Genre), "ngôn tình") || strings.Contains(strings.ToLower(p.Topic), "tình yêu") || strings.Contains(strings.ToLower(p.Genre), "anime") {
			visualStyle = "Makoto Shinkai & CoMix Wave anime aesthetic, stunning detailed romantic anime art, beautiful anime couple with consistent appearance, emotional cinematic lighting, vibrant atmospheric depth, 8k resolution, no text, no watermark."
		}

		sys := "Bạn là biên tập storyboard kiêm đạo diễn hình ảnh điện ảnh Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
		prompt := fmt.Sprintf(`Chia truyện sau thành đúng %d cảnh theo trình tự thời gian cho video kể chuyện 9:16 chuẩn 5 sao.
Mỗi cảnh gồm:
1. text: 1 đoạn văn kể ở ngôi thứ nhất giàu cảm xúc, ngắt nhịp tự nhiên theo hơi thở (40–120 từ).
2. image_prompt: 1 prompt tiếng Anh chi tiết cho AI vẽ ảnh minh họa điện ảnh 9:16 tuyệt đẹp:
   - Tỷ lệ khung hình: 9:16 vertical portrait composition, framed vertically.
   - Tính nhất quán nhân vật (Character Consistency): Thống nhất ngoại hình nhân vật chính xuyên suốt các cảnh (tuổi, trang phục, gương mặt, màu tóc).
   - Cỡ cảnh & góc máy: Đa dạng giữa Wide shot (bối cảnh bao la), Medium shot (tương tác), Close-up (chi tiết biểu cảm nhân vật).
   - Ánh sáng & không khí: Cinematic lighting, volumetric light, moody chiaroscuro hoặc warm ambient, atmospheric depth.
   - Phong cách thị giác: %s

Truyện: %s

Chỉ trả JSON: {"scenes": [{"text": "...", "image_prompt": "..."}, ...]}`,
			p.Scenes, visualStyle, doc.Text)
		cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
		text, err := s.llm.Complete(cctx, sys, prompt)
		ccancel()
		if err == nil {
			var raw struct {
				Scenes []storyScene `json:"scenes"`
			}
			if perr := parseDirectorJSON(text, &raw); perr == nil && len(raw.Scenes) >= 3 {
				var scenes []storyScene
				for _, sc := range raw.Scenes {
					sc.Text = strings.TrimSpace(sc.Text)
					sc.ImagePrompt = strings.TrimSpace(sc.ImagePrompt)
					if sc.Text == "" || sc.ImagePrompt == "" {
						continue
					}
					scenes = append(scenes, sc)
				}
				if len(scenes) >= 3 {
					if len(scenes) > 12 {
						scenes = scenes[:12]
					}
					return scenes, nil
				}
			}
		}
	}
	return defaultDramaticScenes(doc), nil
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
		"-c:a", "aac", "-b:a", "192k", outPath)
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

	if s.mg == nil {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu cấu hình tạo ảnh)"))
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

	// Tự động chuẩn bị nhạc nền nếu bật MusicOn mà chưa chọn file cụ thể
	if p.MusicOn && (strings.TrimSpace(p.MusicPath) == "" || !fileExists(p.MusicPath)) {
		musicCat := p.Genre
		if musicCat == "" {
			musicCat = "story"
		}
		if bg := FindTrendingMusic(musicCat); bg != "" {
			p.MusicPath = bg
			s.appendLog(id, fmt.Sprintf("✓ Tự động chọn nhạc nền phù hợp (%s): %s", musicCat, filepath.Base(bg)))
		}
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
	anchor := NewVisualAnchor(p.Topic, p.Genre, "", "")
	s.appendLog(id, fmt.Sprintf("Visual Anchor Matrix: Khóa tạo hình nhân vật (%s, %s) và bối cảnh (%s) xuyên suốt câu chuyện", anchor.ActorName, anchor.FaceTraits, anchor.Environment))

	var storyRefs []ImageRef
	var clips []string
	var wavs []string
	for i, sc := range scenes {
		aid := s.addAsset(id, i, "photo", sc.ImagePrompt)
		s.setAsset(aid, StatusRunning, "")

		img := filepath.Join(work, fmt.Sprintf("scene-%02d.png", i))
		imgPrompt := anchor.EnrichScenePrompt(sc.ImagePrompt, "slow cinematic drift", i+1, len(scenes))
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
			s.appendLog(id, fmt.Sprintf("Vẽ ảnh cảnh %d/%d (khóa nhân vật & bối cảnh)…", i+1, len(scenes)))
			if err := s.mg.GenerateImage(ctx, imgPrompt, storyRefs, img); err != nil {
				s.setAsset(aid, StatusFailed, "")
				fail(fmt.Errorf("cảnh %d: vẽ ảnh lỗi (%v) — dừng, không dựng thiếu cảnh", i+1, err))
				return
			}
			if i == 0 && fileExists(img) {
				anchor.MasterPhotoPath = img
				storyRefs = append(storyRefs, ImageRef{Path: img})
				s.appendLog(id, "✓ Đã khóa Visual Anchor Cảnh 1 làm mốc nhân vật & bối cảnh chuẩn cho toàn bộ truyện")
			}
		}

		s.appendLog(id, fmt.Sprintf("Đọc giọng cảnh %d/%d…", i+1, len(scenes)))
		wav := filepath.Join(work, fmt.Sprintf("scene-%02d.wav", i))
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
			}
		}
		dur := storyProbeDuration(ctx, wav)
		if dur < 1 {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: audio giọng đọc quá ngắn (%.1fs)", i+1, dur))
			return
		}
		wavs = append(wavs, wav)

		// Clip Ken Burns cho cảnh (tái dùng AssemblePhotoList, khổ 9:16).
		clip := filepath.Join(work, fmt.Sprintf("scene-%02d.mp4", i))
		if err := AssemblePhotoList(ctx, []string{img}, dur, "", 0, clip, "9:16"); err != nil {
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
	if err := ConcatClips(ctx, clips, "9:16", videoOnly); err != nil {
		fail(fmt.Errorf("nối cảnh: %w", err))
		return
	}
	s.appendLog(id, "Ghép giọng đọc…")
	voice := filepath.Join(work, "voice.m4a")
	if err := concatWAVs(ctx, wavs, voice); err != nil {
		fail(fmt.Errorf("nối giọng đọc: %w", err))
		return
	}
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	targetSec := float64(p.DurationMinutes * 60)

	if p.DurationMinutes >= 15 && targetSec > 60 {
		s.appendLog(id, fmt.Sprintf("Mở rộng trường thiên %d phút (%.0fs) chuẩn YouTube...", p.DurationMinutes, targetSec))
		vDur := storyProbeDuration(ctx, videoOnly)
		if vDur <= 0 {
			vDur = 30
		}
		loops := int(targetSec/vDur) + 1
		longVideoOnly := filepath.Join(work, "long-video-only.mp4")
		if err := ffmpegRun(ctx, "-stream_loop", strconv.Itoa(loops), "-i", videoOnly, "-t", fmt.Sprintf("%.2f", targetSec), "-c", "copy", longVideoOnly); err != nil {
			longVideoOnly = videoOnly
		}

		if p.MusicOn && strings.TrimSpace(p.MusicPath) != "" && fileExists(p.MusicPath) {
			s.appendLog(id, "Lồng nhạc nền & soundscape trường thiên...")
			longAudio := filepath.Join(work, "long-audio.m4a")
			filter := fmt.Sprintf("[1:a]aloop=loop=-1:size=2e+09,atrim=0:%.2f,volume=0.25[bg];[0:a]apad=pad_dur=%.2f,atrim=0:%.2f[sp];[sp][bg]amix=inputs=2:duration=first:dropout_transition=0,volume=1.2,loudnorm=I=-14:TP=-1.5:LRA=11[aout]", targetSec, targetSec, targetSec)
			if err := ffmpegRun(ctx, "-i", voice, "-i", p.MusicPath, "-filter_complex", filter, "-map", "[aout]", "-c:a", "aac", "-b:a", "192k", "-t", fmt.Sprintf("%.2f", targetSec), longAudio); err == nil {
				if err := ffmpegRun(ctx, "-i", longVideoOnly, "-i", longAudio, "-c:v", "copy", "-c:a", "copy", "-t", fmt.Sprintf("%.2f", targetSec), final); err != nil {
					fail(err)
					return
				}
			} else {
				s.appendLog(id, "Nhạc nền lỗi ("+err.Error()+"), ghép audio gốc")
				longVoice := filepath.Join(work, "long-voice.m4a")
				_ = ffmpegRun(ctx, "-i", voice, "-af", fmt.Sprintf("apad=pad_dur=%.2f", targetSec), "-c:a", "aac", "-b:a", "160k", "-t", fmt.Sprintf("%.2f", targetSec), longVoice)
				if err := ffmpegRun(ctx, "-i", longVideoOnly, "-i", longVoice, "-c:v", "copy", "-c:a", "copy", "-t", fmt.Sprintf("%.2f", targetSec), final); err != nil {
					fail(err)
					return
				}
			}
		} else {
			longVoice := filepath.Join(work, "long-voice.m4a")
			_ = ffmpegRun(ctx, "-i", voice, "-af", fmt.Sprintf("apad=pad_dur=%.2f", targetSec), "-c:a", "aac", "-b:a", "160k", "-t", fmt.Sprintf("%.2f", targetSec), longVoice)
			if err := ffmpegRun(ctx, "-i", longVideoOnly, "-i", longVoice, "-c:v", "copy", "-c:a", "copy", "-t", fmt.Sprintf("%.2f", targetSec), final); err != nil {
				fail(err)
				return
			}
		}
	} else {
		withAudio := filepath.Join(work, "with-audio.mp4")
		if err := ffmpegRun(ctx, "-i", videoOnly, "-i", voice,
			"-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac",
			"-shortest", withAudio); err != nil {
			fail(fmt.Errorf("ghép audio: %w", err))
			return
		}
		// Không chèn chữ vào video (Ninh chốt 2026-10-12): không phụ đề.
		subbed := withAudio
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
	}

	// 6. QC: Chuẩn 9:16 (1080x1920 hoặc 4K 2160x3840) + có audio + dài > 30s.
	s.appendLog(id, "Kiểm tra chất lượng…")
	w, h := ProbeDims(ctx, final)
	if (w != 1080 && w != 2160 && w != 1920 && w != 3840) || (h != 1920 && h != 3840 && h != 1080 && h != 2160) {
		fail(fmt.Errorf("QC: khổ video %dx%d, cần 9:16 (1080x1920 hoặc 4K 2160x3840)", w, h))
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
	s.appendLog(id, fmt.Sprintf("QC đạt: 9:16 vertical (%dx%d), có audio, %.0fs, %d cảnh", w, h, dur, len(scenes)))

	// Master 4K Delivery: Nâng cấp phim audio trường thiên lên Master 4K UHD
	if IsMasterQuality4K() && fileExists(final) {
		s.appendLog(id, "✨ Xuất bản phim Audio trường thiên chuẩn Master 4K UHD (2160x3840 VideoToolbox 35 Mbps)...")
		story4k := filepath.Join(work, "master_story_4k.mp4")
		if err := UpscaleVideoTo4K(ctx, final, story4k, "9:16"); err == nil && fileExists(story4k) {
			_ = os.Rename(story4k, final)
			s.appendLog(id, "✓ Phim Audio đạt chuẩn điện ảnh 4K Ultra HD sắc nét tuyệt đối!")
		}
	}

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
			if part > 3 {
				break
			}
			length := clipDur
			if start+length > dur {
				length = dur - start
			}
			if length < 15 {
				break
			}
			clipName := fmt.Sprintf("studio-%s-clip%d.mp4", id, part)
			clipPath := filepath.Join(s.outDir, clipName)
			// Trích xuất clip 9:16 vertical cho TikTok/Reels/Shorts (-c copy nhanh & giữ nguyên chất lượng)
			args := []string{
				"-ss", fmt.Sprintf("%.2f", start),
				"-i", final,
				"-t", fmt.Sprintf("%.2f", length),
				"-c", "copy",
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
	if r := []rune(text); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return text
}
