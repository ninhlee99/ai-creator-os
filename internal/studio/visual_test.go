package studio

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	_ "golang.org/x/image/webp"
)

func TestGenerateProductVisual(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "visual-test.png")

	err := GenerateProductVisual("Tai Nghe Chống Ồn Cao Cấp", "công nghệ & điện tử", "PIN 30H - CHỐNG ỒN CHỦ ĐỘNG", 1080, 1920, out)
	if err != nil {
		t.Fatalf("GenerateProductVisual: %v", err)
	}

	info, err := os.Stat(out)
	if err != nil || info.Size() < 1000 {
		t.Fatalf("file output invalid: %v, size: %d", err, info.Size())
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	if img.Bounds().Dx() != 1080 || img.Bounds().Dy() != 1920 {
		t.Fatalf("bounds wrong: %v", img.Bounds())
	}
}

func TestDebugWebpPixels(t *testing.T) {
	path := "../../docs/assets/affiliate-spot/media-generation-shot-3-macro-0-81455621-54ab-4966-9915-e959ad6d4c83.webp"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Logf("format: %s, bounds: %v, color model: %T", format, img.Bounds(), img.ColorModel())
	r, g, b, a := img.At(100, 100).RGBA()
	t.Logf("pixel(100,100) RGBA: %d, %d, %d, %d", r>>8, g>>8, b>>8, a>>8)
}
