package studio

import (
	"os"
	"testing"
)

func TestComposeProductAdCard(t *testing.T) {
	out := "/tmp/test_ad_card.png"
	_ = os.Remove(out)

	opts := AdCardOptions{
		BgPhotoPath:   "../../data/output/studio/models/1/1790869026571752000.jpg",
		Niche:         "thời trang nữ & phụ kiện",
		TopBadge:      "✦ BST THỜI TRANG NỮ 2026 ✦",
		Title:         "ĐẦM DẠ HỘI LỤA SANG TRỌNG",
		Subtitle:      "Thiết kế tôn dáng chữ S · Lụa cao cấp",
		Features: []string{
			"✓ Chất lụa Satin mềm mịn",
			"✓ Tôn dáng quyến rũ",
			"✓ Tặng kèm túi xách đồng bộ",
		},
		RatingText:    "★★★★★ 4.9/5 · 2.4k+ Đã Bán",
		PriceText:     "890.000 ₫",
		OriginalPrice: "1.450.000 ₫",
		DiscountText:  "TIẾT KIỆM 40%",
		CTAText:       "MUA NGAY · GIỎ HÀNG GÓC TRÁI ➔",
		Width:         1080,
		Height:        1920,
	}

	if err := ComposeProductAdCard(opts, out); err != nil {
		t.Fatalf("ComposeProductAdCard: %v", err)
	}

	st, err := os.Stat(out)
	if err != nil || st.Size() < 50000 {
		t.Fatalf("invalid output: %v, size: %d", err, st.Size())
	}
}
