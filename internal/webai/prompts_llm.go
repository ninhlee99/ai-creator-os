package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func generateViaLLM(ctx context.Context, llm LLMCompleter, engine, badge, productName, niche, framework, targetAudience, keyBenefits, cta string) (AdScriptPackage, error) {
	var personaIntro string
	switch engine {
	case EngineClaude:
		personaIntro = `Bạn viết kịch bản kể chuyện gọn, tự nhiên. Chỉ dùng dữ kiện được cung cấp; không giả vờ có trải nghiệm cá nhân.`
	case EngineGrok:
		personaIntro = `Bạn là biên tập viên kịch bản ngắn theo phong cách Grok.
Hook cần rõ và gợi tò mò, không phóng đại, bịa tranh cãi hoặc hứa hẹn hiệu quả.`
	case EngineChatGPT:
		personaIntro = `Bạn là copywriter thương mại theo phong cách ChatGPT.
Trình bày thông tin gọn, rõ và thuyết phục bằng dữ kiện đã cung cấp; không tạo khan hiếm, giảm giá hoặc áp lực mua giả.`
	case EngineGemini:
		personaIntro = `Bạn viết chỉ dẫn thị giác rõ về bố cục, ánh sáng và thao tác. Không tự thêm thông số hoặc đặc điểm ngoài ảnh tham chiếu.`
	default:
		personaIntro = `Bạn là Giám đốc Sáng tạo (Creative Director) chuyên viết kịch bản bán hàng TikTok / Reels / Shorts tại Việt Nam.`
	}

	systemPrompt := personaIntro + `
Nhiệm vụ của bạn là tạo gói kịch bản từ văn bản sản phẩm/ngành hàng người dùng nhập. Bạn không được giả vờ đã xem ảnh listing; các prompt tạo hình phải yêu cầu công cụ phía sau dùng ảnh listing và ảnh nhân vật làm tham chiếu.

Quy tắc dữ kiện:
1. Dùng Product Name, ngành hàng, đối tượng, lợi ích và CTA đã cung cấp; trường trống nghĩa là chưa xác nhận.
2. Không bịa tính năng, chất liệu, giá, ưu đãi, độ phổ biến, kết quả, trải nghiệm cá nhân, đánh giá khách hàng, độ khan hiếm, view, retention hoặc doanh số.
3. Không viết trải nghiệm cá nhân hoặc bằng chứng xã hội nếu người dùng không cung cấp dữ kiện đó.
4. Với hình ảnh/video, ảnh listing là chuẩn chính xác cho sản phẩm; không đổi màu, kiểu dáng, chi tiết hoặc nhãn. Người mẫu phải thực sự mặc/dùng sản phẩm hợp lý.
5. Hook gợi tò mò nhưng không gây hiểu nhầm. CTA chỉ trỏ tới liên kết được cung cấp, không tạo ưu đãi.
6. Không tạo QC score. Server chỉ kiểm tra độ đầy đủ trường kịch bản; việc đó không đánh giá video, nhận diện, chuyển đổi hoặc lượt xem.

Trả về kết quả DUY NHẤT dưới dạng JSON hợp lệ (không kèm markdown ngoài JSON block) theo cấu trúc:
{
  "hook_text": "...",
  "hook_variants": [
    {"type": "curiosity", "title": "Hook Tò Mò", "text": "...", "angle": "..."},
    {"type": "pain_point", "title": "Hook Nỗi Đau", "text": "...", "angle": "..."},
    {"type": "product_detail", "title": "Hook Chi Tiết", "text": "...", "angle": "..."},
    {"type": "warning", "title": "Hook Cảnh Báo", "text": "...", "angle": "..."}
  ],
  "scenes": [
    {
      "index": 1,
      "duration_sec": 5,
      "scene_name": "Cảnh 1: Hook 3s Giữ Chân",
      "visual_concept": "...",
      "gemini_prompt": "Ultra-detailed 4K studio shot, 9:16 vertical, commercial lighting, photorealistic, no text...",
      "muse_ai_prompt": "Smooth cinematic motion, 60fps...",
      "voiceover_text": "...",
      "on_screen_text": "...",
      "camera_motion": "..."
    },
    {
      "index": 2,
      "duration_sec": 8,
      "scene_name": "Cảnh 2: Chi tiết sản phẩm",
      "visual_concept": "...",
      "gemini_prompt": "...",
      "muse_ai_prompt": "...",
      "voiceover_text": "...",
      "on_screen_text": "...",
      "camera_motion": "..."
    },
    {
      "index": 3,
      "duration_sec": 10,
      "scene_name": "Cảnh 3: Mặc hoặc sử dụng",
      "visual_concept": "...",
      "gemini_prompt": "...",
      "muse_ai_prompt": "...",
      "voiceover_text": "...",
      "on_screen_text": "...",
      "camera_motion": "..."
    },
    {
      "index": 4,
      "duration_sec": 7,
      "scene_name": "Cảnh 4: Chốt Đơn CTA",
      "visual_concept": "...",
      "gemini_prompt": "...",
      "muse_ai_prompt": "...",
      "voiceover_text": "...",
      "on_screen_text": "...",
      "camera_motion": "..."
    }
  ]
}`

	userPrompt := fmt.Sprintf(`Sản phẩm: "%s"
Ngành hàng: "%s"
Khung kịch bản: "%s"
Đối tượng mục tiêu: "%s"
Ưu điểm nổi bật: "%s"
Kêu gọi hành động: "%s"
Định hướng Persona AI: "%s"`, productName, niche, framework, targetAudience, keyBenefits, cta, badge)

	resp, err := llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return AdScriptPackage{}, err
	}

	clean := strings.TrimSpace(resp)
	if idx := strings.Index(clean, "{"); idx >= 0 {
		clean = clean[idx:]
	}
	if idx := strings.LastIndex(clean, "}"); idx >= 0 {
		clean = clean[:idx+1]
	}

	var parsed struct {
		HookText     string        `json:"hook_text"`
		HookVariants []HookVariant `json:"hook_variants"`
		Scenes       []AdScene     `json:"scenes"`
	}
	if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
		return AdScriptPackage{}, err
	}

	pkg := AdScriptPackage{
		ProductName:   productName,
		Niche:         niche,
		Framework:     framework,
		CallToAction:  cta,
		TotalDuration: 30,
		HookText:      parsed.HookText,
		HookVariants:  parsed.HookVariants,
		Scenes:        parsed.Scenes,
	}

	pkg.QC = evaluateQC(pkg)
	pkg.QC.CheckedAt = time.Now().Format("15:04 02/01/2006")
	fillComplementaryPackageFields(&pkg, keyBenefits)
	return pkg, nil
}

// generateDomainAwarePackage produces highly targeted, realistic Vietnamese creator scripts
// customized for each specific product niche, avoiding generic corporate templates.
