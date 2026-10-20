package flow

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Compiler converts natural language user requests into optimized ComfyUI-like Graphs.
type Compiler struct {
	registry *Registry
}

// NewCompiler creates a compiler with the global or custom registry.
func NewCompiler(reg *Registry) *Compiler {
	if reg == nil {
		reg = GlobalRegistry()
	}
	return &Compiler{registry: reg}
}

// Compile analyzes the prompt and returns a tailored Graph.
func (c *Compiler) Compile(ctx context.Context, prompt string, extraParams map[string]interface{}) (*Graph, error) {
	promptLower := strings.ToLower(strings.TrimSpace(prompt))

	if extraParams == nil {
		extraParams = make(map[string]interface{})
	}

	// 1. Detect Intent / Domain
	isCinemaDrama := strings.Contains(promptLower, "phim") ||
		strings.Contains(promptLower, "vi kịch") ||
		strings.Contains(promptLower, "drama") ||
		strings.Contains(promptLower, "kling") ||
		strings.Contains(promptLower, "hailuo") ||
		strings.Contains(promptLower, "trung quốc") ||
		strings.Contains(promptLower, "điện ảnh") ||
		strings.Contains(promptLower, "kiếm hiệp")

	isDanceTrend := strings.Contains(promptLower, "nhảy") ||
		strings.Contains(promptLower, "vũ đạo") ||
		strings.Contains(promptLower, "dance") ||
		strings.Contains(promptLower, "sexy") ||
		strings.Contains(promptLower, "douyin") ||
		strings.Contains(promptLower, "trendjack") ||
		strings.Contains(promptLower, "đào lê phương hoa")

	isStory := strings.Contains(promptLower, "truyện") ||
		strings.Contains(promptLower, "kể chuyện") ||
		strings.Contains(promptLower, "audio") ||
		strings.Contains(promptLower, "tts") ||
		strings.Contains(promptLower, "giọng đọc") ||
		strings.Contains(promptLower, "lồng tiếng") ||
		strings.Contains(promptLower, "tình yêu") ||
		strings.Contains(promptLower, "ngôn tình") ||
		strings.Contains(promptLower, "anime") ||
		strings.Contains(promptLower, "makoto") ||
		strings.Contains(promptLower, "kinh dị") ||
		strings.Contains(promptLower, "tiên hiệp") ||
		strings.Contains(promptLower, "trinh thám") ||
		strings.Contains(promptLower, "radio") ||
		strings.Contains(promptLower, "tâm linh")

	isLookbook := strings.Contains(promptLower, "thay đồ") ||
		strings.Contains(promptLower, "phối đồ") ||
		strings.Contains(promptLower, "lookbook") ||
		strings.Contains(promptLower, "thời trang") ||
		strings.Contains(promptLower, "affiliate") ||
		strings.Contains(promptLower, "dễ thương") ||
		strings.Contains(promptLower, "xinh xắn") ||
		strings.Contains(promptLower, "try on") ||
		strings.Contains(promptLower, "try-on") ||
		strings.Contains(promptLower, "outfit") ||
		strings.Contains(promptLower, "váy") ||
		strings.Contains(promptLower, "áo")

	var g *Graph
	var err error

	switch {
	case isCinemaDrama:
		extraParams["prompt"] = prompt
		genre := "tien_hiep"
		if strings.Contains(promptLower, "trinh thám") {
			genre = "trinh_tham"
		} else if strings.Contains(promptLower, "kiếm hiệp") {
			genre = "kiem_hiep"
		} else if strings.Contains(promptLower, "đô thị") {
			genre = "do_thi"
		}
		extraParams["genre"] = genre
		g, err = BuildRecipe(RecipeKeyChineseCinemaDrama, extraParams)
		if err == nil {
			g.Title = fmt.Sprintf("Phim AI Điện Ảnh: %s", truncateText(prompt, 40))
		}
	case isLookbook:
		extraParams["user_prompt"] = prompt
		g, err = BuildRecipe(RecipeKeyLookbookOutfit, extraParams)
		if err == nil {
			g.Title = fmt.Sprintf("Affiliate Thời Trang Nữ: %s", truncateText(prompt, 40))
		}
	case isDanceTrend:
		extraParams["user_prompt"] = prompt
		g, err = BuildRecipe(RecipeKeyDanceTrendjack, extraParams)
		if err == nil {
			g.Title = fmt.Sprintf("Vũ Đạo Sexy Dance: %s", truncateText(prompt, 40))
		}
	case isStory:
		extraParams["topic"] = prompt
		// Extract genre if present
		genre := "tinh_yeu"
		if strings.Contains(promptLower, "kinh dị") || strings.Contains(promptLower, "bóng đè") {
			genre = "kinh_di"
		} else if strings.Contains(promptLower, "tiên hiệp") || strings.Contains(promptLower, "tu tiên") {
			genre = "tien_hiep"
		} else if strings.Contains(promptLower, "trinh thám") {
			genre = "trinh_tham"
		}
		extraParams["genre"] = genre
		g, err = BuildRecipe(RecipeKeyAudioStoryCinema, extraParams)
		if err == nil {
			g.Title = fmt.Sprintf("Truyện Tình Yêu Anime: %s", truncateText(prompt, 40))
		}
	case isLookbook:
		extraParams["user_prompt"] = prompt
		g, err = BuildRecipe(RecipeKeyLookbookOutfit, extraParams)
		if err == nil {
			g.Title = fmt.Sprintf("Lookbook Phối Đồ: %s", truncateText(prompt, 40))
		}
	default:
		// Default to Dance Trendjack with user prompt if generic
		extraParams["user_prompt"] = prompt
		g, err = BuildRecipe(RecipeKeyDanceTrendjack, extraParams)
		if err == nil {
			g.Title = fmt.Sprintf("Quy trình Tạo Video: %s", truncateText(prompt, 40))
		}
	}

	if err != nil {
		return nil, fmt.Errorf("biên dịch graph thất bại: %w", err)
	}

	g.Prompt = prompt
	g.AppendLog(fmt.Sprintf("[%s] Đã nhận lệnh tự nhiên: \"%s\"", time.Now().Format("15:04:05"), prompt))
	g.AppendLog(fmt.Sprintf("[%s] Đã phân tích ý định và biên dịch thành công đồ thị gồm %d Node, %d Dây kết nối.", time.Now().Format("15:04:05"), len(g.Nodes), len(g.Wires)))

	return g, nil
}

func truncateText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "..."
}
