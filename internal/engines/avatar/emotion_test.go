package avatar

import (
	"strings"
	"testing"
)

func TestEmotionCueFromTag(t *testing.T) {
	cue, ok := EmotionCueFromTag("[cười]")
	if !ok || cue.Name != "smile" {
		t.Errorf("cười → %+v, ok=%v", cue, ok)
	}
	if _, ok := EmotionCueFromTag("[không tồn tại]"); ok {
		t.Error("unknown tag must not map")
	}
	// case-insensitive, brackets optional
	if _, ok := EmotionCueFromTag("THỞ DÀI"); !ok {
		t.Error("case-insensitive lookup failed")
	}
}

func TestParseEmotionTags(t *testing.T) {
	cues := ParseEmotionTags("Xin chào [cười] mọi người [thở dài] nhé")
	if len(cues) != 2 {
		t.Fatalf("cues = %d, want 2", len(cues))
	}
	if cues[0].Name != "smile" || cues[1].Name != "sigh" {
		t.Errorf("wrong cues: %+v", cues)
	}
	if cues[0].AtMs != -1 {
		t.Error("AtMs must be -1 before Timed")
	}
	if cues[0].CharOffset >= cues[1].CharOffset {
		t.Error("offsets must increase")
	}
	// unknown tags are skipped, not fatal
	if cues := ParseEmotionTags("hello [xyz]"); len(cues) != 0 {
		t.Errorf("unknown tags must be skipped, got %v", cues)
	}
}

func TestStripEmotionTags(t *testing.T) {
	got := StripEmotionTags("Xin chào [cười] mọi người")
	if strings.Contains(got, "[") || !strings.Contains(got, "Xin chào") {
		t.Errorf("bad strip: %q", got)
	}
}

func TestTimed(t *testing.T) {
	cues := ParseEmotionTags("ab [cười] cd")
	timed := Timed(cues, 6, 6000)
	if len(timed) != 1 {
		t.Fatalf("timed = %d", len(timed))
	}
	// tag sits after "ab " → offset 3 of 6 chars → ~3000ms
	if timed[0].AtMs != 3000 {
		t.Errorf("AtMs = %d, want 3000", timed[0].AtMs)
	}
}
