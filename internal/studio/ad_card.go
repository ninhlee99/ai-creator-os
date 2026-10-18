package studio

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
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

func loadAndScalePhoto(photoPath string, targetW, targetH int) (*image.RGBA, error) {
	f, err := os.Open(photoPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	if srcW == 0 || srcH == 0 {
		return nil, fmt.Errorf("empty image bounds")
	}

	scaleX := float64(targetW) / float64(srcW)
	scaleY := float64(targetH) / float64(srcH)
	scale := scaleX
	if scaleY > scale {
		scale = scaleY
	}
	scaledW := int(float64(srcW)*scale + 0.5)
	scaledH := int(float64(srcH)*scale + 0.5)
	offsetX := (scaledW - targetW) / 2
	offsetY := (scaledH - targetH) / 2

	temp := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	xdraw.BiLinear.Scale(temp, temp.Bounds(), src, src.Bounds(), xdraw.Over, nil)

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.Draw(dst, dst.Bounds(), temp, image.Point{X: offsetX, Y: offsetY}, draw.Src)
	return dst, nil
}

func createLuxuryStudioBackdrop(w, h int, niche string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	nicheLower := strings.ToLower(niche)

	topCol := color.RGBA{24, 24, 27, 255}
	midCol := color.RGBA{39, 39, 42, 255}
	botCol := color.RGBA{9, 9, 11, 255}

	if strings.Contains(nicheLower, "thời trang") || strings.Contains(nicheLower, "phụ kiện") {
		topCol = color.RGBA{44, 30, 24, 255}
		midCol = color.RGBA{68, 48, 38, 255}
		botCol = color.RGBA{18, 12, 10, 255}
	} else if strings.Contains(nicheLower, "mỹ phẩm") || strings.Contains(nicheLower, "làm đẹp") {
		topCol = color.RGBA{64, 18, 38, 255}
		midCol = color.RGBA{98, 28, 58, 255}
		botCol = color.RGBA{24, 6, 14, 255}
	}

	hFloat := float64(h)
	for y := 0; y < h; y++ {
		t := float64(y) / hFloat
		var r, g, b uint8
		if t < 0.5 {
			s := t * 2.0
			r = uint8(float64(topCol.R)*(1-s) + float64(midCol.R)*s)
			g = uint8(float64(topCol.G)*(1-s) + float64(midCol.G)*s)
			b = uint8(float64(topCol.B)*(1-s) + float64(midCol.B)*s)
		} else {
			s := (t - 0.5) * 2.0
			r = uint8(float64(midCol.R)*(1-s) + float64(botCol.R)*s)
			g = uint8(float64(midCol.G)*(1-s) + float64(botCol.G)*s)
			b = uint8(float64(midCol.B)*(1-s) + float64(botCol.B)*s)
		}
		rowCol := color.RGBA{r, g, b, 255}
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, rowCol)
		}
	}
	return img
}

func applyCommercialVignette(img *image.RGBA, w, h int) {
	for y := 0; y < h; y++ {
		var alpha uint8
		if y < int(float64(h)*0.28) {
			factor := 1.0 - float64(y)/float64(float64(h)*0.28)
			alpha = uint8(factor * 200)
		} else if y > int(float64(h)*0.62) {
			factor := (float64(y) - float64(h)*0.62) / (float64(h) * 0.38)
			alpha = uint8(factor * 230)
		}
		if alpha > 0 {
			dimCol := color.RGBA{0, 0, 0, alpha}
			for x := 0; x < w; x++ {
				img.SetRGBA(x, y, blend(img.RGBAAt(x, y), dimCol))
			}
		}
	}
}

func drawBadgePill(img *image.RGBA, text string, centerX, centerY int, face font.Face, bgCol, textCol color.RGBA) {
	d := &font.Drawer{Face: face}
	txtW := d.MeasureString(text).Ceil()
	txtH := face.Metrics().Height.Ceil()

	padX := 32
	padY := 14
	pillW := txtW + padX*2
	pillH := txtH + padY*2
	pillX := centerX - pillW/2
	pillY := centerY - pillH/2

	drawRoundedRect(img, pillX, pillY, pillW, pillH, pillH/2, bgCol)
	drawRoundedBorder(img, pillX, pillY, pillW, pillH, pillH/2, 2, color.RGBA{255, 255, 255, 120})
	drawTextCentered(img, text, centerX, centerY+txtH/4, textCol, face)
}

func drawGradientButton(img *image.RGBA, x, y, w, h, radius int, col1, col2 color.RGBA) {
	for j := 0; j < h; j++ {
		t := float64(j) / float64(h)
		r := uint8(float64(col1.R)*(1-t) + float64(col2.R)*t)
		g := uint8(float64(col1.G)*(1-t) + float64(col2.G)*t)
		b := uint8(float64(col1.B)*(1-t) + float64(col2.B)*t)
		rowCol := color.RGBA{r, g, b, 255}

		for i := 0; i < w; i++ {
			inside := isInsideRoundedRect(i, j, w, h, radius)
			if inside {
				px := x + i
				py := y + j
				if px >= 0 && px < img.Bounds().Dx() && py >= 0 && py < img.Bounds().Dy() {
					img.SetRGBA(px, py, rowCol)
				}
			}
		}
	}
}

func isInsideRoundedRect(i, j, w, h, radius int) bool {
	if i < radius && j < radius {
		dx := radius - i
		dy := radius - j
		return (dx*dx + dy*dy) <= radius*radius
	} else if i > w-radius-1 && j < radius {
		dx := i - (w - radius - 1)
		dy := radius - j
		return (dx*dx + dy*dy) <= radius*radius
	} else if i < radius && j > h-radius-1 {
		dx := radius - i
		dy := j - (h - radius - 1)
		return (dx*dx + dy*dy) <= radius*radius
	} else if i > w-radius-1 && j > h-radius-1 {
		dx := i - (w - radius - 1)
		dy := j - (h - radius - 1)
		return (dx*dx + dy*dy) <= radius*radius
	}
	return true
}

func drawRoundedRect(img *image.RGBA, x, y, w, h, radius int, c color.RGBA) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			if isInsideRoundedRect(i, j, w, h, radius) {
				px := x + i
				py := y + j
				if px >= 0 && px < img.Bounds().Dx() && py >= 0 && py < img.Bounds().Dy() {
					img.SetRGBA(px, py, blend(img.RGBAAt(px, py), c))
				}
			}
		}
	}
}

func drawRoundedBorder(img *image.RGBA, x, y, w, h, radius, thickness int, c color.RGBA) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			if isInsideRoundedRect(i, j, w, h, radius) {
				// check if near boundary
				if !isInsideRoundedRect(i+thickness, j+thickness, w-2*thickness, h-2*thickness, radius-thickness) {
					px := x + i
					py := y + j
					if px >= 0 && px < img.Bounds().Dx() && py >= 0 && py < img.Bounds().Dy() {
						img.SetRGBA(px, py, blend(img.RGBAAt(px, py), c))
					}
				}
			}
		}
	}
}

func drawText(img *image.RGBA, text string, x, y int, c color.RGBA, face font.Face) {
	if face == nil {
		return
	}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}

func drawTextCentered(img *image.RGBA, text string, centerX, y int, c color.RGBA, face font.Face) {
	if face == nil {
		return
	}
	d := &font.Drawer{Face: face}
	txtW := d.MeasureString(text).Ceil()
	drawText(img, text, centerX-txtW/2, y, c, face)
}

func drawTextCenteredWithShadow(img *image.RGBA, text string, centerX, y int, c, shadow color.RGBA, face font.Face) {
	if face == nil {
		return
	}
	d := &font.Drawer{Face: face}
	txtW := d.MeasureString(text).Ceil()
	x := centerX - txtW/2
	drawText(img, text, x+2, y+2, shadow, face)
	drawText(img, text, x, y, c, face)
}
