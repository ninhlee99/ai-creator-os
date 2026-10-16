package studio

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
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
