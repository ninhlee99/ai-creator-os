package web

import (
	"strings"
	"testing"
)

// Đợt X: path Windows phải dựng đúng và escape an toàn cho filter ffmpeg.
func TestWindowsFontCandidates(t *testing.T) {
	got := windowsFontCandidates(`D:\Win`)
	if len(got) != 2 {
		t.Fatalf("muốn 2 ứng viên, được %d", len(got))
	}
	if !strings.HasSuffix(got[0], "arialbd.ttf") || !strings.HasPrefix(got[0], `D:\Win`) {
		t.Fatalf("ứng viên đầu sai: %q", got[0])
	}
	// WINDIR trống → fallback C:\Windows.
	got = windowsFontCandidates("")
	if !strings.HasPrefix(got[0], `C:\Windows`) {
		t.Fatalf("thiếu fallback C:\\Windows: %q", got[0])
	}
}

// dtEscape dùng cho fontfile=: path Windows sau escape không được còn
// `\` đơn hay `:` trần (cả hai đều phá cú pháp filter ffmpeg).
func TestFontfileEscapeWindowsPath(t *testing.T) {
	esc := dtEscape(`C:\Windows\Fonts\arialbd.ttf`)
	if strings.Contains(esc, ":") && !strings.Contains(esc, "\\:") {
		t.Fatalf("còn dấu : chưa escape: %q", esc)
	}
	want := `C\:\\Windows\\Fonts\\arialbd.ttf`
	if esc != want {
		t.Fatalf("escape sai: được %q, muốn %q", esc, want)
	}
}
