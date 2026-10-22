package webai

import (
	"fmt"
	"strings"
	"time"
)

func generateDomainAwarePackage(engine, badge, productName, niche, framework, targetAudience, keyBenefits, cta string) AdScriptPackage {
	pkg := AdScriptPackage{
		ProductName:   productName,
		Niche:         niche,
		Framework:     framework,
		CallToAction:  cta,
		TotalDuration: 30,
		HookText:      fmt.Sprintf("Cùng xem kỹ %s", productName),
		HookVariants: []HookVariant{
			{Type: "curiosity", Title: "Chi tiết sản phẩm", Text: fmt.Sprintf("Điểm nào của %s cần xem kỹ?", productName), Angle: "Mở đầu bằng câu hỏi, không khẳng định lợi ích chưa được xác nhận."},
			{Type: "product_detail", Title: "Đối chiếu sản phẩm", Text: fmt.Sprintf("Đây là %s; cùng đối chiếu với ảnh sản phẩm.", productName), Angle: "Đưa người xem về chi tiết có thể kiểm tra trong ảnh tham chiếu."},
		},
	}

	facts := strings.TrimSpace(keyBenefits)
	if facts == "" {
		facts = "Chưa có lợi ích được xác nhận; chỉ mô tả chi tiết nhìn thấy trong ảnh tham chiếu."
	}
	audience := strings.TrimSpace(targetAudience)
	if audience == "" {
		audience = "người xem đang tìm hiểu sản phẩm"
	}
	productReference := "Dùng ảnh listing được cung cấp làm chuẩn; giữ nguyên sản phẩm, màu, kiểu dáng, nhãn và chi tiết."
	modelReference := "Dùng đúng ảnh nhân vật của kênh; giữ nhận diện và cho nhân vật mặc hoặc sử dụng đúng sản phẩm khi phù hợp với loại sản phẩm."
	scenes := []AdScene{
		{Index: 1, DurationSec: 5, SceneName: "Mở đầu", VisualConcept: fmt.Sprintf("Nhân vật của kênh giới thiệu %s trước máy quay.", productName), GeminiPrompt: productReference + " " + modelReference + " Khung dọc 9:16, không chữ tự sinh, không watermark.", MuseAIPrompt: "Chuyển động máy nhẹ; giữ nguyên sản phẩm và nhận diện nhân vật từ ảnh tham chiếu.", VoiceoverText: fmt.Sprintf("Cùng xem %s.", productName), OnScreenText: productName, CameraMotion: "Cận cảnh ổn định."},
		{Index: 2, DurationSec: 8, SceneName: "Chi tiết nhìn thấy", VisualConcept: "Cận cảnh các chi tiết có trong ảnh listing; không thêm cấu tạo hoặc tính năng.", GeminiPrompt: productReference + " Chỉ tái hiện chi tiết nhìn thấy trong ảnh sản phẩm; không thêm chữ hoặc thông số.", MuseAIPrompt: "Lia máy chậm qua chi tiết có trong ảnh listing; không đổi sản phẩm.", VoiceoverText: "Đối chiếu màu sắc, hình dáng và chi tiết trên ảnh sản phẩm.", OnScreenText: "Chi tiết theo ảnh tham chiếu", CameraMotion: "Lia máy chậm."},
		{Index: 3, DurationSec: 10, SceneName: "Mặc hoặc sử dụng", VisualConcept: "Nhân vật mặc hoặc sử dụng đúng sản phẩm, nếu phù hợp với loại sản phẩm; tránh diễn tả kết quả chưa được xác nhận.", GeminiPrompt: productReference + " " + modelReference + " Chỉ thể hiện thao tác sử dụng trực quan, không thêm hiệu quả hoặc tính năng.", MuseAIPrompt: "Giữ hành động đơn giản, tự nhiên và bám ảnh tham chiếu; không tạo kết quả trước-sau.", VoiceoverText: "Đối chiếu sản phẩm với ảnh tham chiếu trước khi quyết định.", OnScreenText: "Thông tin theo mô tả đã cung cấp", CameraMotion: "Trung cảnh ổn định."},
		{Index: 4, DurationSec: 7, SceneName: "Thông tin và liên kết", VisualConcept: "Nhân vật và sản phẩm xuất hiện cùng khung; giữ nguyên ảnh listing.", GeminiPrompt: productReference + " Đặt sản phẩm rõ trong khung cùng nhân vật của kênh; không thêm ưu đãi, giá hoặc chữ.", MuseAIPrompt: "Kết thúc bằng khung hình ổn định có nhân vật và sản phẩm.", VoiceoverText: cta, OnScreenText: "Xem thông tin tại liên kết", CameraMotion: "Máy quay tĩnh."},
	}
	pkg.Scenes = scenes
	pkg.QC = evaluateQC(pkg)
	pkg.QC.CheckedAt = time.Now().Format("15:04 02/01/2006")
	fillComplementaryPackageFields(&pkg, facts)
	pkg.MediaRetrievalGuides = map[string]string{
		"museai":  "Dùng công cụ Muse AI đã cấu hình và đính kèm ảnh listing cùng ảnh nhân vật của kênh.",
		"gemini":  "Dùng công cụ tạo ảnh đã cấu hình; đính kèm ảnh listing và ảnh nhân vật làm tham chiếu.",
		"chatgpt": "Dùng prompt bên dưới với ảnh listing và ảnh nhân vật đã được chọn.",
		"claude":  "Dùng prompt bên dưới với ảnh listing và ảnh nhân vật đã được chọn.",
	}
	pkg.ChatGPTPrompt = fmt.Sprintf("Tạo kịch bản giới thiệu %q cho nhóm người xem: %s. Chỉ dùng dữ kiện người dùng cung cấp: %s. Dùng ảnh listing làm chuẩn sản phẩm và ảnh nhân vật làm chuẩn nhận diện; yêu cầu nhân vật mặc hoặc sử dụng đúng sản phẩm khi phù hợp. Không bịa lợi ích, giá, ưu đãi, trải nghiệm hay kết quả.", productName, audience, facts)
	pkg.ClaudePrompt = fmt.Sprintf("Lập storyboard 4 cảnh cho %q (%s). Bám ảnh listing để giữ đúng sản phẩm và ảnh nhân vật để giữ đúng người mẫu. Chỉ trình bày dữ kiện đã cung cấp: %s. Không thêm tính năng, chất liệu, giá, ưu đãi, trải nghiệm hoặc kết quả chưa xác nhận.", productName, niche, facts)
	pkg.AIEngine = engine
	pkg.AIEngineBadge = badge
	return pkg
}

// evaluateQC checks verifiable script fields only. It does not score the
// rendered video, retention, sales conversion, or visual fidelity.
