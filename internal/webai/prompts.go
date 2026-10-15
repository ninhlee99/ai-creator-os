package webai

import (
	"fmt"
	"strings"
)

// GenerateAdScriptPackage synthesizes an end-to-end creative script & prompt package
// for product advertising across Claude Web, ChatGPT Web, Gemini Web, and Muse AI.
func GenerateAdScriptPackage(productName, niche, framework, targetAudience, keyBenefits, cta string) AdScriptPackage {
	productName = strings.TrimSpace(productName)
	if productName == "" {
		productName = "Sản phẩm xu hướng"
	}
	niche = strings.TrimSpace(niche)
	if niche == "" {
		niche = "Tiêu dùng & Thời trang"
	}
	if framework == "" {
		framework = "problem_solution"
	}
	if targetAudience == "" {
		targetAudience = "Khách hàng trẻ 18-35 tuổi, thích sự tiện ích và phong cách"
	}
	if keyBenefits == "" {
		keyBenefits = "Chất lượng cao, thiết kế thông minh, tối ưu trải nghiệm hàng ngày, giá cực tốt"
	}
	if cta == "" {
		cta = "Bấm ngay vào giỏ hàng hoặc link bên dưới để nhận ưu đãi hôm nay!"
	}

	pkg := AdScriptPackage{
		ProductName:   productName,
		Niche:         niche,
		Framework:     framework,
		CallToAction:  cta,
		TotalDuration: 30,
	}

	switch framework {
	case "three_reasons":
		pkg.HookText = fmt.Sprintf("3 lý do bạn nhất định phải sở hữu %s ngay hôm nay!", productName)
		pkg.Scenes = []AdScene{
			{
				Index:         1,
				DurationSec:   5,
				SceneName:     "Cảnh 1: Hook 3 Giây — Đòn Đánh Tò Mò",
				VisualConcept: fmt.Sprintf("Cận cảnh người mẫu cầm %s trên tay với biểu cảm ngạc nhiên thích thú.", productName),
				GeminiPrompt:  fmt.Sprintf("Ultra-detailed 4K studio shot, vibrant lighting, trendy Asian creator holding %s with an excited expression, cinematic shallow depth of field, sharp focus, 9:16 vertical ratio, photorealistic, no text, no watermark.", productName),
				MuseAIPrompt:  fmt.Sprintf("Smooth cinematic push-in zoom on %s, dynamic lighting shift, soft particle glow, commercial product showcase motion, 60fps.", productName),
				VoiceoverText: fmt.Sprintf("Nếu bạn vẫn chưa biết đến %s thì bạn đang bỏ lỡ một siêu phẩm đấy!", productName),
				OnScreenText:  fmt.Sprintf("3 LÝ DO PHẢI CÓ %s 🔥", strings.ToUpper(productName)),
				CameraMotion:  "Push-in zoom + dynamic tilt",
			},
			{
				Index:         2,
				DurationSec:   8,
				SceneName:     "Cảnh 2: Lý Do #1 — Thiết Kế & Chất Lượng Vượt Trội",
				VisualConcept: fmt.Sprintf("Góc quay cận cảnh (macro) các chi tiết đắt giá và đường nét hoàn thiện của %s.", productName),
				GeminiPrompt:  fmt.Sprintf("Macro photograph of %s showing premium textures, exquisite details, clean minimalist aesthetic, commercial lighting, tack-sharp 4K, 9:16 vertical, photorealistic, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Slow elegant 360-degree rotation of %s showcasing textures and premium build, soft ambient lighting reflection.", productName),
				VoiceoverText: fmt.Sprintf("Lý do thứ nhất: Thiết kế siêu tinh tế, độ hoàn thiện cao, dùng cực kỳ sang và tiện lợi."),
				OnScreenText:  "#1: THIẾT KẾ CỰC SANG ✨",
				CameraMotion:  "Slow 360 rotation + rack focus",
			},
			{
				Index:         3,
				DurationSec:   9,
				SceneName:     "Cảnh 3: Lý Do #2 & #3 — Công Năng Thực Tế & Đáng Tiền",
				VisualConcept: fmt.Sprintf("Người mẫu trải nghiệm tính năng nổi bật của %s trong đời sống hàng ngày.", productName),
				GeminiPrompt:  fmt.Sprintf("Lifestyle candid shot of trendy young person happily using %s in modern apartment / café, natural daylight, photorealistic 4K, vertical 9:16, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Fluid panning camera following the action of using %s, smooth cinematic motion blur, warm sunbeam lighting.", productName),
				VoiceoverText: fmt.Sprintf("Thứ hai: %s. Và quan trọng nhất: Mức giá quá hời so với chất lượng nhận lại!", keyBenefits),
				OnScreenText:  "#2 & #3: HIỆU QUẢ VƯỢT TRỘI ⚡",
				CameraMotion:  "Tracking pan + subtle handheld drift",
			},
			{
				Index:         4,
				DurationSec:   8,
				SceneName:     "Cảnh 4: Call-To-Action — Thúc Đẩy Mua Ngay",
				VisualConcept: fmt.Sprintf("Hình ảnh chốt đơn: %s tỏa sáng trong khung hình kèm nụ cười tự tin của người mẫu.", productName),
				GeminiPrompt:  fmt.Sprintf("Professional product hero shot of %s on modern pedestal, golden hour backlight, sleek aesthetic, 4K tack-sharp focus, vertical 9:16, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Dynamic fast zoom-out revealing the complete %s with subtle floating particles, premium commercial finish.", productName),
				VoiceoverText: fmt.Sprintf("%s Số lượng ưu đãi có hạn, rước ngay kẻo lỡ nhé!", cta),
				OnScreenText:  "ƯU ĐÃI HÔM NAY — BẤM MUA NGAY 🛒",
				CameraMotion:  "Fast zoom-out + hero lock",
			},
		}

	case "transformation":
		pkg.HookText = fmt.Sprintf("Trước và sau khi biết đến %s — Khác biệt 180 độ!", productName)
		pkg.Scenes = []AdScene{
			{
				Index:         1,
				DurationSec:   6,
				SceneName:     "Cảnh 1: Trước Khi Dùng (Vấn Đề & Mệt Mỏi)",
				VisualConcept: "Tông màu lạnh, người mẫu thở dài thất vọng với những cách truyền thống bất tiện.",
				GeminiPrompt:  fmt.Sprintf("Cinematic moody photograph, desaturated cool tones, tired person struggling with everyday frustration, vertical 9:16, tack-sharp 4K, no text."),
				MuseAIPrompt:  "Slow dolly out, dim lighting, subtle dramatic tension, film grain.",
				VoiceoverText: "Có ai từng mệt mỏi và tốn bao nhiêu thời gian như thế này giống mình không?",
				OnScreenText:  "TRƯỚC ĐÂY… QUÁ NẢN ❌",
				CameraMotion:  "Slow dolly out",
			},
			{
				Index:         2,
				DurationSec:   8,
				SceneName:     "Cảnh 2: Biến Hình & Xuất Hiện Sản Phẩm",
				VisualConcept: fmt.Sprintf("Ánh sáng bừng sáng, nụ cười rạng rỡ khi cầm %s trên tay.", productName),
				GeminiPrompt:  fmt.Sprintf("Bright energetic photograph, warm sunny glow, smiling creator holding %s excitedly, sharp 4K vertical 9:16, photorealistic, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Fast transition whip pan, lighting bursts into bright warm tones, hero shot of %s gleaming.", productName),
				VoiceoverText: fmt.Sprintf("Nhưng từ lúc chuyển sang dùng %s, mọi thứ thay đổi hoàn toàn!", productName),
				OnScreenText:  "TỪ KHI BIẾT ĐẾN SIÊU PHẨM NÀY ✨",
				CameraMotion:  "Whip pan + sudden snap zoom",
			},
			{
				Index:         3,
				DurationSec:   9,
				SceneName:     "Cảnh 3: Kết Quả Ngoạn Mục",
				VisualConcept: fmt.Sprintf("Hiệu quả rõ rệt khi dùng %s, cuộc sống tiện lợi và thảnh thơi.", productName),
				GeminiPrompt:  fmt.Sprintf("Crisp high-definition photo showing flawless results and satisfaction with %s, clean aesthetic, 4K, vertical 9:16, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Smooth orbiting camera around %s showing ease of use and instant benefits.", productName),
				VoiceoverText: fmt.Sprintf("Không chỉ tiết kiệm thời gian mà kết quả còn vượt ngoài mong đợi: %s.", keyBenefits),
				OnScreenText:  "KẾT QUẢ THỰC TẾ 10/10 💯",
				CameraMotion:  "Orbital curve pan",
			},
			{
				Index:         4,
				DurationSec:   7,
				SceneName:     "Cảnh 4: Chốt Đơn & Kêu Gọi",
				VisualConcept: fmt.Sprintf("Cận cảnh sản phẩm %s sang trọng sẵn sàng giao tới tay khách hàng.", productName),
				GeminiPrompt:  fmt.Sprintf("Luxury commercial photograph of %s, clean modern studio pedestal, vertical 9:16, 4K tack-sharp focus, no text.", productName),
				MuseAIPrompt:  "Hero zoom pulse, elegant shimmer effect, commercial finish.",
				VoiceoverText: fmt.Sprintf("Ai muốn thay đổi như mình thì %s", cta),
				OnScreenText:  "ĐẶT HÀNG NGAY TẠI LINK ⬇️",
				CameraMotion:  "Hero zoom pulse",
			},
		}

	default: // problem_solution
		pkg.HookText = fmt.Sprintf("Đừng mua %s nếu bạn chưa biết điều bí mật này!", productName)
		pkg.Scenes = []AdScene{
			{
				Index:         1,
				DurationSec:   5,
				SceneName:     "Cảnh 1: Hook Dừng Lướt 3 Giây",
				VisualConcept: fmt.Sprintf("Khoảnh khắc bất ngờ: người mẫu chỉ tay vào %s với ánh nhìn đầy kịch tính.", productName),
				GeminiPrompt:  fmt.Sprintf("Dramatic cinematic vertical 9:16 photograph, expressive Asian creator pointing at %s, modern room, high contrast commercial lighting, 4K tack-sharp, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Rapid snap zoom toward %s with subtle glitch transition, high energy, commercial pacing.", productName),
				VoiceoverText: fmt.Sprintf("Nếu bạn đang định tìm kiếm giải pháp cho cuộc sống hàng ngày, dừng lại 3 giây xem hết video này!"),
				OnScreenText:  fmt.Sprintf("DỪNG LẠI 3 GIÂY! ĐỪNG BỎ QUA 🛑"),
				CameraMotion:  "Snap zoom + micro shake",
			},
			{
				Index:         2,
				DurationSec:   8,
				SceneName:     "Cảnh 2: Khơi Gợi Vấn Đề (Pain Point)",
				VisualConcept: "Diễn tả sự bất tiện của các sản phẩm cũ thông thường trên thị trường.",
				GeminiPrompt:  fmt.Sprintf("Realistic photography of daily frustration with generic alternatives, vertical 9:16, natural indoor lighting, 4K, no text."),
				MuseAIPrompt:  "Slow panning shot showcasing the problem with realistic ambient lighting.",
				VoiceoverText: "Bao nhiêu lần bạn mua phải những món đồ dùng vài bữa là hỏng, vừa tốn tiền vừa bực mình?",
				OnScreenText:  "TỐN TIỀN VÌ CHỌN SAI? 💸",
				CameraMotion:  "Slow lateral track",
			},
			{
				Index:         3,
				DurationSec:   10,
				SceneName:     "Cảnh 3: Giải Pháp Đỉnh Cao — Trình Diễn Sản Phẩm",
				VisualConcept: fmt.Sprintf("Khám phá %s: Cận cảnh chất liệu, tính năng giải quyết triệt để vấn đề.", productName),
				GeminiPrompt:  fmt.Sprintf("Commercial 4K vertical 9:16 macro photograph of %s, glowing accents, premium build quality, modern luxury studio lighting, photorealistic, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Cinematic dynamic rotating camera showcasing %s from all angles, slow-motion particle dust, premium feel.", productName),
				VoiceoverText: fmt.Sprintf("Đó là lý do %s xuất hiện: %s, giải quyết gọn gàng mọi phiền toái chỉ trong tích tắc!", productName, keyBenefits),
				OnScreenText:  fmt.Sprintf("GIẢI PHÁP ĐỈNH CAO: %s ✅", strings.ToUpper(productName)),
				CameraMotion:  "Dynamic 3D rotational pan",
			},
			{
				Index:         4,
				DurationSec:   7,
				SceneName:     "Cảnh 4: Kêu Gọi Hành Động (CTA Chốt Sale)",
				VisualConcept: fmt.Sprintf("Hình ảnh hoàn mỹ của %s cùng ưu đãi đặc biệt.", productName),
				GeminiPrompt:  fmt.Sprintf("Hero product advertisement shot of %s with celebratory vibrant lighting, 4K tack-sharp focus, vertical 9:16, no text.", productName),
				MuseAIPrompt:  fmt.Sprintf("Smooth pull-back commercial reveal of %s with gleaming light streak effect.", productName),
				VoiceoverText: fmt.Sprintf("Đang có deal hời nhất tháng, %s", cta),
				OnScreenText:  "SĂN DEAL HỜI NGAY HÔM NAY 🎁",
				CameraMotion:  "Pull-back commercial reveal",
			},
		}
	}

	pkg.Caption = fmt.Sprintf("🔥 Siêu phẩm %s đã có mặt! %s\n\n👉 %s\n\n#affiliate #review #%s #xuhuong #tiktokmademebuyit #shorts",
		productName, pkg.HookText, cta, sanitizeTag(niche))

	pkg.Hashtags = []string{
		"#xuhuong",
		"#tiktokmademebuyit",
		"#" + sanitizeTag(productName),
		"#" + sanitizeTag(niche),
		"#reviewchat",
		"#affiliatevietnam",
	}

	// 4 Hook variants for A/B testing based on consumer psychology
	pkg.HookVariants = []HookVariant{
		{
			Type:  "curiosity",
			Title: "Hook Tò Mò (Curiosity Gap)",
			Text:  fmt.Sprintf("Bí mật đằng sau %s mà các shop lớn không muốn bạn biết...", productName),
			Angle: "Kích thích sự tò mò vô thức, giữ người xem nán lại xem giải thích.",
		},
		{
			Type:  "pain_point",
			Title: "Hook Nỗi Đau (Pain Point)",
			Text:  fmt.Sprintf("Ai đang mệt mỏi vì đồ dùng nhanh hỏng, xem hết video này để không phí tiền mua %s nhầm!", productName),
			Angle: "Đánh trúng nỗi sợ mua nhầm đồ kém chất lượng, tạo sự đồng cảm lập tức.",
		},
		{
			Type:  "social_proof",
			Title: "Hook Trải Nghiệm (Social Proof)",
			Text:  fmt.Sprintf("Mình đã test thử %s trong 14 ngày và đây là sự thật không ai nói cho bạn!", productName),
			Angle: "Xây dựng lòng tin tuyệt đối bằng trải nghiệm thực tế của người dùng.",
		},
		{
			Type:  "warning",
			Title: "Hook Cảnh Báo (Pattern Interrupt)",
			Text:  fmt.Sprintf("DỪNG LẠI 3 GIÂY! Nếu bạn đang có ý định mua đồ trong tầm giá này thì hãy xem %s trước!", productName),
			Angle: "Cắt đứt thói quen lướt vô thức trên feed bằng cảnh báo trực diện.",
		},
	}

	// Multi-platform customized posts
	cleanTag := sanitizeTag(productName)
	cleanNiche := sanitizeTag(niche)
	pkg.PlatformPosts = map[string]PlatformPost{
		"tiktok": {
			Platform: "TikTok",
			Title:    fmt.Sprintf("🔥 Review thực tế %s: Có đáng tiền không?", productName),
			Caption: fmt.Sprintf("%s\n\nƯu điểm đắt giá: %s.\n\n👇 Link săn deal chính hãng ở góc trái giỏ hàng màu vàng!",
				pkg.HookText, keyBenefits),
			Hashtags:     []string{"#tiktokmademebuyit", "#xuhuong", "#review", "#" + cleanTag, "#" + cleanNiche, "#goclamdep"},
			CallToAction: "Bấm vào giỏ hàng góc trái bên dưới để nhận voucher giảm giá hôm nay!",
			Tip:          "Nên ghim bình luận có link affiliate và trả lời các thắc mắc về kích thước/bảo hành trong 1 giờ đầu sau khi đăng.",
		},
		"youtube_shorts": {
			Platform: "YouTube Shorts",
			Title:    fmt.Sprintf("%s — Bí Mật Đáng Mua Nhất 2026? #Shorts", productName),
			Caption: fmt.Sprintf("%s\n\n📌 Đánh giá nhanh %s:\n- %s\n\n🛒 Link mua hàng chính hãng + mã giảm giá: Xem ở bình luận ghim bên dưới!",
				pkg.HookText, productName, keyBenefits),
			Hashtags:     []string{"#Shorts", "#review", "#" + cleanTag, "#" + cleanNiche, "#congnghe"},
			CallToAction: "Link chi tiết và mã giảm giá mình để trong phần bình luận ghim nhé!",
			Tip:          "Đặt tiêu đề viết hoa các từ quan trọng để tăng CTR trên YouTube Search và Shorts Feed.",
		},
		"facebook_reels": {
			Platform: "Facebook Reels & Fanpage",
			Title:    fmt.Sprintf("Đánh giá chân thực: %s", productName),
			Caption: fmt.Sprintf("Mọi người đã thử %s chưa? 😍\n\nSau thời gian trải nghiệm thì mình thấy: %s.\n\n👉 Bạn nào cần link mua chuẩn giá tốt thì xem ở phần bình luận đầu tiên nhé!",
				productName, keyBenefits),
			Hashtags:     []string{"#ReelsVN", "#review", "#" + cleanTag, "#lifestyle"},
			CallToAction: "Thả tim và lưu lại bài viết nếu thấy hữu ích nha cả nhà!",
			Tip:          "Facebook ưu tiên các video có lượt tương tác bình luận tự nhiên và chia sẻ về trang cá nhân.",
		},
		"threads": {
			Platform: "Threads",
			Title:    fmt.Sprintf("Góc review chân thật về %s", productName),
			Caption: fmt.Sprintf("Thật sự bất ngờ với %s mọi người ạ. Ban đầu tưởng quảng cáo thổi phồng nhưng dùng rồi mới thấy: %s.\n\nAi quan tâm link mua chuẩn thì mình để bên dưới thread này nhé 👇",
				productName, keyBenefits),
			Hashtags:     []string{"#" + cleanTag, "#review", "#gocnhiepanh", "#lifestyle"},
			CallToAction: "Ai đã dùng rồi cho mình xin thêm cảm nhận ở bên dưới nhé!",
			Tip:          "Viết theo phong cách tâm sự đời thường, ảnh thật không chèn chữ to để tránh thuật toán Threads bóp tương tác.",
		},
	}

	// Neuromarketing psychological triggers applied in this script
	pkg.PsychologicalTriggers = []string{
		"Đòn bẩy tò mò (Curiosity Gap) giữ chân 3 giây đầu tiên",
		"Khai thác nỗi đau hiện hữu (Pain Point Agitation) để mở khóa nhu cầu",
		"Trực quan hóa lợi ích vượt trội (Visualizing Transformation & Benefit)",
		"Bằng chứng xã hội & chi tiết thực tế (Social Proof & Macro Texture)",
		"Nỗi sợ bỏ lỡ cơ hội (FOMO & Limited Scarcity) thúc đẩy chốt đơn",
	}

	// Step-by-step guides for retrieving assets from Web AIs
	pkg.MediaRetrievalGuides = map[string]string{
		"museai":  "Khi Muse AI sinh xong clip, bấm vào video hoặc nút Chia sẻ, chọn 'Copy Video Link' (URL dạng .mp4 hoặc CDN). Trong Studio AI, bạn có thể dán link này vào ô 'URL Video' để hệ thống tự động tải về, hoặc bấm tải về máy rồi upload.",
		"gemini":  "Trên Gemini Web (Imagen 3), click chuột phải vào ảnh đã tạo chọn 'Sao chép địa chỉ hình ảnh' (Copy image address) hoặc 'Lưu hình ảnh'. Bạn có thể dán link ảnh hoặc kéo thả ảnh vào Studio để dựng video 4K.",
		"chatgpt": "Sao chép prompt bên dưới và dán vào ChatGPT Web. Bạn có thể yêu cầu ChatGPT viết thêm các góc nhìn kịch tính hoặc thêm chi tiết về phân khúc khách hàng cụ thể.",
		"claude":  "Sao chép prompt đạo diễn bên dưới và dán vào Claude Web. Claude sẽ phân tích chi tiết từng góc máy, ánh sáng và cử động điện ảnh cho từng giây của video.",
	}

	// Craft specialized prompt for ChatGPT Web
	pkg.ChatGPTPrompt = fmt.Sprintf(`Bạn là một chuyên gia sáng tạo kịch bản video bán hàng triệu view trên TikTok & Reels Việt Nam.
Hãy viết kịch bản chi tiết cho sản phẩm: "%s" (Ngành hàng: %s).
Đối tượng khách hàng: %s.
Ưu điểm chính: %s.
Khung kịch bản: %s.
Thời lượng: 30 giây.

Yêu cầu cụ thể:
1. Viết Hook 3 giây đầu tiên cực mạnh, giữ chân người xem không lướt qua.
2. Lời thoại voiceover tiếng Việt tự nhiên, gãy gọn, đánh trúng tâm lý, tạo cảm giác người dùng thật review chứ không phải quảng cáo gượng ép.
3. Chia thành 4 phân cảnh: Mô tả chi tiết hình ảnh mỗi cảnh + Lời thoại + Chữ hiển thị trên màn hình.
4. Lời kêu gọi hành động (Call To Action) thôi thúc bấm giỏ hàng ngay.
5. Viết kèm 1 Caption cuốn hút và 5 hashtag thịnh hành nhất.`, productName, niche, targetAudience, keyBenefits, framework)

	// Craft specialized prompt for Claude Web
	pkg.ClaudePrompt = fmt.Sprintf(`Bạn là một Đạo diễn Sáng tạo Nghệ thuật (Creative Film Director) chuyên sản xuất video quảng cáo sản phẩm cao cấp (Commercial Ads).
Tôi cần sản xuất video quảng cáo ngắn 30s tỉ lệ 9:16 dọc cho sản phẩm: "%s".
Ngành hàng: %s.
Đặc tính cốt lõi: %s.
Định dạng: %s.

Hãy thực hiện nhiệm vụ sau:
1. Đạo diễn phân cảnh (Storyboard 4 cảnh):
   - Cảnh 1 (0-5s): Hook thị giác và âm thanh.
   - Cảnh 2 (5-13s): Trực quan hóa vấn đề / tính năng đặc biệt.
   - Cảnh 3 (13-22s): Trải nghiệm thực tế và cảm xúc thăng hoa.
   - Cảnh 4 (22-30s): Cảnh Hero Shot tôn vinh sản phẩm và chốt đơn.
2. Viết Prompt tiếng Anh chi tiết cho từng cảnh để tạo ảnh 4K bằng Google Gemini Imagen 3 (đầy đủ thông số: lighting, camera lens, depth of field, photorealistic, 9:16).
3. Viết Prompt tiếng Anh cho Muse AI để tạo chuyển động camera mượt mà (camera panning, rotation, zoom speed).
4. Lời thoại voiceover tiếng Việt truyền cảm, nhịp điệu cuốn hút.`, productName, niche, keyBenefits, framework)

	return pkg
}

func sanitizeTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "&", "")
	return s
}

