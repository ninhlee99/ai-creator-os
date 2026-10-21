package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// LLMCompleter matches the LLMClient interface.
type LLMCompleter interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
}

// AI Engine persona constants for creative copywriting and storyboard generation.
const (
	EngineAuto    = "auto"
	EngineClaude  = "claude"
	EngineGrok    = "grok"
	EngineChatGPT = "chatgpt"
	EngineGemini  = "gemini"
)

// EngineInfo holds metadata for display and persona styling.
type EngineInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Badge       string `json:"badge"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
}

// SupportedEngines returns metadata for all available AI engine personas.
func SupportedEngines() []EngineInfo {
	return []EngineInfo{
		{
			ID:          EngineAuto,
			Name:        "Tự động chọn phong cách",
			Badge:       "Tự động chọn phong cách",
			Icon:        "⚡",
			Description: "Chọn phong cách viết theo ngành hàng đã nhập",
		},
		{
			ID:          EngineClaude,
			Name:        "Storytelling",
			Badge:       "Storytelling",
			Icon:        "🟣",
			Description: "Kịch bản kể chuyện, chỉ dùng dữ kiện đã cung cấp",
		},
		{
			ID:          EngineGrok,
			Name:        "Hook rõ ràng",
			Badge:       "Hook rõ ràng",
			Icon:        "🚀",
			Description: "Hook gợi tò mò, không tạo tranh cãi hoặc cam kết lượt xem",
		},
		{
			ID:          EngineChatGPT,
			Name:        "Copy thương mại",
			Badge:       "Copy thương mại",
			Icon:        "💬",
			Description: "Trình bày dữ kiện sản phẩm gọn và rõ",
		},
		{
			ID:          EngineGemini,
			Name:        "Đạo diễn thị giác",
			Badge:       "Đạo diễn thị giác",
			Icon:        "✨",
			Description: "Mô tả bố cục và thao tác theo ảnh tham chiếu",
		},
	}
}

func getEngineBadge(engine string) string {
	for _, e := range SupportedEngines() {
		if e.ID == engine {
			return e.Badge
		}
	}
	return "⚡ Tự Động (Smart Auto-Balance)"
}

// GenerateAdScriptPackage generates a creative script & prompt package (fallback to domain-aware engine).
func GenerateAdScriptPackage(productName, niche, framework, targetAudience, keyBenefits, cta string) AdScriptPackage {
	return GenerateAdScriptPackageWithEngine(context.Background(), nil, EngineAuto, productName, niche, framework, targetAudience, keyBenefits, cta)
}

// GenerateAdScriptPackageWithLLM maintains backwards compatibility.
func GenerateAdScriptPackageWithLLM(ctx context.Context, llm LLMCompleter, productName, niche, framework, targetAudience, keyBenefits, cta string) AdScriptPackage {
	return GenerateAdScriptPackageWithEngine(ctx, llm, EngineAuto, productName, niche, framework, targetAudience, keyBenefits, cta)
}

// GenerateAdScriptPackageWithEngine generates a script package using the selected writing style.
func GenerateAdScriptPackageWithEngine(ctx context.Context, llm LLMCompleter, engine, productName, niche, framework, targetAudience, keyBenefits, cta string) AdScriptPackage {
	productName = strings.TrimSpace(productName)
	if productName == "" {
		productName = "Sản phẩm chưa có tên"
	}
	niche = strings.TrimSpace(niche)
	if framework == "" {
		framework = "product_detail"
	}
	if targetAudience == "" {
		targetAudience = "Người xem đang tìm hiểu sản phẩm"
	}
	if cta == "" {
		cta = "Xem thêm thông tin sản phẩm."
	}

	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		engine = EngineAuto
	}

	effectiveEngine := engine
	badge := getEngineBadge(engine)

	if engine == EngineAuto {
		nicheLower := strings.ToLower(niche)
		if strings.Contains(nicheLower, "mỹ phẩm") || strings.Contains(nicheLower, "làm đẹp") || strings.Contains(nicheLower, "da") || strings.Contains(nicheLower, "thời trang") {
			effectiveEngine = EngineClaude
			badge = "Tự động → Storytelling"
		} else if strings.Contains(nicheLower, "công nghệ") || strings.Contains(nicheLower, "điện tử") || strings.Contains(nicheLower, "trend") || strings.Contains(nicheLower, "drama") {
			effectiveEngine = EngineGrok
			badge = "Tự động → Hook rõ ràng"
		} else if strings.Contains(nicheLower, "gia dụng") || strings.Contains(nicheLower, "deal") || strings.Contains(nicheLower, "bán lẻ") {
			effectiveEngine = EngineChatGPT
			badge = "Tự động → Copy thương mại"
		} else {
			effectiveEngine = EngineGemini
			badge = "Tự động → Đạo diễn thị giác"
		}
	}

	// 1. Try real LLM generation if client is available (bounded to 12s so interactive UI never hangs)
	if llm != nil {
		llmCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		pkg, err := generateViaLLM(llmCtx, llm, effectiveEngine, badge, productName, niche, framework, targetAudience, keyBenefits, cta)
		cancel()
		if err == nil && len(pkg.Scenes) > 0 {
			pkg.AIEngine = effectiveEngine
			pkg.AIEngineBadge = badge
			return pkg
		}
	}

	// 2. High-quality Domain-Aware Fallback Engine with Niche & Persona Knowledge & QC
	pkg := generateDomainAwarePackage(effectiveEngine, badge, productName, niche, framework, targetAudience, keyBenefits, cta)
	pkg.AIEngine = effectiveEngine
	pkg.AIEngineBadge = badge
	return pkg
}

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
func evaluateQC(pkg AdScriptPackage) QCScorecard {
	hook := strings.TrimSpace(pkg.HookText)
	hookScore := 0
	if n := len([]rune(hook)); n >= 15 && n <= 120 {
		hookScore += 50
	}
	if mentionsAny(hook, productTerms(pkg.ProductName)) {
		hookScore += 50
	}

	var sceneText strings.Builder
	for _, scene := range pkg.Scenes {
		sceneText.WriteString(" ")
		sceneText.WriteString(strings.ToLower(strings.Join([]string{
			scene.SceneName, scene.VisualConcept, scene.GeminiPrompt, scene.MuseAIPrompt, scene.VoiceoverText,
		}, " ")))
	}
	nicheTerms := productTerms(pkg.Niche)
	nicheScore := 0
	if len(nicheTerms) > 0 {
		matched := 0
		for _, term := range nicheTerms {
			if strings.Contains(sceneText.String(), term) {
				matched++
			}
		}
		nicheScore = matched * 100 / len(nicheTerms)
	} else if mentionsAny(sceneText.String(), productTerms(pkg.ProductName)) {
		nicheScore = 100
	}

	persuasionScore := 0
	if strings.TrimSpace(pkg.CallToAction) != "" {
		persuasionScore += 50
	}
	spokenScenes := 0
	for _, scene := range pkg.Scenes {
		if strings.TrimSpace(scene.VoiceoverText) != "" {
			spokenScenes++
		}
	}
	if len(pkg.Scenes) > 0 {
		persuasionScore += spokenScenes * 50 / len(pkg.Scenes)
	}

	promptFields, promptFieldsPresent := 0, 0
	for _, scene := range pkg.Scenes {
		for _, field := range []string{scene.VisualConcept, scene.GeminiPrompt, scene.CameraMotion} {
			promptFields++
			if strings.TrimSpace(field) != "" {
				promptFieldsPresent++
			}
		}
	}
	avScore := 0
	if promptFields > 0 {
		avScore = promptFieldsPresent * 100 / promptFields
	}
	overall := (hookScore + nicheScore + persuasionScore + avScore) / 4
	verdict := "Còn thiếu trường kiểm tra"
	if overall >= 85 {
		verdict = "Đủ trường checklist"
	} else if overall < 50 {
		verdict = "Thiếu nhiều trường"
	}

	strengths := make([]string, 0, 4)
	if hookScore == 100 {
		strengths = append(strengths, "Hook có độ dài hợp lệ và nhắc sản phẩm")
	}
	if nicheScore >= 70 {
		strengths = append(strengths, "Các cảnh có từ khóa ngành hàng")
	}
	if strings.TrimSpace(pkg.CallToAction) != "" {
		strengths = append(strengths, "Có CTA trong kịch bản")
	}
	if avScore == 100 {
		strengths = append(strengths, "Mọi cảnh có visual concept, prompt ảnh và chuyển động camera")
	}
	recommendations := []string{
		"Checklist này chỉ kiểm tra văn bản kịch bản; chưa đánh giá video render, độ đúng sản phẩm/nhân vật, chất lượng hình/tiếng hoặc hiệu quả kênh.",
	}
	if hookScore < 100 {
		recommendations = append(recommendations, "Viết lại hook trong 15–120 ký tự và gọi đúng sản phẩm.")
	}
	if nicheScore < 70 {
		recommendations = append(recommendations, "Đưa chi tiết ngành hàng/sản phẩm vào hành động và prompt của các cảnh.")
	}
	if persuasionScore < 100 {
		recommendations = append(recommendations, "Bổ sung CTA hoặc lời thoại cho cảnh còn thiếu.")
	}
	if avScore < 100 {
		recommendations = append(recommendations, "Bổ sung visual concept, prompt ảnh và chuyển động camera cho từng cảnh.")
	}
	return QCScorecard{
		Scope:            "Script fields only",
		OverallScore:     overall,
		HookScore:        hookScore,
		NicheMatchScore:  nicheScore,
		PersuasionScore:  persuasionScore,
		AudioVisualScore: avScore,
		Verdict:          verdict,
		Summary:          fmt.Sprintf("Checklist đạt %d/100 theo 4 nhóm trường kịch bản; đây không phải điểm chất lượng video.", overall),
		Strengths:        strengths,
		Recommendations:  recommendations,
	}
}

func productTerms(text string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(text)) {
		term = strings.Trim(term, `.,!?;:"'()[]{}-/`)
		if len([]rune(term)) < 3 || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

func mentionsAny(text string, terms []string) bool {
	text = strings.ToLower(text)
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func fillComplementaryPackageFields(pkg *AdScriptPackage, keyBenefits string) {
	cleanTag := sanitizeTag(pkg.ProductName)
	cleanNiche := sanitizeTag(pkg.Niche)
	facts := strings.TrimSpace(keyBenefits)
	parts := []string{pkg.HookText}
	if facts != "" && !strings.Contains(facts, "Chưa có lợi ích") {
		parts = append(parts, facts)
	}
	if strings.TrimSpace(pkg.CallToAction) != "" {
		parts = append(parts, pkg.CallToAction)
	}
	pkg.Caption = strings.Join(parts, "\n\n")
	pkg.Hashtags = nonEmptyTags(cleanTag, cleanNiche)

	platforms := []struct {
		key   string
		name  string
		title string
	}{
		{key: "tiktok", name: "TikTok", title: pkg.ProductName},
		{key: "youtube_shorts", name: "YouTube Shorts", title: pkg.ProductName},
		{key: "facebook_reels", name: "Facebook Reels", title: pkg.ProductName},
		{key: "threads", name: "Threads", title: pkg.ProductName},
	}
	pkg.PlatformPosts = make(map[string]PlatformPost, len(platforms))
	for _, platform := range platforms {
		pkg.PlatformPosts[platform.key] = PlatformPost{
			Platform:     platform.name,
			Title:        platform.title,
			Caption:      pkg.Caption,
			Hashtags:     append([]string(nil), pkg.Hashtags...),
			CallToAction: pkg.CallToAction,
			Tip:          "Chỉ đăng thông tin khớp ảnh listing và mô tả đã xác nhận.",
		}
	}
	pkg.PsychologicalTriggers = nil
}

func nonEmptyTags(values ...string) []string {
	var tags []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			tags = append(tags, "#"+value)
		}
	}
	return tags
}

func sanitizeTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "&", "")
	return s
}
