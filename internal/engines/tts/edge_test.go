package tts

import (
	"net"
	"strings"
	"testing"
)

// No live network in these tests: they exercise the pure framing helpers and
// a loopback frame round-trip.

func TestExtractEdgeAudio(t *testing.T) {
	frame := []byte("Path:audio\r\nContent-Type: audio/mpeg\r\n\r\nMP3DATA")
	if got := extractEdgeAudio(frame); string(got) != "MP3DATA" {
		t.Fatalf("got %q", got)
	}
	// single-CRLF variant
	frame2 := []byte("Path:audio\r\nMP3DATA2")
	if got := extractEdgeAudio(frame2); string(got) != "MP3DATA2" {
		t.Fatalf("got %q", got)
	}
	if got := extractEdgeAudio([]byte("Path:turn.end\r\n\r\n")); got != nil {
		t.Fatalf("non-audio frame returned %q", got)
	}
	if got := extractEdgeAudio([]byte("hello")); got != nil {
		t.Fatalf("garbage returned %q", got)
	}
}

func TestEdgeSSMLEscapes(t *testing.T) {
	// xmlEscaper must neutralize markup like Python's saxutils.escape.
	in := `a & b <c> "d"`
	out := xmlEscaper.Replace(in)
	if strings.Contains(out, "<c>") || !strings.Contains(out, "&amp;") || !strings.Contains(out, "&lt;") {
		t.Fatalf("escaped = %q", out)
	}
}

func TestEdgeWSFrameRoundTrip(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	wa := &edgeWS{conn: a}
	wb := &edgeWS{conn: b}

	done := make(chan error, 1)
	go func() {
		done <- wb.sendText("xin chào")
	}()
	kind, data, err := wa.recvMessage()
	if serr := <-done; serr != nil {
		t.Fatalf("send: %v", serr)
	}
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if kind != "text" || string(data) != "xin chào" {
		t.Fatalf("kind=%q data=%q", kind, data)
	}

	// binary frame carrying an audio message
	go func() {
		done <- wb.sendFrame(0x2, []byte("Path:audio\r\n\r\nMP3"))
	}()
	kind, data, err = wa.recvMessage()
	if serr := <-done; serr != nil {
		t.Fatalf("send binary: %v", serr)
	}
	if err != nil {
		t.Fatalf("recv binary: %v", err)
	}
	if kind != "binary" {
		t.Fatalf("kind=%q", kind)
	}
	if got := extractEdgeAudio(data); string(got) != "MP3" {
		t.Fatalf("audio = %q", got)
	}
}

func TestEdgeVoiceMap(t *testing.T) {
	if edgeVoices["default"] != "vi-VN-HoaiMyNeural" {
		t.Fatalf("default = %q", edgeVoices["default"])
	}
	if edgeVoices["male"] != "vi-VN-NamMinhNeural" {
		t.Fatalf("male = %q", edgeVoices["male"])
	}
}
