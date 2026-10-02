package tts

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func fakeWav(t *testing.T, fill byte, n int, rate int) []byte {
	t.Helper()
	pcm := bytes.Repeat([]byte{fill}, n)
	if rate == 0 {
		rate = 24000
	}
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+n))
	copy(hdr[8:], "WAVE")
	copy(hdr[12:], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1)
	binary.LittleEndian.PutUint16(hdr[22:], 1)
	binary.LittleEndian.PutUint32(hdr[24:], uint32(rate))
	binary.LittleEndian.PutUint32(hdr[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(hdr[32:], 2)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(n))
	return append(hdr, pcm...)
}

func TestConcatWavs(t *testing.T) {
	a := fakeWav(t, 0x11, 100, 0)
	b := fakeWav(t, 0x22, 200, 0)
	c := fakeWav(t, 0x33, 300, 0)
	got, err := ConcatWavs([][]byte{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	r, ch, bits, err := wavParams(got)
	if err != nil {
		t.Fatalf("joined header invalid: %v", err)
	}
	if r != 24000 || ch != 1 || bits != 16 {
		t.Fatalf("joined params wrong: %d/%d/%d", r, ch, bits)
	}
	off, err := wavDataOffset(got)
	if err != nil {
		t.Fatal(err)
	}
	data := got[off:]
	if len(data) != 600 {
		t.Fatalf("joined data len = %d, want 600", len(data))
	}
	// Order preserved: 0x11×100, 0x22×200, 0x33×300.
	for i, want := range []struct {
		fill  byte
		start int
		end   int
	}{{0x11, 0, 100}, {0x22, 100, 300}, {0x33, 300, 600}} {
		for j := want.start; j < want.end; j++ {
			if data[j] != want.fill {
				t.Fatalf("segment %d byte %d = %#x, want %#x", i, j, data[j], want.fill)
			}
		}
	}
}

func TestConcatWavsMismatch(t *testing.T) {
	a := fakeWav(t, 0x11, 100, 0)
	bad := fakeWav(t, 0x22, 100, 8000) // 8kHz ≠ 24kHz
	if _, err := ConcatWavs([][]byte{a, bad}); err == nil {
		t.Fatal("want error on param mismatch, got nil")
	}
}

func TestConcatWavsEmpty(t *testing.T) {
	if _, err := ConcatWavs(nil); err == nil {
		t.Fatal("want error on empty input, got nil")
	}
	if _, err := ConcatWavs([][]byte{[]byte("not a wav")}); err == nil {
		t.Fatal("want error on garbage chunk, got nil")
	}
}
