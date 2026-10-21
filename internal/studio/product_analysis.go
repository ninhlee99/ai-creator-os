package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AffiliateProductProfile is the product analyst's evidence-bound brief.
// It records visible facts and the physical interaction the shoot must show.
type AffiliateProductProfile struct {
	Category          string   `json:"category"`
	Item              string   `json:"item"`
	VisibleAttributes []string `json:"visible_attributes"`
	Evidence          []string `json:"evidence"`
	Confidence        string   `json:"confidence"`
	UseAction         string   `json:"use_action"`
	OperatorNotes     []string `json:"operator_notes,omitempty"`
	Focus             string   `json:"focus"`
}

var affiliateProductCategories = map[string]bool{
	"apparel": true, "footwear": true, "bag": true, "jewelry": true,
	"cosmetics": true, "home": true, "tech": true, "food": true, "other": true,
}

func normalizeAffiliateProductCategory(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "", "auto", "automatic", "tự động", "tự nhận diện":
		return ""
	case "clothing", "clothes", "apparel", "quần áo", "thời trang":
		return "apparel"
	case "shoe", "shoes", "footwear", "giày dép":
		return "footwear"
	case "bags", "bag", "túi", "túi xách":
		return "bag"
	case "jewellery", "jewelry", "trang sức":
		return "jewelry"
	case "beauty", "cosmetic", "cosmetics", "mỹ phẩm":
		return "cosmetics"
	case "home", "household", "đồ gia dụng", "nhà cửa":
		return "home"
	case "tech", "electronics", "điện tử", "công nghệ":
		return "tech"
	case "food", "đồ ăn", "thực phẩm":
		return "food"
	case "other", "khác":
		return "other"
	default:
		return "invalid"
	}
}

func affiliateProductReferences(p AffiliateParams) []ImageRef {
	paths := append([]string{p.ProductPhoto}, p.ExtraPhotos...)
	refs := make([]ImageRef, 0, 6)
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || !fileExists(path) || seen[path] {
			continue
		}
		seen[path] = true
		refs = append(refs, ImageRef{Path: path})
		if len(refs) == 6 {
			break
		}
	}
	return refs
}

func affiliateModelReferences(p AffiliateParams, max int) []ImageRef {
	paths := append([]string{p.ModelPhoto}, p.ModelPhotos...)
	refs := make([]ImageRef, 0, min(len(paths), max))
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		clean := filepath.Clean(path)
		if path == "" || !fileExists(path) || seen[clean] {
			continue
		}
		seen[clean] = true
		refs = append(refs, ImageRef{Path: path})
		if len(refs) == max {
			break
		}
	}
	return refs
}

func analyzeAffiliateProduct(ctx context.Context, analyzer visualImageAnalyzer, p AffiliateParams) (AffiliateProductProfile, error) {
	var empty AffiliateProductProfile
	refs := affiliateProductReferences(p)
	if len(refs) == 0 {
		return empty, fmt.Errorf("Product Analyst: thiếu ảnh listing sản phẩm đọc được")
	}
	categoryOverride := normalizeAffiliateProductCategory(p.ProductCategory)
	if categoryOverride == "invalid" {
		return empty, fmt.Errorf("Product Analyst: nhóm sản phẩm không hợp lệ: %q", p.ProductCategory)
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, `You are the product analyst for a commercial video shoot. Inspect only the product listing images that follow.
Product title: %q. Listing category: %q. Operator category override: %q. Operator use notes: %q. Requested product focus: %q.
Images are ordered listing references for the same exact item: image 1 is primary; later images are alternate listing angles/details.
Identify the item category and exact visible item. Record only attributes directly visible in these images. Do not infer fabric, material, performance, quality, price claims, health effects, or included accessories. If title and image conflict, mark product_match as mismatch. If item or category is unclear, use uncertain/unknown; do not guess.
Check whether requested focus is directly visible in at least one listing image. If not, mark requested_focus_support uncertain or mismatch and cite evidence for the selected focus.
Operator use notes are optional preferences only; they cannot override category-required physical use. Mark operator_use_support conflict if notes ask for posing beside or holding a product that must be worn or used.
Required use: apparel is worn on the character; footwear is worn on feet; bags are carried or worn; jewelry is worn; cosmetics are applied; home and tech products are operated; food is prepared or consumed as appropriate.
Return JSON only with requested_focus_support, selected_focus, focus_evidence, and operator_use_support: {"product_match":"match|mismatch|uncertain","category":"apparel|footwear|bag|jewelry|cosmetics|home|tech|food|other|unknown","item":"short visible item name","visible_attributes":["..."],"evidence":["what is visibly present in listing image"],"confidence":"high|medium|low","ambiguities":["..."],"requested_focus_support":"match|uncertain|mismatch","selected_focus":"visible feature","focus_evidence":"specific listing evidence","operator_use_support":"match|conflict|unspecified"}.`, p.ProductName, p.Niche, p.ProductCategory, p.ProductUsage, p.ProductFocus)
	raw, err := analyzer.AnalyzeImages(ctx, prompt.String(), refs)
	if err != nil {
		return empty, fmt.Errorf("Product Analyst: phân tích listing lỗi: %w", err)
	}
	var result struct {
		ProductMatch       string   `json:"product_match"`
		Category           string   `json:"category"`
		Item               string   `json:"item"`
		VisibleAttributes  []string `json:"visible_attributes"`
		Evidence           []string `json:"evidence"`
		Confidence         string   `json:"confidence"`
		Ambiguities        []string `json:"ambiguities"`
		FocusSupport       string   `json:"requested_focus_support"`
		SelectedFocus      string   `json:"selected_focus"`
		FocusEvidence      string   `json:"focus_evidence"`
		OperatorUseSupport string   `json:"operator_use_support"`
	}
	if err := parseDirectorJSON(raw, &result); err != nil {
		return empty, fmt.Errorf("Product Analyst: kết quả listing không phải JSON hợp lệ: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(result.ProductMatch), "match") {
		return empty, fmt.Errorf("Product Analyst: không xác nhận được đúng sản phẩm từ listing (%s); cần sửa ảnh/tên/nhóm sản phẩm", strings.TrimSpace(result.ProductMatch))
	}
	visionCategory := normalizeAffiliateProductCategory(result.Category)
	if visionCategory == "invalid" {
		visionCategory = ""
	}
	category := categoryOverride
	if category == "" {
		category = visionCategory
	} else if visionCategory != "" && visionCategory != category {
		return empty, fmt.Errorf("Product Analyst: nhóm người dùng chọn (%s) không khớp ảnh listing (%s)", category, visionCategory)
	}
	if category == "" {
		return empty, fmt.Errorf("Product Analyst: chưa phân loại chắc chắn sản phẩm; chọn nhóm hàng và nhập cách dùng chính xác")
	}
	if strings.EqualFold(strings.TrimSpace(result.OperatorUseSupport), "conflict") {
		return empty, fmt.Errorf("Product Analyst: ghi chú cách dùng mâu thuẫn với yêu cầu mặc/dùng trực tiếp của nhóm hàng; hãy sửa ghi chú")
	}
	confidence := strings.ToLower(strings.TrimSpace(result.Confidence))
	if confidence != "high" && confidence != "medium" {
		if categoryOverride == "" || strings.TrimSpace(p.ProductUsage) == "" {
			return empty, fmt.Errorf("Product Analyst: độ chắc chắn thấp; chọn nhóm hàng và nhập cách dùng chính xác")
		}
	}
	attrs := cleanBriefLines(result.VisibleAttributes, 8)
	evidence := cleanBriefLines(result.Evidence, 8)
	if len(evidence) == 0 {
		return empty, fmt.Errorf("Product Analyst: thiếu bằng chứng thị giác trong listing")
	}
	focus := strings.TrimSpace(p.ProductFocus)
	focusEvidence := strings.TrimSpace(result.FocusEvidence)
	if focus != "" && !strings.EqualFold(strings.TrimSpace(result.FocusSupport), "match") {
		return empty, fmt.Errorf("Product Analyst: điểm focus %q chưa được xác nhận trong ảnh listing; thêm ảnh phù hợp hoặc chọn điểm nhìn thấy", focus)
	}
	if focus == "" {
		focus = strings.TrimSpace(result.SelectedFocus)
	}
	if focus == "" {
		return empty, fmt.Errorf("Product Analyst: listing chưa cho thấy chi tiết để focus; thêm ảnh góc khác hoặc nhập điểm focus nhìn thấy")
	}
	if focusEvidence == "" {
		return empty, fmt.Errorf("Product Analyst: thiếu bằng chứng ảnh cho điểm focus")
	}
	evidence = cleanBriefLines(append(evidence, focusEvidence), 8)
	useAction := defaultAffiliateUseAction(category)
	var operatorNotes []string
	if note := strings.TrimSpace(p.ProductUsage); note != "" && category != "other" {
		if !strings.EqualFold(strings.TrimSpace(result.OperatorUseSupport), "match") {
			return empty, fmt.Errorf("Product Analyst: chưa xác nhận ghi chú cách dùng phù hợp với nhóm hàng")
		}
		operatorNotes = []string{note}
	}
	if category == "other" && strings.TrimSpace(p.ProductUsage) == "" {
		return empty, fmt.Errorf("Product Analyst: nhóm hàng khác cần nhập cách sử dụng chính xác")
	}
	if category == "other" {
		useAction = strings.TrimSpace(p.ProductUsage)
	}
	return AffiliateProductProfile{
		Category:          category,
		Item:              strings.TrimSpace(result.Item),
		VisibleAttributes: attrs,
		Evidence:          evidence,
		Confidence:        confidence,
		UseAction:         useAction,
		OperatorNotes:     operatorNotes,
		Focus:             focus,
	}, nil
}

func cleanBriefLines(values []string, max int) []string {
	clean := make([]string, 0, min(len(values), max))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, value)
		if len(clean) == max {
			break
		}
	}
	return clean
}

func defaultAffiliateUseAction(category string) string {
	switch category {
	case "apparel":
		return "Nhân vật mặc trực tiếp món đồ; cho thấy phom tổng thể, chuyển động vải tự nhiên và một thao tác có thật trên chi tiết đang nhìn thấy."
	case "footwear":
		return "Nhân vật mang đôi giày; quay rõ lúc bước đi và dáng giày trên chân, không chỉ cầm giày tạo dáng."
	case "bag":
		return "Nhân vật đeo hoặc xách túi theo kiểu thấy được trong listing; bước đi và thao tác với quai/túi tự nhiên. Chỉ mở hoặc lấy đồ nếu listing cho thấy cấu tạo phù hợp."
	case "jewelry":
		return "Nhân vật đeo trang sức đúng vị trí trên cơ thể; quay rõ khi đeo và trong chuyển động tự nhiên."
	case "cosmetics":
		return "Nhân vật trực tiếp thoa hoặc sử dụng mỹ phẩm đúng vị trí; chỉ thể hiện thao tác, không nói hiệu quả hay kết quả điều trị."
	case "home":
		return "Nhân vật thao tác món đồ trong bối cảnh sử dụng thật; không chỉ đặt sản phẩm trên bàn."
	case "tech":
		return "Nhân vật cầm, đeo hoặc thao tác thiết bị theo cấu tạo nhìn thấy; không bịa tính năng hay màn hình."
	case "food":
		return "Nhân vật cầm, chuẩn bị hoặc dùng sản phẩm theo cách ăn/uống nhìn thấy; không thêm tuyên bố dinh dưỡng hay sức khỏe."
	default:
		return "Nhân vật tương tác trực tiếp với đúng sản phẩm theo cách dùng do người vận hành mô tả."
	}
}

func writeAffiliateProductProfile(path string, profile AffiliateProductProfile) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
