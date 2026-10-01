package studio

import (
	"context"
	"fmt"
	"strings"
)

// WriteCaption asks the LLM for a Vietnamese TikTok caption + hashtags for
// an affiliate video. Style: short hook line, one honest benefit line, soft
// CTA, then 5-8 relevant hashtags. No false claims (no "rẻ nhất", no fake
// discounts), no emoji spam.
func WriteCaption(ctx context.Context, llm LLM, productName, niche string) (string, error) {
	if llm == nil {
		return "", fmt.Errorf("caption: no LLM")
	}
	if strings.TrimSpace(productName) == "" {
		productName = "sản phẩm"
	}
	sys := "Bạn là copywriter TikTok Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	prompt := fmt.Sprintf(`Viết caption tiếng Việt cho video affiliate TikTok 30 giây, phong cách giật-giật.
Sản phẩm: %s. Ngành hàng: %s.
Yêu cầu:
- Dòng 1: hook ngắn gây tò mò (dưới 60 ký tự).
- Dòng 2: 1 lợi ích thật của sản phẩm (không bịa đặt, không "rẻ nhất thị trường").
- Dòng 3: CTA nhẹ nhàng ("Link giỏ hàng ở video nha", "Xem giỏ hàng để biết thêm chi tiết").
- Cuối: 5-8 hashtag liên quan (có #affiliate hoặc #tiktokshop, #xuhuong, hashtag theo ngành hàng).
- Tổng dưới 300 ký tự. Không emoji quá 3 cái.
Chỉ trả JSON: {"caption": "<caption đầy đủ, xuống dòng bằng \\n>"}`, productName, niche)
	text, err := llm.Complete(ctx, sys, prompt)
	if err != nil {
		return "", fmt.Errorf("caption: %w", err)
	}
	var out struct {
		Caption string `json:"caption"`
	}
	if err := parseDirectorJSON(text, &out); err != nil {
		return "", fmt.Errorf("caption: %w", err)
	}
	out.Caption = strings.TrimSpace(out.Caption)
	if out.Caption == "" {
		return "", fmt.Errorf("caption: empty")
	}
	return out.Caption, nil
}
