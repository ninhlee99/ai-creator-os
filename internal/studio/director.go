package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Director plans TikTok affiliate shots and source-grounded viral recreations.

// ---------------------------------------------------------------------------
// shot điện ảnh
// ---------------------------------------------------------------------------

// ProShot là một shot quay theo ngôn ngữ điện ảnh.
type ProShot struct {
	Index              int    `json:"index"`
	Purpose            string `json:"purpose"`     // hook | build | payoff …
	ShotSize           string `json:"shot_size"`   // ECU, CU, MCU, MS, WS, EWS, drone …
	CameraMove         string `json:"camera_move"` // dolly-in, tracking, handheld, static …
	LensLight          string `json:"lens_light"`  // 35mm f/1.8, golden hour …
	Seconds            int    `json:"seconds"`
	EmotionBeat        string `json:"emotion_beat"`                  // intended audience feeling for this beat
	WardrobeStyling    string `json:"wardrobe_styling"`              // campaign styling; consistent across shots
	SceneSetting       string `json:"scene_setting"`                 // campaign location; consistent across shots
	Action             string `json:"action"`                        // diễn xuất/hành động (tiếng Việt)
	ProductUseMode     string `json:"product_use_mode,omitempty"`    // wear | carry | apply | operate | prepare | consume | use
	ProductInteraction string `json:"product_interaction,omitempty"` // hành động có thật với sản phẩm
	ProductFocus       string `json:"product_focus,omitempty"`       // phần sản phẩm cần giữ rõ trong khung
	ImagePrompt        string `json:"image_prompt"`                  // prompt ảnh (tiếng Anh)
	VideoPrompt        string `json:"video_prompt"`                  // prompt video (tiếng Anh)
	TrailerWorthy      bool   `json:"trailer_worthy"`                // shot đắt giá → cắt trailer 9:16
}

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

func parseDirectorJSON(text string, v any) error {
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "```"))
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("director JSON: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Affiliate shot list
// ---------------------------------------------------------------------------

// WriteProShotList writes a TikTok affiliate shot list from product evidence.
func WriteProShotList(ctx context.Context, llm LLM, productName, niche string, seconds int) ([]ProShot, error) {
	return writeProShotList(ctx, llm, productName, niche, seconds, AffiliateProductProfile{}, ChannelCreativeProfile{}, false)
}

// WriteAffiliateShotList writes a shot plan grounded in a product analyst's
// visual brief. Unlike the legacy wrapper, it rejects plans without explicit
// product interaction, product framing, and required coverage.
func WriteAffiliateShotList(ctx context.Context, llm LLM, productName, niche string, seconds int, profile AffiliateProductProfile) ([]ProShot, error) {
	return WriteAffiliateShotListForChannel(ctx, llm, productName, niche, seconds, profile, ChannelCreativeProfile{})
}

// WriteAffiliateShotListForChannel adds the selected channel's saved brand
// guidance to the product-grounded shot plan.
func WriteAffiliateShotListForChannel(ctx context.Context, llm LLM, productName, niche string, seconds int, profile AffiliateProductProfile, channel ChannelCreativeProfile) ([]ProShot, error) {
	return WriteAffiliateShotListForChannelWithLearning(ctx, llm, productName, niche, seconds, profile, channel, "")
}

// WriteAffiliateShotListForChannelWithLearning adds channel-scoped performance
// lessons as weak creative guidance. Product evidence and physical use remain authoritative.
func WriteAffiliateShotListForChannelWithLearning(ctx context.Context, llm LLM, productName, niche string, seconds int, profile AffiliateProductProfile, channel ChannelCreativeProfile, learning string) ([]ProShot, error) {
	if !affiliateProductCategories[profile.Category] || strings.TrimSpace(profile.UseAction) == "" || strings.TrimSpace(profile.Focus) == "" {
		return nil, fmt.Errorf("director: product brief thiếu nhóm hàng, cách dùng hoặc điểm focus")
	}
	return writeProShotList(ctx, llm, productName, niche, seconds, profile, channel, true, learning)
}

func affiliateShotCount(seconds int) int {
	n := (seconds + 5) / 6
	if n < 5 {
		n = 5
	}
	if n > 30 {
		n = 30
	}
	return n
}

func normalizeChannelCreativeProfile(profile ChannelCreativeProfile) ChannelCreativeProfile {
	profile.Model = boundedPromptValue(profile.Model, 200)
	profile.Positioning = boundedPromptValue(profile.Positioning, 400)
	profile.Audience = boundedPromptValue(profile.Audience, 400)
	profile.Tone = boundedPromptValue(profile.Tone, 300)
	profile.VisualStyle = boundedPromptValue(profile.VisualStyle, 400)
	profile.RedLines = cleanBriefLines(profile.RedLines, 10)
	for i := range profile.RedLines {
		profile.RedLines[i] = boundedPromptValue(profile.RedLines[i], 240)
	}
	return profile
}

func boundedPromptValue(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}

func writeProShotList(ctx context.Context, llm LLM, productName, niche string, seconds int, profile AffiliateProductProfile, channel ChannelCreativeProfile, strict bool, learning ...string) ([]ProShot, error) {
	n := affiliateShotCount(seconds)
	sys := "Bạn lập shot list sản phẩm rõ ràng theo dữ kiện và ảnh tham chiếu. Chỉ trả lời JSON thuần."
	channel = normalizeChannelCreativeProfile(channel)
	channelJSON, err := json.Marshal(channel)
	if err != nil {
		return nil, fmt.Errorf("director: không mã hóa được hồ sơ kênh: %w", err)
	}
	channelProfileJSON := string(channelJSON)
	if channelProfileJSON == "{}" {
		channelProfileJSON = ""
	}
	channelLearning := ""
	if len(learning) > 0 {
		channelLearning = boundedPromptValue(learning[0], 2400)
	}
	prompt, err := directorPrompt("affiliate_shotlist.txt", promptData{
		Topic: productName, Niche: niche, Seconds: seconds, N: n,
		ChannelProfile:  channelProfileJSON,
		ChannelLearning: channelLearning,
		ProductCategory: profile.Category,
		ProductUse:      profile.UseAction,
		ProductUseNotes: strings.Join(profile.OperatorNotes, "; "),
		ProductFocus:    profile.Focus,
		ProductEvidence: strings.Join(profile.Evidence, "; "),
	})
	if err != nil {
		return nil, err
	}
	text, err := llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("director: %w", err)
	}
	var plan struct {
		SceneSetting    string    `json:"scene_setting"`
		WardrobeStyling string    `json:"wardrobe_styling"`
		Shots           []ProShot `json:"shots"`
	}
	if err := parseDirectorJSON(text, &plan); err != nil {
		return nil, err
	}
	if len(plan.Shots) < 4 {
		return nil, fmt.Errorf("director: chỉ có %d shot, cần >= 4", len(plan.Shots))
	}
	if strict && (strings.TrimSpace(plan.SceneSetting) == "" || strings.TrimSpace(plan.WardrobeStyling) == "") {
		return nil, fmt.Errorf("director: thiếu khóa bối cảnh hoặc styling chung cho chiến dịch")
	}
	if strict {
		minShots := (seconds + veoMaxSeconds - 1) / veoMaxSeconds
		maxShots := seconds / 4
		if len(plan.Shots) < minShots || len(plan.Shots) > maxShots {
			return nil, fmt.Errorf("director: %d shot không thể phủ %ds với clip 4-%ds", len(plan.Shots), seconds, veoMaxSeconds)
		}
	}
	baseSeconds := seconds / len(plan.Shots)
	remainder := seconds % len(plan.Shots)
	var out []ProShot
	macroCoverage, wideCoverage := false, false
	shotSizes := map[string]bool{}
	actions := map[string]bool{}
	for i, sh := range plan.Shots {
		if strings.TrimSpace(sh.Purpose) == "" ||
			strings.TrimSpace(sh.ShotSize) == "" ||
			strings.TrimSpace(sh.CameraMove) == "" ||
			strings.TrimSpace(sh.LensLight) == "" ||
			strings.TrimSpace(sh.Action) == "" ||
			strings.TrimSpace(sh.ImagePrompt) == "" ||
			strings.TrimSpace(sh.VideoPrompt) == "" {
			return nil, fmt.Errorf("director: shot %d thiếu chỉ đạo quay hoặc prompt chuyển động", i+1)
		}
		sh.Purpose = strings.ToLower(strings.TrimSpace(sh.Purpose))
		if strict {
			if strings.TrimSpace(sh.ProductInteraction) == "" || strings.TrimSpace(sh.ProductFocus) == "" || strings.TrimSpace(sh.EmotionBeat) == "" {
				return nil, fmt.Errorf("director: shot %d thiếu hành động sản phẩm, focus hoặc nhịp cảm xúc", i+1)
			}
			if !validAffiliateProductUseMode(profile.Category, sh.ProductUseMode) {
				return nil, fmt.Errorf("director: shot %d có product_use_mode %q không phù hợp nhóm %s", i+1, sh.ProductUseMode, profile.Category)
			}
			sh.SceneSetting = plan.SceneSetting
			sh.WardrobeStyling = plan.WardrobeStyling
			sh.ShotSize = normalizeAffiliateShotSize(sh.ShotSize)
			switch sh.Purpose {
			case "hook", "build", "macro", "lifestyle", "payoff":
			default:
				return nil, fmt.Errorf("director: shot %d có purpose không hợp lệ: %q", i+1, sh.Purpose)
			}
			macroCoverage = macroCoverage || sh.Purpose == "macro" || strings.EqualFold(sh.ShotSize, "ECU")
			wideCoverage = wideCoverage || strings.EqualFold(sh.ShotSize, "WS")
			shotSizes[strings.ToUpper(sh.ShotSize)] = true
			actionKey := strings.ToLower(strings.Join(strings.Fields(sh.Action), " "))
			if actions[actionKey] {
				return nil, fmt.Errorf("director: shot %d lặp lại hành động ở cảnh khác", i+1)
			}
			actions[actionKey] = true
		}
		sh.Seconds = baseSeconds
		if i < remainder {
			sh.Seconds++
		}
		if sh.Seconds < 4 || sh.Seconds > veoMaxSeconds {
			return nil, fmt.Errorf("director: %d shot không thể phủ chính xác %ds với giới hạn %d-%ds mỗi clip",
				len(plan.Shots), seconds, 4, veoMaxSeconds)
		}
		if strict && i == 0 && sh.Purpose != "hook" {
			return nil, fmt.Errorf("director: shot đầu phải là hook có sản phẩm đang được dùng")
		}
		if strict && i == len(plan.Shots)-1 && sh.Purpose != "payoff" {
			return nil, fmt.Errorf("director: shot cuối phải là payoff có sản phẩm vẫn nhìn rõ")
		}
		sh.Index = i + 1
		out = append(out, sh)
	}
	if strict && !macroCoverage {
		return nil, fmt.Errorf("director: shot list thiếu cận cảnh chi tiết sản phẩm có bằng chứng")
	}
	if strict && !wideCoverage {
		return nil, fmt.Errorf("director: shot list thiếu góc rộng để thấy sản phẩm trong cách dùng thực tế")
	}
	if strict && len(shotSizes) < 3 {
		return nil, fmt.Errorf("director: shot list cần ít nhất 3 cỡ cảnh khác nhau để tránh lặp bố cục")
	}
	return out, nil
}

func validAffiliateProductUseMode(category, mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch category {
	case "apparel", "footwear", "jewelry":
		return mode == "wear"
	case "bag":
		return mode == "carry" || mode == "wear"
	case "cosmetics":
		return mode == "apply"
	case "home":
		return mode == "operate"
	case "tech":
		return mode == "use" || mode == "wear" || mode == "operate"
	case "food":
		return mode == "prepare" || mode == "consume"
	case "other":
		return mode == "use"
	default:
		return false
	}
}

func normalizeAffiliateShotSize(size string) string {
	switch strings.ToUpper(strings.TrimSpace(size)) {
	case "WS", "WIDE", "WIDE SHOT", "WIDE (WS)":
		return "WS"
	case "MS", "MEDIUM", "MEDIUM SHOT", "MEDIUM (MS)":
		return "MS"
	case "MCU", "MEDIUM CLOSE-UP", "MEDIUM CLOSE UP":
		return "MCU"
	case "CU", "CLOSE-UP", "CLOSE UP":
		return "CU"
	case "ECU", "EXTREME CLOSE-UP", "EXTREME CLOSE UP", "MACRO":
		return "ECU"
	default:
		return strings.TrimSpace(size)
	}
}
