package studio

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
				Text:        "Cơn mưa rào đầu hạ bất chợt đổ xuống sân trường cấp ba rực rỡ hoa phượng đỏ. Tôi và cậu ấy cùng đứng trú mưa dưới mái hiên dãy hành lang quen thuộc.",
				ImagePrompt: "Breathtaking Makoto Shinkai anime style vertical illustration 9:16, vibrant high school campus hallway in summer rain, a handsome anime boy with dark hair in crisp white school uniform holding a clear umbrella, standing next to a beautiful cute anime girl with long silky hair blushing softly, sparkling rain droplets falling, lush green trees and pink blossoms in background, emotional nostalgic anime masterpiece, vibrant colors, 8k",
			},
			{
				Text:        "Cậu ấy khẽ nghiêng chiếc ô về phía tôi, bờ vai ướt đẫm nước mưa nhưng nụ cười vẫn ấm áp như ánh mặt trời ban mai.",
				ImagePrompt: "Makoto Shinkai anime style vertical 9:16, cinematic close-up of the handsome anime boy gently tilting his umbrella over the cute anime girl, gentle affectionate smile on his face, rain mist glowing in golden ambient light, high school romance aesthetic, hyper-detailed anime art",
			},
			{
				Text:        "Những năm tháng thanh xuân cùng nhau học bài, cùng đạp xe qua từng con phố rợp bóng cây xanh đã khắc sâu vào ký ức không thể nào quên.",
				ImagePrompt: "Stunning anime style vertical 9:16, late afternoon golden hour sunset, the anime boy riding a bicycle with the cute anime girl sitting in the back holding his waist, joyful smiles, wind blowing through hair, scenic riverside road lined with blossoming trees, beautiful anime aesthetic",
			},
			{
				Text:        "Ngày bế giảng chia tay, dưới tán bằng lăng tím ngát, cậu ấy trao cho tôi lá thư hẹn ước và lời tỏ tình ngọt ngào nhất cuộc đời.",
				ImagePrompt: "Emotional Makoto Shinkai anime climax scene vertical 9:16, graduation day under blooming purple royal poinciana trees, sunset sky with pastel clouds, the anime boy confessing his love to the anime girl, tears of joy, holding hands, romantic anime masterpiece, 8k",
			},
		}
	}
	return []storyScene{
		{
			Text:        "Tôi và anh ký vào bản hợp đồng hôn nhân thời hạn hai năm trong một buổi chiều mưa tầm tã tại văn phòng luật sư xa hoa.",
			ImagePrompt: "Cinematic vertical portrait illustration 9:16 of a luxurious law office interior, rain pouring outside the floor-to-ceiling windows, a young beautiful Vietnamese woman in elegant white blazer and a handsome businessman in dark navy suit signing legal documents on a mahogany desk, dramatic moody lighting, 8k masterpiece",
		},
		{
			Text:        "Với mọi người ngoài kia, anh là người chồng hoàn hảo, nhưng đằng sau cánh cửa căn penthouse xa hoa, chúng tôi chỉ là hai người xa lạ cùng diễn một vở kịch.",
			ImagePrompt: "Modern high-end penthouse living room overlooking city skyline at dusk, vertical 9:16 composition, cold emotional distance between the beautiful Vietnamese woman and handsome businessman standing apart, warm ambient interior light contrasting cold twilight outside",
		},
		{
			Text:        "Thời gian trôi qua, những cử chỉ chăm sóc thầm lặng và ánh mắt ấm áp của anh bắt đầu khiến trái tim tôi rung động.",
			ImagePrompt: "A cozy modern kitchen at night, vertical 9:16 framing, soft warm lamplight, the same handsome Vietnamese businessman gently offering a warm cup of tea to the pensive beautiful woman, tender and emotional atmosphere",
		},
		{
			Text:        "Một đêm nọ, tôi phát hiện căn phòng bí mật ở tầng áp mái chứa toàn bộ sự thật về vụ tai nạn năm xưa và sự bảo vệ âm thầm suốt mười năm qua của anh.",
			ImagePrompt: "A secret attic room lit by a desk lamp, vertical 9:16 framing, old photo albums and archived documents on a wooden table, the same woman discovering heartfelt secrets in astonishment and tearful eyes, dramatic cinematic composition",
		},
		{
			Text:        "Kẻ thù tìm đến đe dọa, giữa lằn ranh sinh tử trong đêm mưa bão, anh đã lao ra lấy thân mình che chở cho tôi.",
			ImagePrompt: "Dramatic intense confrontation under pouring heavy rain at night on an empty city street, vertical 9:16 composition, headlights casting sharp shadows, the handsome man bravely shielding the woman, high tension cinematic drama",
		},
		{
			Text:        "Anh nắm chặt tay tôi và nói: Bản hợp đồng đã kết thúc từ lâu, nhưng tình yêu của anh dành cho em là mãi mãi.",
			ImagePrompt: "Heartfelt romantic emotional climax in the aftermath of rain, vertical 9:16 framing, wet asphalt reflecting streetlights, the couple embracing closely with tearful loving expressions, breathtaking cinematic lighting",
		},
	}
}

// writeStory dùng LLM viết truyện ngôi thứ nhất (tái dùng story.txt), có cơ chế phục hồi tự động.
func (s *Studio) writeStory(ctx context.Context, p StoryParams) (*storyDoc, error) {
	custom := strings.TrimSpace(p.CustomText)
	if custom != "" {
		if len([]rune(custom)) > 1_000_000 {
			return nil, fmt.Errorf("nội dung truyện vượt giới hạn 1 MB")
		}
		title := strings.TrimSpace(p.Topic)
		if title == "" {
			title = "Truyện tự viết"
		}
		return &storyDoc{Title: title, Text: custom}, nil
	}
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
			Orient:  "Viết ở NGÔI THỨ NHẤT (xưng \"tôi\") từ đầu đến cuối, tình tiết lôi cuốn, cao trào cảm xúc",
			Seconds: storyWords / 3,
		})
		if err == nil {
			cctx, ccancel := context.WithTimeout(ctx, 90*time.Second)
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
	if strings.TrimSpace(p.CustomText) != "" {
		return splitCustomStoryScenes(doc.Text, p.Scenes), nil
	}
	if s.llm != nil {
		visualStyle := "Masterclass cinematic digital painting, highly detailed 8k, photorealistic, Vietnamese context if applicable, no text, no watermark."
		if strings.Contains(strings.ToLower(p.Genre), "tình yêu") || strings.Contains(strings.ToLower(p.Genre), "ngôn tình") || strings.Contains(strings.ToLower(p.Topic), "tình yêu") || strings.Contains(strings.ToLower(p.Genre), "anime") {
			visualStyle = "Makoto Shinkai & CoMix Wave anime aesthetic, stunning detailed romantic anime art, beautiful anime couple with consistent appearance, emotional cinematic lighting, vibrant atmospheric depth, 8k resolution, no text, no watermark."
		}

		sys := "Bạn là biên tập storyboard kiêm đạo diễn hình ảnh điện ảnh Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
		prompt := fmt.Sprintf(`Chia truyện sau thành đúng %d cảnh theo trình tự thời gian cho video kể chuyện 9:16 chuẩn 5 sao.
Mỗi cảnh gồm:
1. text: 1 đoạn văn kể ở ngôi thứ nhất giàu cảm xúc, ngắt nhịp tự nhiên theo hơi thở (40–120 từ).
   - Hãy chèn các thẻ cảm xúc dẫn xuất ở đầu câu thoại phù hợp: [trầm lặng], [cao trào], [sôi nổi], [buồn bã], [tức giận], [vui vẻ], [thì thầm] để giọng đọc AI biến chuyển cảm xúc chân thật.
2. image_prompt: 1 prompt tiếng Anh chi tiết cho AI vẽ ảnh minh họa điện ảnh 9:16 tuyệt đẹp:
   - Tỷ lệ khung hình: 9:16 vertical portrait composition, framed vertically.
   - Tính nhất quán nhân vật (Character Consistency): Thống nhất ngoại hình nhân vật chính xuyên suốt các cảnh (tuổi, trang phục, gương mặt, màu tóc).
   - Cỡ cảnh & góc máy: Đa dạng giữa Wide shot (bối cảnh bao la), Medium shot (tương tác), Close-up (chi tiết biểu cảm nhân vật).
   - Ánh sáng & không khí: Cinematic lighting, volumetric light, moody chiaroscuro hoặc warm ambient, atmospheric depth.
   - Phong cách thị giác: %s

Truyện: %s

Chỉ trả JSON: {"scenes": [{"text": "...", "image_prompt": "..."}, ...]}`,
			p.Scenes, visualStyle, doc.Text)
		cctx, ccancel := context.WithTimeout(ctx, 60*time.Second)
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

func splitCustomStoryScenes(text string, requested int) []storyScene {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	count := requested
	if count < 3 {
		count = 6
	}
	if count > 12 {
		count = 12
	}
	if count > len(words) {
		count = len(words)
	}
	if count < 1 {
		count = 1
	}
	scenes := make([]storyScene, 0, count)
	for i := 0; i < count; i++ {
		start := i * len(words) / count
		end := (i + 1) * len(words) / count
		part := strings.Join(words[start:end], " ")
		promptText := part
		if len([]rune(promptText)) > 500 {
			promptText = string([]rune(promptText)[:500])
		}
		scenes = append(scenes, storyScene{
			Text:        part,
			ImagePrompt: fmt.Sprintf("Create one cinematic illustration that literally depicts this story passage: %s. Keep characters consistent, Vietnamese context where relevant, no text, no watermark.", promptText),
		})
	}
	return scenes
}
