package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveMuseImageDataSupportsPNGJPEGAndWebP(t *testing.T) {
	for _, target := range []struct {
		name string
		ext  string
		mime string
		data []byte
	}{
		{"png", ".png", "image/png", []byte("\x89PNG\r\n\x1a\n12345678")},
		{"jpeg", ".jpg", "image/jpeg", []byte{0xff, 0xd8, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xd9}},
		{"webp", ".webp", "image/webp", []byte("RIFF0000WEBP")},
	} {
		t.Run(target.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "image"+target.ext)
			ext, err := saveMuseImageData(output, target.data, target.mime)
			if err != nil {
				t.Fatal(err)
			}
			if ext != target.ext {
				t.Fatalf("extension = %q, want %q", ext, target.ext)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(target.data) {
				t.Fatalf("saved image data differs from input")
			}
		})
	}
}
