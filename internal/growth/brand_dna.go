package growth

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// BrandDNA represents the authentic brand identity, audience persona,
// and quality standards for an account/channel. AI Creator OS uses this profile
// to produce original content without relying on third-party reuploads.
type BrandDNA struct {
	AccountID       int64    `json:"account_id"`
	Positioning     string   `json:"positioning"`       // Định vị kênh (e.g. Chuyên gia review mỹ phẩm trung thực)
	AudiencePersona string   `json:"audience_persona"`  // Chân dung khán giả (tuổi, giới tính, nỗi đau, mong muốn)
	ValuePromise    string   `json:"value_promise"`     // Lời hứa giá trị cốt lõi
	ToneOfVoice     string   `json:"tone_of_voice"`     // Giọng điệu (chân thành, sắc sảo, hóm hỉnh, kịch tính...)
	HookRules       []string `json:"hook_rules"`        // Nguyên tắc mở đầu 3s (câu hỏi ngược đời, con số gây sốc...)
	ContentPillars  []string `json:"content_pillars"`   // 3-5 trụ cột nội dung đặc trưng
	VisualStyle     string   `json:"visual_style"`      // Phong cách hình ảnh (ánh sáng, nhịp cắt, màu sắc chủ đạo)
	AudioStyle      string   `json:"audio_style"`       // Phong cách âm thanh & giọng đọc (nhạc nền, âm lượng, tốc độ)
	RedLines        []string `json:"red_lines"`         // Ranh giới đỏ & từ cấm chính sách
	QCChecklist     []string `json:"qc_checklist"`      // Tiêu chuẩn nghiệm thu video nguyên bản (10 tiêu chí)
	UpdatedAt       string   `json:"updated_at"`
}

// DefaultBrandDNA returns an industry-standard Brand DNA template tailored
// to a niche and account ID.
func DefaultBrandDNA(niche string, accountID int64) *BrandDNA {
	now := time.Now().Format("2006-01-02 15:04:05")
	nl := strings.ToLower(niche)

	dna := &BrandDNA{
		AccountID: accountID,
		UpdatedAt: now,
		QCChecklist: []string{
			"Hook 3s đầu kích thích tò mò hoặc đánh đúng nỗi đau khán giả",
			"Mật độ giá trị thông tin cao (Value-per-second > 80%)",
			"100% nội dung nguyên bản — không sao chép nguyên trạng video Douyin/TikTok",
			"Hình ảnh & đồ họa rõ nét, tỷ lệ 9:16 chuẩn vùng an toàn TikTok/Shorts",
			"Âm thanh giọng đọc rõ ràng, nhạc nền lofi/ambient cân bằng chuẩn -14 LUFS",
			"Không chứa từ cấm hoặc tuyên bố sai sự thật vi phạm chính sách nền tảng",
			"Có CTA (kêu gọi hành động) tự nhiên, gắn link bio hoặc giỏ hàng hợp lệ",
			"Tông giọng và ngôn ngữ nhất quán với bản sắc thương hiệu đã định vị",
			"Nhịp dựng dứt khoát, cắt bỏ toàn bộ khoảng lặng thừa (dead air)",
			"Mỗi video giải quyết trọn vẹn 1 vấn đề cụ thể của người xem",
		},
	}

	switch {
	case strings.Contains(nl, "mỹ phẩm") || strings.Contains(nl, "skincare") || strings.Contains(nl, "làm đẹp") || strings.Contains(nl, "beauty"):
		dna.Positioning = "Chuyên gia bóc tách thành phần mỹ phẩm & hướng dẫn chăm sóc da khoa học, nói thẳng sự thật"
		dna.AudiencePersona = "Nữ 18-35 tuổi, dân văn phòng & sinh viên, quan tâm mụn/thâm/lão hóa, sợ mua nhầm hàng giả hoặc kem trộn"
		dna.ValuePromise = "Giúp bạn hiểu đúng làn da và chọn đúng sản phẩm hiệu quả trong 60 giây, tiết kiệm hàng triệu đồng"
		dna.ToneOfVoice = "Thân thiện, chân thật, đồng cảm sâu sắc, giải thích thuật ngữ phức tạp bằng ví dụ đời thường"
		dna.HookRules = []string{
			"Bắt đầu bằng câu hỏi ngược đời: 'Tại sao càng dưỡng da mụn càng bùng phát?'",
			"Đập vỡ ảo tưởng: 'Đừng phí tiền mua serum này nếu bạn chưa biết điều sau...'",
			"Con số gây sốc: '90% người dùng BHA sai cách khiến da bị breakout'",
			"Tình huống đồng cảm: 'Có bà nào trang điểm xong 2 tiếng là mốc meo vùng mũi không?'",
		}
		dna.ContentPillars = []string{
			"Giải mã bảng thành phần hoạt chất (Niacinamide, Retinol, BHA)",
			"So sánh trực diện 2 sản phẩm hot cùng phân khúc giá",
			"Thử nghiệm thực tế 7-14 ngày trước và sau",
			"Cảnh báo sai lầm kinh điển khi skincare tại nhà",
			"Gợi ý chu trình tối giản cho người bận rộn",
		}
		dna.VisualStyle = "Ánh sáng studio tự nhiên trong trẻo, cận cảnh texture chất kem, bảng biểu so sánh tối giản tương phản cao"
		dna.AudioStyle = "Giọng nữ truyền cảm ngọt ngào, tốc độ 140 từ/phút, nhạc nền lofi acoustic êm dịu"
		dna.RedLines = []string{
			"Cấm cam kết 'trắng da cấp tốc sau 1 đêm' hoặc 'chữa khỏi 100%'",
			"Không dìm nhãn hàng đối thủ vô căn cứ — chỉ nhận xét dựa trên bảng thành phần công bố",
			"Không dùng từ ngữ y khoa bị nền tảng quét hạn chế phân phối",
		}

	case strings.Contains(nl, "công nghệ") || strings.Contains(nl, "tai nghe") || strings.Contains(nl, "điện tử") || strings.Contains(nl, "tech"):
		dna.Positioning = "Reviewer công nghệ thực dụng: Tối ưu hiệu năng trên từng đồng chi phí"
		dna.AudiencePersona = "Nam & nữ 18-32 tuổi, yêu thích tiện ích số, tìm kiếm thiết bị công nghệ bền - đẹp - rẻ"
		dna.ValuePromise = "Đánh giá thiết bị công nghệ khách quan, không nhận tiền nói quá, chỉ điểm đúng ưu - nhược điểm cốt lõi"
		dna.ToneOfVoice = "Khách quan, logic, dứt khoát, hiện đại, mang tính kỹ thuật nhưng diễn đạt dễ hiểu"
		dna.HookRules = []string{
			"So sánh giá trị: 'Chiếc tai nghe 300k này có đè bẹp được hàng 2 triệu?'",
			"Cảnh báo người mua: 'Dừng lại ngay nếu bạn đang định mua món đồ công nghệ này...'",
			"Trải nghiệm thực chiến: 'Sau 30 ngày dùng thử chiếc củ sạc GaN siêu nhỏ này, đây là sự thật'",
		}
		dna.ContentPillars = []string{
			"Thử nghiệm độ bền & tính năng ngặt nghèo (test chống ồn, test rơi)",
			"Mẹo ẩn & tính năng ít người biết trên thiết bị phổ thông",
			"So sánh cấu hình và trải nghiệm thực tế",
			"Bộ sưu tập góc làm việc / phụ kiện công nghệ tiện ích",
		}
		dna.VisualStyle = "Ánh sáng tối huyền bí (dark aesthetic), macro cận cảnh chi tiết góc cạnh, text overlay chỉ số kỹ thuật"
		dna.AudioStyle = "Giọng nam dứt khoát đĩnh đạc, nhạc beat điện tử hiện đại nhịp nhàng, hiệu ứng sound effect tinh tế"
		dna.RedLines = []string{
			"Không quảng cáo sản phẩm không rõ nguồn gốc kiểm định chất lượng",
			"Không hứa hẹn các tính năng không có trong tài liệu nhà sản xuất",
		}

	case strings.Contains(nl, "gia dụng") || strings.Contains(nl, "bếp") || strings.Contains(nl, "nhà") || strings.Contains(nl, "đời sống"):
		dna.Positioning = "Cố vấn gia đình thông thái: Giải phóng sức lao động trong căn bếp & tổ ấm"
		dna.AudiencePersona = "Phụ nữ & gia đình 22-45 tuổi, bận rộn công việc, muốn chăm sóc nhà cửa tiện lợi và tiết kiệm thời gian"
		dna.ValuePromise = "Giúp việc nội trợ trở nên nhẹ nhàng, tinh tế và ngập tràn niềm vui với thiết bị thông minh"
		dna.ToneOfVoice = "Ấm áp, ân cần, gần gũi như người chị em trong gia đình"
		dna.HookRules = []string{
			"Giải quyết bực bội: 'Mỗi lần dọn bếp mất 1 tiếng? Món này cắt giảm còn 10 phút'",
			"Tình huống thực tế: 'Bữa cơm 15 phút cho người đi làm về muộn'",
			"Hỏi xoáy đáp xoay: 'Có nên mua nồi chiên không dầu hơi nước không?'",
		}
		dna.ContentPillars = []string{
			"Nấu món ngon siêu tốc với thiết bị gia dụng thông minh",
			"Mẹo vệ sinh nhà cửa nhanh - sạch - không độc hại",
			"Đánh giá độ bền và tính thực tế sau 6 tháng sử dụng",
			"Tối ưu không gian sống cho nhà nhỏ / chung cư",
		}
		dna.VisualStyle = "Tone màu ấm cúng (warm amber/beige), góc máy sinh hoạt gia đình tự nhiên, các cảnh quay thức ăn hấp dẫn"
		dna.AudioStyle = "Giọng nữ ấm áp nhẹ nhàng, có âm thanh ASMR nấu nướng chân thực, nhạc nền gia đình ấm áp"
		dna.RedLines = []string{
			"Không nói quá công dụng an toàn điện máy",
			"Mọi thử nghiệm nhiệt/điện phải tuân thủ khuyến cáo an toàn",
		}

	case strings.Contains(nl, "kể chuyện") || strings.Contains(nl, "lịch sử") || strings.Contains(nl, "truyện") || strings.Contains(nl, "story"):
		dna.Positioning = "Người kể chuyện đêm khuya: Đánh thức chiều sâu tâm hồn qua những góc nhìn nhân văn"
		dna.AudiencePersona = "Khán giả 16-45 tuổi thích lắng đọng, chiêm nghiệm cuộc sống, nghe chuyện trước khi ngủ hoặc khi làm việc"
		dna.ValuePromise = "Những câu chuyện nguyên bản chạm đến trái tim, mang lại sự bình yên và bài học sâu sắc"
		dna.ToneOfVoice = "Trầm lắng, sâu sắc, giàu hình ảnh gợi cảm xúc, triết lý nhẹ nhàng"
		dna.HookRules = []string{
			"Mở đầu bằng nghịch lý cuộc đời: 'Có những người cả đời đi tìm hạnh phúc, đến khi tìm thấy thì...'",
			"Cảnh ngộ đặc biệt: 'Năm tôi 18 tuổi, một người lạ đã dạy cho tôi bài học đắt nhất cuộc đời'",
			"Bí ẩn lịch sử: 'Hơn 500 năm trước, một quyết định trong đêm mưa đã thay đổi cả vận mệnh'",
		}
		dna.ContentPillars = []string{
			"Truyện ngắn ngôi thứ nhất về tình thân, sự nghiệp, nhân duyên",
			"Góc nhìn tâm lý học và bài học thấu hiểu bản thân",
			"Kể lại những điển tích, sự kiện lịch sử qua góc nhìn nhân vật phụ",
			"Những mẩu chuyện truyền cảm hứng vượt qua nghịch cảnh",
		}
		dna.VisualStyle = "Đồ họa cinematic 16:9 hoặc 9:16 có chiều sâu ánh sáng, chuyển cảnh Ken Burns chậm rãi gợi mở"
		dna.AudioStyle = "Giọng đọc truyền cảm, có độ ngân và khoảng ngắt giàu cảm xúc, nhạc nền piano/cello không lời êm dịu"
		dna.RedLines = []string{
			"Không sử dụng nội dung mê tín dị đoan thái quá hoặc bạo lực rùng rợn",
			"Không vi phạm bản quyền tác phẩm văn học đã có chủ sở hữu",
		}

	default: // Tổng hợp / Đa năng
		dna.Positioning = "Kênh sáng tạo nội dung nguyên bản chất lượng cao: Cung cấp giải pháp thực tế cho cuộc sống"
		dna.AudiencePersona = "Khán giả trẻ năng động 18-35 tuổi, thích thông tin nhanh, gọn, có ích và có tính ứng dụng cao"
		dna.ValuePromise = "Đúc kết kiến thức hữu ích trong từng giây, tôn trọng thời gian của khán giả"
		dna.ToneOfVoice = "Năng lượng, thông minh, tin cậy, truyền cảm hứng"
		dna.HookRules = []string{
			"3 giây đầu đập thẳng vào vấn đề mà không chào hỏi rườm rà",
			"Đưa ra giải pháp trực quan trước khi giải thích chi tiết",
		}
		dna.ContentPillars = []string{
			"Hướng dẫn thực hành từng bước (Step-by-step)",
			"Top 3 sai lầm phổ biến và cách khắc phục",
			"Đánh giá & gợi ý giải pháp tối ưu",
		}
		dna.VisualStyle = "Giao diện hiện đại, tương phản sắc nét, đồ họa trực quan"
		dna.AudioStyle = "Giọng đọc truyền cảm rõ ràng, nhạc nền nhịp điệu tích cực"
		dna.RedLines = []string{
			"Tuyệt đối không reup nguyên bản nội dung của người khác mà không có giá trị gia tăng",
			"Tuân thủ nghiêm ngặt chính sách cộng đồng và bản quyền âm nhạc",
		}
	}

	return dna
}

// GetBrandDNA loads the Brand DNA profile for an account. If no record exists,
// it returns a sensible default without saving, allowing the user to customize it.
func (s *Store) GetBrandDNA(accountID int64) (*BrandDNA, error) {
	row := s.db.QueryRow(`
		SELECT account_id, positioning, audience_persona, value_promise, tone_of_voice,
		       hook_rules_json, content_pillars_json, visual_style, audio_style,
		       red_lines_json, qc_checklist_json, updated_at
		FROM brand_dna WHERE account_id = ?`, accountID)

	var d BrandDNA
	var hooks, pillars, redLines, qc string
	err := row.Scan(&d.AccountID, &d.Positioning, &d.AudiencePersona, &d.ValuePromise,
		&d.ToneOfVoice, &hooks, &pillars, &d.VisualStyle, &d.AudioStyle,
		&redLines, &qc, &d.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(hooks), &d.HookRules)
	_ = json.Unmarshal([]byte(pillars), &d.ContentPillars)
	_ = json.Unmarshal([]byte(redLines), &d.RedLines)
	_ = json.Unmarshal([]byte(qc), &d.QCChecklist)

	return &d, nil
}

// SaveBrandDNA saves or updates the Brand DNA profile for an account.
func (s *Store) SaveBrandDNA(dna *BrandDNA) error {
	if dna == nil {
		return nil
	}
	hooks, _ := json.Marshal(dna.HookRules)
	pillars, _ := json.Marshal(dna.ContentPillars)
	redLines, _ := json.Marshal(dna.RedLines)
	qc, _ := json.Marshal(dna.QCChecklist)

	now := time.Now().Format("2006-01-02 15:04:05")
	dna.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO brand_dna (
			account_id, positioning, audience_persona, value_promise, tone_of_voice,
			hook_rules_json, content_pillars_json, visual_style, audio_style,
			red_lines_json, qc_checklist_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			positioning = excluded.positioning,
			audience_persona = excluded.audience_persona,
			value_promise = excluded.value_promise,
			tone_of_voice = excluded.tone_of_voice,
			hook_rules_json = excluded.hook_rules_json,
			content_pillars_json = excluded.content_pillars_json,
			visual_style = excluded.visual_style,
			audio_style = excluded.audio_style,
			red_lines_json = excluded.red_lines_json,
			qc_checklist_json = excluded.qc_checklist_json,
			updated_at = excluded.updated_at
	`, dna.AccountID, dna.Positioning, dna.AudiencePersona, dna.ValuePromise, dna.ToneOfVoice,
		string(hooks), string(pillars), dna.VisualStyle, dna.AudioStyle,
		string(redLines), string(qc), now)

	return err
}

// ValidateContentAgainstDNA evaluates a proposed script/hook against Brand DNA red lines and quality rules.
func (dna *BrandDNA) ValidateContentAgainstDNA(hook, script string) (pass bool, violations []string) {
	if dna == nil {
		return true, nil
	}
	combined := strings.ToLower(hook + " " + script)

	// Check generic policy red lines
	prohibitedWords := []string{
		"kem trộn", "thuốc trị bách bệnh", "chữa dứt điểm 100%", "cam kết làm giàu",
		"tăng follow nhanh", "đảm bảo viral", "hàng fake", "hàng nhái",
	}

	for _, pw := range prohibitedWords {
		if strings.Contains(combined, pw) {
			violations = append(violations, "Chứa từ cấm nền tảng: '"+pw+"'")
		}
	}

	// Check brand-specific red lines
	for _, rl := range dna.RedLines {
		rlLower := strings.ToLower(rl)
		if strings.Contains(rlLower, "cấm") {
			parts := strings.Split(rlLower, "cấm")
			if len(parts) > 1 {
				forbidden := strings.TrimSpace(parts[1])
				if len(forbidden) > 3 && strings.Contains(combined, forbidden) {
					violations = append(violations, "Vi phạm ranh giới đỏ: "+rl)
				}
			}
		}
	}

	return len(violations) == 0, violations
}
