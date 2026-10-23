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
	_ "golang.org/x/image/webp"
)

// GenerateCinematicStoryScene creates a vertical 9:16 (1080x1920) cinematic scene frame
// for YouTube romance/story videos without any text/captions (pure visual).
func GenerateCinematicStoryScene(sceneIdx, totalScenes int, prompt string, outPath string) error {
	return fmt.Errorf("không có ảnh minh họa phù hợp; hãy kết nối ImageGen hoặc tải ảnh tham chiếu lên")
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

// GenerateProductVisual creates a neutral product card from supplied text and
// an abstract background. It does not claim to depict the real product.
func GenerateProductVisual(productName, niche, subTitle string, width, height int, outPath string) error {
	return GenerateProductVisualWithPhoto("", productName, niche, subTitle, width, height, outPath)
}

// GenerateProductVisualWithPhoto creates a neutral product card. It never
// invents product claims or borrows an unrelated local photo.
func GenerateProductVisualWithPhoto(photoPath, productName, niche, subTitle string, width, height int, outPath string) error {
	if width <= 0 {
		width = 1080
	}
	if height <= 0 {
		height = 1920
	}
	if productName == "" {
		productName = "Sản phẩm"
	}
	if niche == "" {
		niche = "Sản phẩm"
	}

	badge := fmt.Sprintf("✦ %s ✦", strings.ToUpper(niche))
	if subTitle != "" && strings.HasPrefix(subTitle, "PHÂN CẢNH") {
		badge = fmt.Sprintf("✦ %s ✦", subTitle)
	}

	opts := AdCardOptions{
		BgPhotoPath: photoPath,
		Niche:       niche,
		TopBadge:    badge,
		Title:       strings.ToUpper(productName),
		Subtitle:    subTitle,
		Width:       width,
		Height:      height,
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
	drawFilledRect(img, x, y, w, thickness, c)             // top
	drawFilledRect(img, x, y+h-thickness, w, thickness, c) // bottom
	drawFilledRect(img, x, y, thickness, h, c)             // left
	drawFilledRect(img, x+w-thickness, y, thickness, h, c) // right
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
