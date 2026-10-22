package webai

import (
	"context"
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
