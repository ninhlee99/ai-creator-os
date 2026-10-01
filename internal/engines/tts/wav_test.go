package tts

import (
	"testing"
)

func TestPCMToWavHeader(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04} // 2 samples s16le
	wav := pcmToWav(pcm, 24000)

	if len(wav) != 44+len(pcm) {
		t.Fatalf("len = %d, want %d", len(wav), 44+len(pcm))
	}
	if string(wav[0:4]) != "RIFF" {
		t.Fatalf("missing RIFF: %q", wav[0:4])
	}
	if string(wav[8:12]) != "WAVE" {
		t.Fatalf("missing WAVE: %q", wav[8:12])
	}
	if string(wav[12:16]) != "fmt " {
		t.Fatalf("missing fmt chunk: %q", wav[12:16])
	}
	if string(wav[36:40]) != "data" {
		t.Fatalf("missing data chunk: %q", wav[36:40])
	}
	// RIFF size = 36 + data
	if got := int(uint32(wav[4]) | uint32(wav[5])<<8 | uint32(wav[6])<<16 | uint32(wav[7])<<24); got != 36+len(pcm) {
		t.Fatalf("riff size = %d", got)
	}

	// Round-trip through the parser: 24kHz mono s16le.
	rate, ch, bits, err := wavParams(wav)
	if err != nil {
		t.Fatalf("wavParams: %v", err)
	}
	if rate != 24000 || ch != 1 || bits != 16 {
		t.Fatalf("params = %dHz ch=%d bits=%d, want 24000/1/16", rate, ch, bits)
	}
	// Payload intact.
	for i, b := range pcm {
		if wav[44+i] != b {
			t.Fatalf("payload byte %d = %x, want %x", i, wav[44+i], b)
		}
	}
}

func TestWavParamsRejectsGarbage(t *testing.T) {
	if _, _, _, err := wavParams([]byte("nope")); err == nil {
		t.Fatal("expected error for garbage")
	}
	if _, _, _, err := wavParams(make([]byte, 100)); err == nil {
		t.Fatal("expected error for non-WAV")
	}
}
