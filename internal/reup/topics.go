package reup

// TopicSampleUser là một kênh Douyin mẫu chất lượng cao thuộc chủ đề.
type TopicSampleUser struct {
	Username    string `json:"username"`     // ID tài khoản Douyin (để quét qua TikWM)
	DisplayName string `json:"display_name"` // Tên gợi nhớ tiếng Việt
	Followers   string `json:"followers"`    // Ước tính quy mô kênh
	Note        string `json:"note"`         // Ghi chú đặc trưng
}

// ViralTopic là một chủ đề video viral trên Douyin được tuyển chọn sẵn.
type ViralTopic struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Icon        string            `json:"icon"`
	Badge       string            `json:"badge"`
	BadgeClass  string            `json:"badge_class"`
	ChineseTag  string            `json:"chinese_tag"`
	Keywords    []string          `json:"keywords"`
	Description string            `json:"description"`
	SampleUsers []TopicSampleUser `json:"sample_users"`
}

// DefaultViralTopics trả về danh mục các chủ đề Douyin hot nhất hiện nay.
func DefaultViralTopics() []ViralTopic {
	return []ViralTopic{
		{
			ID:          "sexy_dance",
			Title:       "Nhảy Sexy & Vũ Đạo Bốc Lửa",
			Icon:        "💃",
			Badge:       "🔥 TRIỆU VIEW NHANH",
			BadgeClass:  "badge-err",
			ChineseTag:  "热舞 / 性感舞蹈",
			Keywords:    []string{"热舞", "性感舞蹈", "甩臀舞", "韩舞翻跳", "微密圈", "摇摆舞"},
			Description: "Các điệu nhảy quyến rũ, visual triệu view, âm nhạc remix bắt tai. Hút tương tác cực nhanh, cần dùng Transform (lật gương, lồng nhạc) để bảo vệ kênh.",
			SampleUsers: []TopicSampleUser{
				{
					Username:    "hotdance888",
					DisplayName: "Tiểu Mị Hot Dance",
					Followers:   "1.8M+",
					Note:        "Chuyên vũ đạo EDM remix, nhảy bốc lửa bắt beat",
				},
				{
					Username:    "linlin_dance",
					DisplayName: "Lâm Lâm Vũ Đạo",
					Followers:   "2.5M+",
					Note:        "Cover vũ đạo K-Pop quyến rũ, visual đỉnh",
				},
				{
					Username:    "kpop_hot_dance",
					DisplayName: "Hàn Vũ Tuyển Chọn",
					Followers:   "950K+",
					Note:        "Clip ngắn 15s bắt mắt, tương tác tim cực cao",
				},
			},
		},
		{
			ID:          "transform",
			Title:       "Biến Hình & Chuyển Cảnh Giật Beat",
			Icon:        "✨",
			Badge:       "👑 GIỮ CHÂN 85%+",
			BadgeClass:  "badge-ok",
			ChineseTag:  "变装 / 卡点",
			Keywords:    []string{"变装", "卡点变装", "国风变装", "变身", "高级脸", "逆袭变装"},
			Description: "Video biến hình vịt hóa thiên nga, trước/sau chuẩn nhịp nhạc. Tỷ lệ giữ chân người xem cực cao, thuật toán TikTok đề xuất cực mạnh.",
			SampleUsers: []TopicSampleUser{
				{
					Username:    "bianzhuang_king",
					DisplayName: "Vương Bài Biến Hình",
					Followers:   "4.2M+",
					Note:        "Bậc thầy chuyển cảnh giật beat, phong cách thời thượng",
				},
				{
					Username:    "guofeng_beauty",
					DisplayName: "Cổ Phong Mỹ Nữ",
					Followers:   "3.1M+",
					Note:        "Biến hình phong cách trang phục cổ trang kiếm hiệp",
				},
			},
		},
		{
			ID:          "home_goods",
			Title:       "Gia Dụng Thông Minh & Mẹo Đời Sống",
			Icon:        "🏠",
			Badge:       "💰 RA ĐƠN AFFILIATE",
			BadgeClass:  "badge-info",
			ChineseTag:  "家居好物 / 居家生活",
			Keywords:    []string{"家居好物", "居家好物", "厨房神器", "生活小妙招", "黑科技好物", "收纳神器"},
			Description: "Review các món đồ tiện ích thông minh giải quyết nỗi đau người xem. Khách hàng xem xong click mua ngay tại giỏ hàng hoặc bio.",
			SampleUsers: []TopicSampleUser{
				{
					Username:    "jiaju_haowu",
					DisplayName: "Gia Dụng Thần Kỳ",
					Followers:   "2.8M+",
					Note:        "Chuyên đồ dùng bếp & phòng ngủ thông minh cực hút đơn",
				},
				{
					Username:    "shenghuo_miao",
					DisplayName: "Mẹo Vặt Đời Sống",
					Followers:   "1.9M+",
					Note:        "Mẹo vặt gia đình kết hợp giới thiệu sản phẩm khéo léo",
				},
			},
		},
		{
			ID:          "beauty_skincare",
			Title:       "Mỹ Phẩm, Make-up & Dưỡng Da",
			Icon:        "💄",
			Badge:       "🛍️ HOA HỒNG CAO 25%+",
			BadgeClass:  "badge-ok",
			ChineseTag:  "美妆 / 护肤分享",
			Keywords:    []string{"美妆教程", "护肤分享", "沉浸式护肤", "神仙妆容", "好物测评", "变美逆袭"},
			Description: "Quy trình chăm sóc da ngâm (chén thánh), makeup tone Tây/Trung, swatch son dưỡng. Thích hợp nhất cho kênh TikTok Shop mỹ phẩm.",
			SampleUsers: []TopicSampleUser{
				{
					Username:    "meizhuang_da",
					DisplayName: "Chuyên Gia Skincare Trung",
					Followers:   "3.4M+",
					Note:        "Routine dưỡng trắng da, review kem nền mỏng nhẹ",
				},
				{
					Username:    "chunjing_makeup",
					DisplayName: "Thuần Khiết Makeup",
					Followers:   "1.7M+",
					Note:        "Makeup tự nhiên đi học, đi làm, review son hot",
				},
			},
		},
		{
			ID:          "street_fashion",
			Title:       "Thời Trang & Street Style Phối Đồ",
			Icon:        "👗",
			Badge:       "✨ TỶ TỶ DOUYIN",
			BadgeClass:  "badge-info",
			ChineseTag:  "穿搭 / 街拍",
			Keywords:    []string{"穿搭技巧", "街拍", "微胖穿搭", "显瘦神裤", "辣妹穿搭", "秋冬穿搭"},
			Description: "Quay street style Thượng Hải/Hàng Châu, phối đồ tôn dáng, hack chiều cao. Hút cả tệp khán giả nam lẫn nữ mua sắm quần áo.",
			SampleUsers: []TopicSampleUser{
				{
					Username:    "jiepai_fashion",
					DisplayName: "Street Style Thượng Hải",
					Followers:   "5.1M+",
					Note:        "Gu thời trang sang chảnh, dáng chuẩn triệu view",
				},
				{
					Username:    "chuanda_girl",
					DisplayName: "Tỷ Tỷ Phối Đồ",
					Followers:   "2.2M+",
					Note:        "Cách phối đồ che khuyết điểm, quần ống suông tôn dáng",
				},
			},
		},
		{
			ID:          "comedy_drama",
			Title:       "Tiểu Phẩm Hài & Đời Sống Quay Xe",
			Icon:        "😂",
			Badge:       "🚀 VIRAL TỰ NHIÊN",
			BadgeClass:  "badge-ok",
			ChineseTag:  "搞笑 / 剧情反转",
			Keywords:    []string{"搞笑日常", "沙雕视频", "反转剧情", "情侣日常", "幽默段子", "戏精日常"},
			Description: "Tình huống hài hước ngắn, kết thúc 'quay xe' bất ngờ. Dễ ăn đề xuất tự nhiên, hút hàng triệu view và lượt bình luận tranh luận.",
			SampleUsers: []TopicSampleUser{
				{
					Username:    "gaoxiao_duanzi",
					DisplayName: "Cười Nghiêng Ngả",
					Followers:   "4.8M+",
					Note:        "Tiểu phẩm tình huống ngắn gia đình hài hước",
				},
				{
					Username:    "fanzhuan_drama",
					DisplayName: "Kịch Bản Bẻ Lái",
					Followers:   "3.6M+",
					Note:        "Tình huống lật mặt phút chót giữ chân 100%",
				},
			},
		},
		{
			ID:          "food_cooking",
			Title:       "Ẩm Thực & Nấu Ăn Đêm Khuya",
			Icon:        "🍲",
			Badge:       "🔥 KÍCH THÍCH THỊ GIÁC",
			BadgeClass:  "badge-warn",
			ChineseTag:  "美食 / 街头小吃",
			Keywords:    []string{"美食制作", "街头美食", "夜市探店", "治愈系美食", "家常菜", "深夜食堂"},
			Description: "Quay cận cảnh xèo xèo đồ ăn đường phố, nấu ăn chữa lành, âm thanh ASMR nấu nướng giữ chân người xem cực tốt.",
			SampleUsers: []TopicSampleUser{
				{
					Username:    "meishi_tan",
					DisplayName: "Ẩm Thực Đêm Douyin",
					Followers:   "3.9M+",
					Note:        "Quay cận cảnh đồ ăn đêm đường phố âm thanh ASMR",
				},
			},
		},
	}
}
