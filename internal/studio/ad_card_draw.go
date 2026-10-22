package studio

import (
	"fmt"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"os"
	"strings"
)

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
