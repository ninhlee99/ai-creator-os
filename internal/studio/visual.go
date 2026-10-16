package studio

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// GenerateProductVisual creates a clean, high-resolution product showcase image
// using pure Go image synthesis. It ensures that video rendering and ad mockups
// always have high quality visual media even if external AI keys are unavailable.
func GenerateProductVisual(productName, niche, subTitle string, width, height int, outPath string) error {
	if width <= 0 {
		width = 1080
	}
	if height <= 0 {
		height = 1920
	}
	if productName == "" {
		productName = "Sản phẩm chất lượng cao"
	}
	if niche == "" {
		niche = "Thương mại điện tử"
	}

	// 1. Pick color theme based on niche
	nicheLower := strings.ToLower(niche)
	var (
		bgTop    color.RGBA
		bgMid    color.RGBA
		bgBot    color.RGBA
		accent   color.RGBA
		badgeBg  color.RGBA
	)

	switch {
	case strings.Contains(nicheLower, "công nghệ") || strings.Contains(nicheLower, "điện tử"):
		bgTop = color.RGBA{11, 19, 43, 255}    // Deep Navy
		bgMid = color.RGBA{28, 37, 65, 255}    // Indigo Blue
		bgBot = color.RGBA{10, 14, 26, 255}    // Midnight Dark
		accent = color.RGBA{56, 189, 248, 255} // Cyan
		badgeBg = color.RGBA{14, 116, 144, 255}
	case strings.Contains(nicheLower, "thời trang") || strings.Contains(nicheLower, "phụ kiện"):
		bgTop = color.RGBA{38, 28, 24, 255}    // Deep Espresso
		bgMid = color.RGBA{64, 46, 38, 255}    // Warm Bronze
		bgBot = color.RGBA{18, 14, 12, 255}    // Charcoal Dark
		accent = color.RGBA{251, 191, 36, 255} // Gold
		badgeBg = color.RGBA{180, 83, 9, 255}
	case strings.Contains(nicheLower, "mỹ phẩm") || strings.Contains(nicheLower, "làm đẹp"):
		bgTop = color.RGBA{76, 5, 25, 255}     // Deep Rosewood
		bgMid = color.RGBA{131, 24, 67, 255}   // Magenta Berry
		bgBot = color.RGBA{28, 3, 10, 255}     // Dark Cherry
		accent = color.RGBA{244, 114, 182, 255}// Soft Pink
		badgeBg = color.RGBA{190, 24, 93, 255}
	case strings.Contains(nicheLower, "gia dụng") || strings.Contains(nicheLower, "thông minh"):
		bgTop = color.RGBA{6, 78, 59, 255}     // Deep Emerald
		bgMid = color.RGBA{4, 120, 87, 255}    // Forest Green
		bgBot = color.RGBA{2, 44, 34, 255}     // Dark Pine
		accent = color.RGBA{52, 211, 153, 255} // Mint
		badgeBg = color.RGBA{5, 150, 105, 255}
	default:
		bgTop = color.RGBA{15, 23, 42, 255}    // Slate Dark
		bgMid = color.RGBA{30, 58, 138, 255}   // Royal Blue
		bgBot = color.RGBA{2, 6, 23, 255}      // Void Black
		accent = color.RGBA{96, 165, 250, 255} // Bright Blue
		badgeBg = color.RGBA{37, 99, 235, 255}
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 2. Render Vertical Smooth Gradient
	hFloat := float64(height)
	for y := 0; y < height; y++ {
		t := float64(y) / hFloat
		var r, g, b uint8
		if t < 0.5 {
			s := t * 2.0
			r = uint8(float64(bgTop.R)*(1-s) + float64(bgMid.R)*s)
			g = uint8(float64(bgTop.G)*(1-s) + float64(bgMid.G)*s)
			b = uint8(float64(bgTop.B)*(1-s) + float64(bgMid.B)*s)
		} else {
			s := (t - 0.5) * 2.0
			r = uint8(float64(bgMid.R)*(1-s) + float64(bgBot.R)*s)
			g = uint8(float64(bgMid.G)*(1-s) + float64(bgBot.G)*s)
			b = uint8(float64(bgMid.B)*(1-s) + float64(bgBot.B)*s)
		}
		rowColor := color.RGBA{r, g, b, 255}
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, rowColor)
		}
	}

	// 3. Decorative subtle background grid & ambient circular glow
	gridStep := 80
	gridCol := color.RGBA{255, 255, 255, 8}
	for x := 0; x < width; x += gridStep {
		for y := 0; y < height; y++ {
			img.SetRGBA(x, y, blend(img.RGBAAt(x, y), gridCol))
		}
	}
	for y := 0; y < height; y += gridStep {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, blend(img.RGBAAt(x, y), gridCol))
		}
	}

	// 4. Center Glassmorphic Product Showcase Card
	cardMarginX := int(float64(width) * 0.08)
	cardMarginY := int(float64(height) * 0.12)
	cardW := width - 2*cardMarginX
	cardH := height - 2*cardMarginY
	cardBg := color.RGBA{15, 23, 42, 220}
	borderCol := color.RGBA{accent.R, accent.G, accent.B, 180}

	drawFilledRect(img, cardMarginX, cardMarginY, cardW, cardH, cardBg)
	drawRectBorder(img, cardMarginX, cardMarginY, cardW, cardH, 4, borderCol)

	// Top accent glow line
	drawFilledRect(img, cardMarginX, cardMarginY, cardW, 8, accent)

	// 5. Draw Header Badges
	badgeY := cardMarginY + 50
	badgeW := 480
	badgeH := 48
	badgeX := (width - badgeW) / 2
	drawFilledRect(img, badgeX, badgeY, badgeW, badgeH, badgeBg)
	drawSimpleText(img, "✦ AI CREATIVE COMMERCIAL ✦", badgeX+36, badgeY+14, 2, color.RGBA{255, 255, 255, 255})

	// 6. Draw Product Showcase Frame (Inner Hero Box)
	heroBoxMargin := 60
	heroBoxX := cardMarginX + heroBoxMargin
	heroBoxY := badgeY + badgeH + 40
	heroBoxW := cardW - 2*heroBoxMargin
	heroBoxH := int(float64(cardH) * 0.46)
	heroBoxBg := color.RGBA{0, 0, 0, 160}
	drawFilledRect(img, heroBoxX, heroBoxY, heroBoxW, heroBoxH, heroBoxBg)
	drawRectBorder(img, heroBoxX, heroBoxY, heroBoxW, heroBoxH, 2, color.RGBA{255, 255, 255, 40})

	// Stylized Product Icon / Emblem in center of hero box
	iconSize := 160
	iconX := heroBoxX + (heroBoxW-iconSize)/2
	iconY := heroBoxY + (heroBoxH-iconSize)/2 - 30
	drawFilledRect(img, iconX, iconY, iconSize, iconSize, color.RGBA{accent.R, accent.G, accent.B, 40})
	drawRectBorder(img, iconX, iconY, iconSize, iconSize, 3, accent)
	drawSimpleText(img, "PROD", iconX+32, iconY+68, 3, accent)

	// Category tag under icon
	catLabel := fmt.Sprintf("[%s]", strings.ToUpper(niche))
	drawSimpleText(img, catLabel, heroBoxX+(heroBoxW-len(catLabel)*18)/2, iconY+iconSize+30, 2, color.RGBA{203, 213, 225, 255})

	// 7. Product Title & Descriptions below hero box
	textStartY := heroBoxY + heroBoxH + 40
	lines := wrapText(productName, 26)
	for i, l := range lines {
		if i >= 3 {
			break
		}
		textX := cardMarginX + heroBoxMargin
		drawSimpleText(img, strings.ToUpper(l), textX, textStartY+i*52, 4, color.RGBA{255, 255, 255, 255})
	}

	// Subtitle / USP Highlight
	uspY := textStartY + len(lines)*52 + 20
	uspText := subTitle
	if uspText == "" {
		uspText = "CHAT LUONG CAO - CHINH HANG 100%"
	}
	drawSimpleText(img, uspText, cardMarginX+heroBoxMargin, uspY, 2, accent)

	// 8. Bottom CTA Banner
	ctaH := 70
	ctaY := cardMarginY + cardH - ctaH - 40
	ctaW := cardW - 2*heroBoxMargin
	ctaX := cardMarginX + heroBoxMargin
	drawFilledRect(img, ctaX, ctaY, ctaW, ctaH, accent)
	drawSimpleText(img, "DAT MUA NGAY - UU DAI HOM NAY", ctaX+(ctaW-29*18)/2, ctaY+22, 2, color.RGBA{15, 23, 42, 255})

	// Ensure destination directory exists
	if dir := filepath.Dir(outPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

func blend(c1, c2 color.RGBA) color.RGBA {
	a2 := float64(c2.A) / 255.0
	a1 := 1.0 - a2
	return color.RGBA{
		R: uint8(float64(c1.R)*a1 + float64(c2.R)*a2),
		G: uint8(float64(c1.G)*a1 + float64(c2.G)*a2),
		B: uint8(float64(c1.B)*a1 + float64(c2.B)*a2),
		A: 255,
	}
}

func drawFilledRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	rect := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	draw.Draw(img, rect, &image.Uniform{c}, image.Point{}, draw.Over)
}

func drawRectBorder(img *image.RGBA, x, y, w, h, thickness int, c color.RGBA) {
	drawFilledRect(img, x, y, w, thickness, c)                     // top
	drawFilledRect(img, x, y+h-thickness, w, thickness, c)         // bottom
	drawFilledRect(img, x, y, thickness, h, c)                     // left
	drawFilledRect(img, x+w-thickness, y, thickness, h, c)         // right
}

func wrapText(s string, maxChars int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	curr := ""
	for _, w := range words {
		if curr == "" {
			curr = w
		} else if len(curr)+1+len(w) <= maxChars {
			curr += " " + w
		} else {
			lines = append(lines, curr)
			curr = w
		}
	}
	if curr != "" {
		lines = append(lines, curr)
	}
	return lines
}

// Simple 8x8 bitmap font renderer for crisp clean labels at scale
var font8x8 = map[rune][8]byte{
	'A': {0x00, 0x18, 0x3C, 0x66, 0x7E, 0x66, 0x66, 0x00},
	'B': {0x00, 0x7C, 0x66, 0x7C, 0x66, 0x66, 0x7C, 0x00},
	'C': {0x00, 0x3C, 0x66, 0x60, 0x60, 0x66, 0x3C, 0x00},
	'D': {0x00, 0x78, 0x6C, 0x66, 0x66, 0x6C, 0x78, 0x00},
	'E': {0x00, 0x7E, 0x60, 0x78, 0x60, 0x60, 0x7E, 0x00},
	'F': {0x00, 0x7E, 0x60, 0x78, 0x60, 0x60, 0x60, 0x00},
	'G': {0x00, 0x3C, 0x66, 0x60, 0x6E, 0x66, 0x3A, 0x00},
	'H': {0x00, 0x66, 0x66, 0x7E, 0x66, 0x66, 0x66, 0x00},
	'I': {0x00, 0x3C, 0x18, 0x18, 0x18, 0x18, 0x3C, 0x00},
	'J': {0x00, 0x1E, 0x0C, 0x0C, 0x0C, 0x6C, 0x38, 0x00},
	'K': {0x00, 0x66, 0x6C, 0x78, 0x6C, 0x66, 0x66, 0x00},
	'L': {0x00, 0x60, 0x60, 0x60, 0x60, 0x60, 0x7E, 0x00},
	'M': {0x00, 0x63, 0x77, 0x7F, 0x6B, 0x63, 0x63, 0x00},
	'N': {0x00, 0x66, 0x76, 0x7E, 0x6E, 0x66, 0x66, 0x00},
	'O': {0x00, 0x3C, 0x66, 0x66, 0x66, 0x66, 0x3C, 0x00},
	'P': {0x00, 0x7C, 0x66, 0x66, 0x7C, 0x60, 0x60, 0x00},
	'Q': {0x00, 0x3C, 0x66, 0x66, 0x66, 0x6E, 0x3C, 0x06},
	'R': {0x00, 0x7C, 0x66, 0x66, 0x7C, 0x6C, 0x66, 0x00},
	'S': {0x00, 0x3C, 0x66, 0x30, 0x18, 0x66, 0x3C, 0x00},
	'T': {0x00, 0x7E, 0x18, 0x18, 0x18, 0x18, 0x18, 0x00},
	'U': {0x00, 0x66, 0x66, 0x66, 0x66, 0x66, 0x3C, 0x00},
	'V': {0x00, 0x66, 0x66, 0x66, 0x66, 0x3C, 0x18, 0x00},
	'W': {0x00, 0x63, 0x63, 0x6B, 0x7F, 0x77, 0x63, 0x00},
	'X': {0x00, 0x66, 0x66, 0x3C, 0x18, 0x3C, 0x66, 0x00},
	'Y': {0x00, 0x66, 0x66, 0x3C, 0x18, 0x18, 0x18, 0x00},
	'Z': {0x00, 0x7E, 0x06, 0x0C, 0x18, 0x30, 0x7E, 0x00},
	'0': {0x00, 0x3C, 0x66, 0x6E, 0x76, 0x66, 0x3C, 0x00},
	'1': {0x00, 0x18, 0x38, 0x18, 0x18, 0x18, 0x7E, 0x00},
	'2': {0x00, 0x3C, 0x66, 0x0C, 0x18, 0x30, 0x7E, 0x00},
	'3': {0x00, 0x7E, 0x0C, 0x18, 0x0C, 0x66, 0x3C, 0x00},
	'4': {0x00, 0x0C, 0x1C, 0x3C, 0x6C, 0x7E, 0x0C, 0x00},
	'5': {0x00, 0x7E, 0x60, 0x7C, 0x06, 0x66, 0x3C, 0x00},
	'6': {0x00, 0x3C, 0x60, 0x7C, 0x66, 0x66, 0x3C, 0x00},
	'7': {0x00, 0x7E, 0x06, 0x0C, 0x18, 0x30, 0x30, 0x00},
	'8': {0x00, 0x3C, 0x66, 0x3C, 0x66, 0x66, 0x3C, 0x00},
	'9': {0x00, 0x3C, 0x66, 0x66, 0x3E, 0x06, 0x3C, 0x00},
	'-': {0x00, 0x00, 0x00, 0x7E, 0x00, 0x00, 0x00, 0x00},
	'+': {0x00, 0x18, 0x18, 0x7E, 0x18, 0x18, 0x00, 0x00},
	':': {0x00, 0x18, 0x18, 0x00, 0x18, 0x18, 0x00, 0x00},
	'[': {0x00, 0x3C, 0x30, 0x30, 0x30, 0x30, 0x3C, 0x00},
	']': {0x00, 0x3C, 0x0C, 0x0C, 0x0C, 0x0C, 0x3C, 0x00},
	'✦': {0x18, 0x3C, 0x7E, 0xFF, 0x7E, 0x3C, 0x18, 0x00},
	' ': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
}

func drawSimpleText(img *image.RGBA, text string, x, y, scale int, c color.RGBA) {
	text = stripAccents(text)
	curX := x
	for _, ch := range text {
		rows, ok := font8x8[ch]
		if !ok {
			rows = font8x8[' ']
		}
		for r := 0; r < 8; r++ {
			b := rows[r]
			for col := 0; col < 8; col++ {
				if (b & (0x80 >> col)) != 0 {
					drawFilledRect(img, curX+col*scale, y+r*scale, scale, scale, c)
				}
			}
		}
		curX += 9 * scale
	}
}

// stripAccents converts Vietnamese accented text to clean ASCII glyphs for bitmap font rendering
func stripAccents(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch r {
		case 'À', 'Á', 'Ả', 'Ã', 'Ạ', 'Ă', 'Ằ', 'Ắ', 'Ẳ', 'Ẵ', 'Ặ', 'Â', 'Ầ', 'Ấ', 'Ẩ', 'Ẫ', 'Ậ':
			sb.WriteRune('A')
		case 'Đ':
			sb.WriteRune('D')
		case 'È', 'É', 'Ẻ', 'Ẽ', 'Ẹ', 'Ê', 'Ề', 'Ế', 'Ể', 'Ễ', 'Ệ':
			sb.WriteRune('E')
		case 'Ì', 'Í', 'Ỉ', 'Ĩ', 'Ị':
			sb.WriteRune('I')
		case 'Ò', 'Ó', 'Ỏ', 'Õ', 'Ọ', 'Ô', 'Ồ', 'Ố', 'Ổ', 'Ỗ', 'Ộ', 'Ơ', 'Ờ', 'Ớ', 'Ở', 'Ỡ', 'Ợ':
			sb.WriteRune('O')
		case 'Ù', 'Ú', 'Ủ', 'Ũ', 'Ụ', 'Ư', 'Ừ', 'Ứ', 'Ử', 'Ữ', 'Ự':
			sb.WriteRune('U')
		case 'Ỳ', 'Ý', 'Ỷ', 'Ỹ', 'Ỵ':
			sb.WriteRune('Y')
		default:
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' || r == ':' || r == '[' || r == ']' || r == '✦' {
				sb.WriteRune(r)
			} else {
				sb.WriteRune(' ')
			}
		}
	}
	return sb.String()
}
