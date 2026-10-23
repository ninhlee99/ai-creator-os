package web

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestSanitizeBioJPEGMetadataPreservesImageICCAndOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 80, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	exif := append([]byte("Exif\x00\x00"), bioTIFFOrientationData(6, true)...)
	exif = append(exif, []byte("GPSLatitude=10.1")...)
	segments := [][]byte{
		bioJPEGExifOrientationSegment(6, true),
		bioTestJPEGSegment(0xe1, exif),
		bioTestJPEGSegment(0xe2, []byte("ICC_PROFILE\x00\x01\x01test profile")),
		bioTestJPEGSegment(0xed, []byte("Photoshop 3.0 IPTC author")),
		bioTestJPEGSegment(0xfe, []byte("private comment")),
	}
	var input bytes.Buffer
	input.Write(encoded.Bytes()[:2])
	for _, segment := range segments {
		input.Write(segment)
	}
	input.Write(encoded.Bytes()[2:])

	clean, err := stripBioJPEGMetadata(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{"GPSLatitude", "Photoshop 3.0", "private comment"} {
		if bytes.Contains(clean, []byte(privateValue)) {
			t.Errorf("sanitized JPEG retained %q", privateValue)
		}
	}
	if !bytes.Contains(clean, []byte("ICC_PROFILE\x00")) {
		t.Fatal("sanitized JPEG lost ICC color profile")
	}
	if !bytes.Contains(clean, bioJPEGExifOrientationSegment(6, true)) {
		t.Fatal("sanitized JPEG did not preserve orientation")
	}
	assertBioImagePixelsEqual(t, input.Bytes(), clean)
}

func TestSanitizeBioPNGMetadataPreservesImageColorAndOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 100, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	private := []byte("Author\x00Ninh GPS=10.1,106.1")
	var chunks bytes.Buffer
	bioWritePNGChunk(&chunks, "tEXt", private)
	bioWritePNGChunk(&chunks, "sRGB", []byte{0})
	bioWritePNGChunk(&chunks, "eXIf", append(bioTIFFOrientationData(6, true), []byte("GPSLatitude=10.1")...))
	input := make([]byte, 0, encoded.Len()+chunks.Len())
	input = append(input, encoded.Bytes()[:33]...)
	input = append(input, chunks.Bytes()...)
	input = append(input, encoded.Bytes()[33:]...)

	clean, err := stripBioPNGMetadata(input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(clean, []byte("Ninh GPS")) || bytes.Contains(clean, []byte("GPSLatitude")) {
		t.Fatal("sanitized PNG retained private text or EXIF data")
	}
	if !bytes.Contains(clean, []byte("sRGB")) {
		t.Fatal("sanitized PNG lost color-space metadata")
	}
	if !bytes.Contains(clean, []byte("eXIf")) || !bytes.Contains(clean, bioTIFFOrientationData(6, true)) {
		t.Fatal("sanitized PNG did not preserve orientation")
	}
	assertBioImagePixelsEqual(t, input, clean)
}

func bioTestJPEGSegment(marker byte, payload []byte) []byte {
	length := len(payload) + 2
	return append([]byte{0xff, marker, byte(length >> 8), byte(length)}, payload...)
}

func assertBioImagePixelsEqual(t *testing.T, before, after []byte) {
	t.Helper()
	original, _, err := image.Decode(bytes.NewReader(before))
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	clean, _, err := image.Decode(bytes.NewReader(after))
	if err != nil {
		t.Fatalf("decode sanitized: %v", err)
	}
	if original.Bounds() != clean.Bounds() {
		t.Fatalf("bounds changed: original %v, sanitized %v", original.Bounds(), clean.Bounds())
	}
	for y := original.Bounds().Min.Y; y < original.Bounds().Max.Y; y++ {
		for x := original.Bounds().Min.X; x < original.Bounds().Max.X; x++ {
			or, og, ob, oa := original.At(x, y).RGBA()
			cr, cg, cb, ca := clean.At(x, y).RGBA()
			if or != cr || og != cg || ob != cb || oa != ca {
				t.Fatalf("pixel changed at %d,%d", x, y)
			}
		}
	}
	if strings.TrimSpace(string(after)) == "" {
		t.Fatal("sanitized image is empty")
	}
}
