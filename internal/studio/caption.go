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
	prompt := fmt.Sprintf(`Viết caption tiếng Việt cho video affiliate TikTok 30 giây, phong cách tự nhiên, chuyên nghiệp.
Sản phẩm: %s. Ngành hàng: %s.
Yêu cầu:
- Dòng 1: hook ngắn gây tò mò (dưới 60 ký tự).
- Dòng 2: 1 lợi ích thật của sản phẩm (không bịa đặt, không "rẻ nhất thị trường").
- Dòng 3: CTA nhẹ nhàng trỏ tới link ở bio/mô tả ("Link mua ở bio/mô tả nha"). KHÔNG nhắc giỏ hàng/TikTok Shop (video không gắn giỏ).
- Cuối: 5-8 hashtag: 1-2 hashtag tên/loại sản phẩm cụ thể, 2-3 hashtag ngành hàng, #xuhuong. Không dùng #tiktokshop.
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

// nicheTags: hashtag theo nhóm sản phẩm cho caption mẫu.
var nicheTags = map[string]string{
	"thời trang/phụ kiện": "#thoitrang #outfit #ootd",
	"mỹ phẩm":             "#mypham #skincare #lamdep",
	"gia dụng":            "#giadung #nhacuadep #tienich",
	"công nghệ":           "#congnghe #review #gadget",
}

// captionHooks: câu mở xoay vòng theo sản phẩm (không lặp 1 mẫu cho mọi
// video). %s = tên sản phẩm. Không bịa thông số/giá.
var captionHooks = []string{
	"%s — xem tới cuối để thấy lý do ai cũng hỏi link 👀",
	"Mình đã thử %s và đây là cảm nhận thật",
	"%s: đáng tiền hay không? Xem nhanh 15 giây",
	"Ai đang tìm %s thì xem cái này trước khi mua",
	"%s — chi tiết nhỏ mà ít ai để ý",
}

var vnFold = strings.NewReplacer(
	"à", "a", "á", "a", "ả", "a", "ã", "a", "ạ", "a", "ă", "a", "ằ", "a", "ắ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
	"â", "a", "ầ", "a", "ấ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a", "đ", "d",
	"è", "e", "é", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e", "ê", "e", "ề", "e", "ế", "e", "ể", "e", "ễ", "e", "ệ", "e",
	"ì", "i", "í", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
	"ò", "o", "ó", "o", "ỏ", "o", "õ", "o", "ọ", "o", "ô", "o", "ồ", "o", "ố", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
	"ơ", "o", "ờ", "o", "ớ", "o", "ở", "o", "ỡ", "o", "ợ", "o",
	"ù", "u", "ú", "u", "ủ", "u", "ũ", "u", "ụ", "u", "ư", "u", "ừ", "u", "ứ", "u", "ử", "u", "ữ", "u", "ự", "u",
	"ỳ", "y", "ý", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y")

// productTag: hashtag tìm kiếm từ tên sản phẩm ("Túi kem quilted" →
// "#tuikemquilted"), tối đa 3 từ đầu cho gọn. "" khi tên rỗng.
func productTag(name string) string {
	words := strings.Fields(vnFold.Replace(strings.ToLower(name)))
	var b strings.Builder
	for i, w := range words {
		if i >= 3 {
			break
		}
		for _, r := range w {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
	}
	if b.Len() < 3 {
		return ""
	}
	return "#" + b.String()
}

// TemplateCaption là caption dự phòng khi không có LLM: câu mở tò mò (xoay
// theo sản phẩm) + kêu gọi hành động + hashtag tên sản phẩm và nhóm hàng.
// Không bịa thông số sản phẩm.
func TemplateCaption(productName, niche string) string {
	name := strings.TrimSpace(productName)
	if name == "" {
		name = "Món này"
	}
	h := 0
	for _, r := range name {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	hook := fmt.Sprintf(captionHooks[h%len(captionHooks)], name)
	tags := nicheTags[strings.ToLower(strings.TrimSpace(niche))]
	if tags == "" {
		tags = "#review #muasam"
	}
	if pt := productTag(name); pt != "" {
		tags = pt + " " + tags
	}
	return hook + "\nLink mua ở bio/mô tả nhé!\n" + tags + " #xuhuong"
}
