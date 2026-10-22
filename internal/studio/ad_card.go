package studio

import (
	"fmt"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
)

var (
	systemFontOnce sync.Once
	systemParsed   *opentype.Font
)

func getSystemFont() *opentype.Font {
	systemFontOnce.Do(func() {
		candidates := []string{
			"/Library/Fonts/Arial Unicode.ttf",
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Arial Bold.ttf",
			"/System/Library/Fonts/Helvetica.ttc",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		}
		for _, p := range candidates {
			if data, err := os.ReadFile(p); err == nil {
				if f, err := opentype.Parse(data); err == nil {
					systemParsed = f
					return
				}
			}
		}
		systemParsed, _ = opentype.Parse(gobold.TTF)
	})
	return systemParsed
}

func getFontFace(size float64) (font.Face, error) {
	f := getSystemFont()
	if f == nil {
		f, _ = opentype.Parse(gobold.TTF)
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

// AdCardOptions defines the text and visual components of a commercial ad slide.
type AdCardOptions struct {
	BgPhotoPath   string
	Niche         string
	TopBadge      string
	Title         string
	Subtitle      string
	Features      []string
	RatingText    string
	PriceText     string
	OriginalPrice string
	DiscountText  string
	CTAText       string
	Width         int
	Height        int
}

// ComposeProductAdCard renders a commercial-grade e-commerce promotional ad card.
func ComposeProductAdCard(opts AdCardOptions, outPath string) error {
	w := opts.Width
	if w <= 0 {
		w = 1080
	}
	h := opts.Height
	if h <= 0 {
		h = 1920
	}

	var canvas *image.RGBA

	// 1. Try loading real photo background if provided
	if opts.BgPhotoPath != "" && fileExists(opts.BgPhotoPath) {
		loaded, err := loadAndScalePhoto(opts.BgPhotoPath, w, h)
		if err == nil {
			canvas = loaded
		}
	}

	// 2. If no photo, generate a luxury studio backdrop
	if canvas == nil {
		canvas = createLuxuryStudioBackdrop(w, h, opts.Niche)
	}

	// 3. Apply cinematic vignette overlays for text legibility (top 28% and bottom 38%)
	applyCommercialVignette(canvas, w, h)

	// 4. Render Top Campaign / Badge Pill
	badgeFace, _ := getFontFace(26)
	titleFace, _ := getFontFace(46)
	subFace, _ := getFontFace(28)
	bodyFace, _ := getFontFace(26)
	priceFace, _ := getFontFace(54)
	ctaFace, _ := getFontFace(32)

	topBadge := opts.TopBadge
	if topBadge == "" {
		topBadge = "✦ BST THỜI TRANG CAO CẤP 2026 ✦"
	}

	// Draw top badge pill
	topMarginY := 120
	drawBadgePill(canvas, topBadge, w/2, topMarginY, badgeFace,
		color.RGBA{245, 158, 11, 240}, color.RGBA{255, 255, 255, 255})

	// 5. Draw Product Title (Bold, Center)
	title := opts.Title
	if title == "" {
		title = "ĐẦM DẠ HỘI LỤA SANG TRỌNG"
	}
	titleY := topMarginY + 85
	lines := wrapText(title, 22)
	for i, l := range lines {
		if i >= 2 {
			break
		}
		drawTextCenteredWithShadow(canvas, l, w/2, titleY+i*55,
			color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 220}, titleFace)
	}

	// Subtitle under title
	subY := titleY + len(lines)*55 + 10
	if opts.Subtitle != "" {
		drawTextCenteredWithShadow(canvas, opts.Subtitle, w/2, subY,
			color.RGBA{253, 230, 138, 255}, color.RGBA{0, 0, 0, 220}, subFace)
	}

	// 6. Draw Middle Feature Bullets if provided (or floating badges)
	featY := 480
	if len(opts.Features) > 0 {
		for i, ft := range opts.Features {
			if i >= 3 {
				break
			}
			pillW := 460
			pillH := 52
			pillX := 80
			pillCurY := featY + i*68
			drawRoundedRect(canvas, pillX, pillCurY, pillW, pillH, 26, color.RGBA{15, 23, 42, 190})
			drawRoundedBorder(canvas, pillX, pillCurY, pillW, pillH, 26, 2, color.RGBA{245, 158, 11, 160})
			drawText(canvas, ft, pillX+24, pillCurY+36, color.RGBA{255, 255, 255, 255}, bodyFace)
		}
	}

	// 7. Bottom Section: Social Proof, Price & CTA
	bottomAreaY := int(float64(h) * 0.72)

	// Rating & Social Proof
	rating := opts.RatingText
	if rating == "" {
		rating = "★★★★★ 4.9/5 · Đã Bán 2.4k+"
	}
	drawBadgePill(canvas, rating, w/2, bottomAreaY, bodyFace,
		color.RGBA{30, 41, 59, 210}, color.RGBA{251, 191, 36, 255})

	// Price Block
	priceY := bottomAreaY + 80
	priceText := opts.PriceText
	if priceText == "" {
		priceText = "890.000 ₫"
	}
	drawTextCenteredWithShadow(canvas, priceText, w/2, priceY,
		color.RGBA{251, 191, 36, 255}, color.RGBA{0, 0, 0, 240}, priceFace)

	// Original Price & Discount Pill
	discountY := priceY + 45
	if opts.DiscountText != "" || opts.OriginalPrice != "" {
		tag := opts.DiscountText
		if tag == "" {
			tag = "TIẾT KIỆM 40%"
		}
		if opts.OriginalPrice != "" {
			tag = fmt.Sprintf("Gốc: %s · %s", opts.OriginalPrice, tag)
		}
		drawTextCenteredWithShadow(canvas, tag, w/2, discountY,
			color.RGBA{203, 213, 225, 255}, color.RGBA{0, 0, 0, 220}, subFace)
	}

	// Bottom CTA Button
	ctaText := opts.CTAText
	if ctaText == "" {
		ctaText = "👉 BẤM VÀO LINK BIO ĐỂ SĂN DEAL"
	}
	ctaY := h - 160
	ctaW := w - 160
	ctaH := 84
	ctaX := 80
	// Pulsing bright red/amber gradient CTA button
	drawGradientButton(canvas, ctaX, ctaY, ctaW, ctaH, 20,
		color.RGBA{239, 68, 68, 255}, color.RGBA{220, 38, 38, 255})
	drawRoundedBorder(canvas, ctaX, ctaY, ctaW, ctaH, 20, 3, color.RGBA{254, 202, 202, 200})
	drawTextCenteredWithShadow(canvas, ctaText, w/2, ctaY+54,
		color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 180}, ctaFace)

	// Ensure target directory exists
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, canvas)
}
