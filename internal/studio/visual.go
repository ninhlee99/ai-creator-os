package studio

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// GenerateCinematicStoryScene creates a widescreen 16:9 (1920x1080) cinematic scene frame
// for YouTube romance/story videos without any text/captions (pure visual).
func GenerateCinematicStoryScene(sceneIdx, totalScenes int, prompt string, outPath string) error {
	w, h := 1920, 1080
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	type palette struct {
		top, mid, bottom color.RGBA
		warmth           float64
	}
	moods := []palette{
		// Scene 1: Rainy moody dusk / melancholic intro (deep slate blues & navy)
		{color.RGBA{15, 23, 42, 255}, color.RGBA{30, 41, 59, 255}, color.RGBA{10, 15, 29, 255}, 0.2},
		// Scene 2: Penthouse cold distance / secret twilight (deep indigo & muted charcoal)
		{color.RGBA{23, 19, 38, 255}, color.RGBA{45, 36, 68, 255}, color.RGBA{15, 12, 28, 255}, 0.3},
		// Scene 3: Warm tender kitchen / candlelit soft glow (warm espresso & golden amber)
		{color.RGBA{44, 24, 16, 255}, color.RGBA{88, 47, 28, 255}, color.RGBA{24, 12, 8, 255}, 0.8},
		// Scene 4: Secret attic mystery / old parchment (warm sepia & moody bronze)
		{color.RGBA{38, 28, 20, 255}, color.RGBA{68, 50, 32, 255}, color.RGBA{20, 14, 10, 255}, 0.6},
		// Scene 5: Intense rainstorm confrontation (dramatic stormy deep cyan & dark graphite)
		{color.RGBA{12, 28, 36, 255}, color.RGBA{24, 52, 68, 255}, color.RGBA{8, 18, 24, 255}, 0.1},
		// Scene 6: Heartfelt resolution & eternal romance (rich golden hour & rose twilight)
		{color.RGBA{58, 24, 38, 255}, color.RGBA{110, 48, 62, 255}, color.RGBA{35, 14, 24, 255}, 0.9},
	}
	mIdx := sceneIdx % len(moods)
	mood := moods[mIdx]

	var photoPath string
	candidates := []string{
		"data/output/studio/models/1/1792293509522488000.jpeg",
		"data/output/studio/models/1/1792293509525665000.jpg",
		"data/output/studio/models/1/1792293509527521000.jpg",
		"data/output/studio/models/1/1792293509529067000.jpg",
	}
	if len(candidates) > 0 {
		idx := sceneIdx % len(candidates)
		if fileExists(candidates[idx]) {
			photoPath = candidates[idx]
		}
	}

	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		var c color.RGBA
		if t < 0.5 {
			s := t / 0.5
			c = color.RGBA{
				R: uint8(float64(mood.top.R)*(1-s) + float64(mood.mid.R)*s),
				G: uint8(float64(mood.top.G)*(1-s) + float64(mood.mid.G)*s),
				B: uint8(float64(mood.top.B)*(1-s) + float64(mood.mid.B)*s),
				A: 255,
			}
		} else {
			s := (t - 0.5) / 0.5
			c = color.RGBA{
				R: uint8(float64(mood.mid.R)*(1-s) + float64(mood.bottom.R)*s),
				G: uint8(float64(mood.mid.G)*(1-s) + float64(mood.bottom.G)*s),
				B: uint8(float64(mood.mid.B)*(1-s) + float64(mood.bottom.B)*s),
				A: 255,
			}
		}
		for x := 0; x < w; x++ {
			dst.SetRGBA(x, y, c)
		}
	}

	if photoPath != "" {
		if f, err := os.Open(photoPath); err == nil {
			if srcImg, _, err := image.Decode(f); err == nil {
				sb := srcImg.Bounds()
				sw, sh := sb.Dx(), sb.Dy()

				targetH := h
				targetW := int(float64(targetH) * float64(sw) / float64(sh))
				if targetW > w {
					targetW = w
				}

				var posX int
				if sceneIdx%2 == 0 {
					posX = w/2 - targetW/2
				} else {
					posX = (w * 2) / 3 - targetW / 2
				}

				scaledPhoto := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
				xdraw.BiLinear.Scale(scaledPhoto, scaledPhoto.Bounds(), srcImg, sb, xdraw.Over, nil)

				for py := 0; py < targetH; py++ {
					for px := 0; px < targetW; px++ {
						origColor := scaledPhoto.RGBAAt(px, py)
						edgeDist := float64(px)
						if float64(targetW-px) < edgeDist {
							edgeDist = float64(targetW - px)
						}
						featherWidth := 160.0
						alpha := 1.0
						if edgeDist < featherWidth {
							alpha = edgeDist / featherWidth
						}

						dx := posX + px
						if dx >= 0 && dx < w && py < h {
							bgCol := dst.RGBAAt(dx, py)
							tr := uint8(math.Min(255, float64(origColor.R)*(0.85+0.25*mood.warmth)))
							tg := uint8(math.Min(255, float64(origColor.G)*0.95))
							tb := uint8(math.Min(255, float64(origColor.B)*(1.05-0.15*mood.warmth)))
							blended := color.RGBA{
								R: uint8(float64(bgCol.R)*(1-alpha) + float64(tr)*alpha),
								G: uint8(float64(bgCol.G)*(1-alpha) + float64(tg)*alpha),
								B: uint8(float64(bgCol.B)*(1-alpha) + float64(tb)*alpha),
								A: 255,
							}
							dst.SetRGBA(dx, py, blended)
						}
					}
				}
			}
			f.Close()
		}
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cx, cy := float64(x-w/2)/(float64(w)/2), float64(y-h/2)/(float64(h)/2)
			r2 := cx*cx + cy*cy
			if r2 > 0.4 {
				factor := 1.0 - math.Min(0.5, (r2-0.4)*0.45)
				pix := dst.RGBAAt(x, y)
				dst.SetRGBA(x, y, color.RGBA{
					R: uint8(float64(pix.R) * factor),
					G: uint8(float64(pix.G) * factor),
					B: uint8(float64(pix.B) * factor),
					A: 255,
				})
			}
		}
	}

	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	outF, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outF.Close()
	return png.Encode(outF, dst)
}

// CreateCircularAvatarBadge creates a high-res circular avatar badge with a gradient ring
// for branding creator videos on TikTok / Reels.
func CreateCircularAvatarBadge(photoPath string, size int, outPath string) error {
	if size <= 0 {
		size = 140
	}
	f, err := os.Open(photoPath)
	if err != nil {
		return err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()

	// Square crop source from center
	minDim := sw
	if sh < minDim {
		minDim = sh
	}
	cropRect := image.Rect(
		sb.Min.X+(sw-minDim)/2,
		sb.Min.Y+(sh-minDim)/2,
		sb.Min.X+(sw-minDim)/2+minDim,
		sb.Min.Y+(sh-minDim)/2+minDim,
	)

	scaled := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), src, cropRect, xdraw.Over, nil)

	badge := image.NewRGBA(image.Rect(0, 0, size, size))
	radius := float64(size) / 2.0
	center := radius
	borderWidth := 3.5

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - center
			dy := float64(y) - center
			dist := math.Sqrt(dx*dx + dy*dy)

			if dist <= radius-borderWidth {
				badge.SetRGBA(x, y, scaled.RGBAAt(x, y))
			} else if dist <= radius {
				badge.SetRGBA(x, y, color.RGBA{R: 255, G: 215, B: 0, A: 255})
			} else if dist <= radius+1.0 {
				alpha := uint8(math.Max(0, (radius+1.0-dist)*255))
				badge.SetRGBA(x, y, color.RGBA{R: 255, G: 215, B: 0, A: alpha})
			} else {
				badge.SetRGBA(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
			}
		}
	}

	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	outF, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outF.Close()
	return png.Encode(outF, badge)
}

// GenerateProductVisual creates a clean, high-resolution product showcase image
// using pure Go image synthesis. It ensures that video rendering and ad mockups
// always have high quality visual media even if external AI keys are unavailable.
func GenerateProductVisual(productName, niche, subTitle string, width, height int, outPath string) error {
	return GenerateProductVisualWithPhoto("", productName, niche, subTitle, width, height, outPath)
}

// GenerateProductVisualWithPhoto creates a commercial ad card compositing a background photo
// (if available) with true TrueType typography, gold campaign badge, star ratings, and CTA button.
func GenerateProductVisualWithPhoto(photoPath, productName, niche, subTitle string, width, height int, outPath string) error {
	if width <= 0 {
		width = 1080
	}
	if height <= 0 {
		height = 1920
	}
	if productName == "" {
		productName = "Sản Phẩm Chất Lượng Cao"
	}
	if niche == "" {
		niche = "Thời trang & Phụ kiện"
	}

	// If photoPath is empty, try discovering any available model/product photo
	if photoPath == "" || !fileExists(photoPath) {
		candidates := []string{
			"data/output/studio/models/1/1790869026571752000.jpg",
			"data/output/studio/models/1/1790869026576011000.jpg",
			"/tmp/unsplash_dress.jpg",
			"/tmp/unsplash_jewelry.jpg",
		}
		for _, c := range candidates {
			if fileExists(c) {
				photoPath = c
				break
			}
		}
	}

	badge := fmt.Sprintf("✦ %s · BST MỚI 2026 ✦", strings.ToUpper(niche))
	if subTitle != "" && strings.HasPrefix(subTitle, "PHÂN CẢNH") {
		badge = fmt.Sprintf("✦ %s ✦", subTitle)
	}

	features := []string{
		"Chất liệu cao cấp, mềm mịn & thoáng mát",
		"Thiết kế tôn dáng thanh lịch, thời thượng",
		"Bảo hành chính hãng - Đổi trả 7 ngày",
	}

	opts := AdCardOptions{
		BgPhotoPath:   photoPath,
		Niche:         niche,
		TopBadge:      badge,
		Title:         strings.ToUpper(productName),
		Subtitle:      subTitle,
		Features:      features,
		RatingText:    "★★★★★ 4.9/5 · 2.4k+ Đã Bán",
		PriceText:     "890.000 ₫",
		OriginalPrice: "1.490.000 ₫",
		DiscountText:  "TIẾT KIỆM 40%",
		CTAText:       "MUA NGAY · GIỎ HÀNG GÓC TRÁI ➔",
		Width:         width,
		Height:        height,
	}

	return ComposeProductAdCard(opts, outPath)
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
