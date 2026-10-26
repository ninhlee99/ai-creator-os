package growth

import (
	"strings"
	"testing"
)

func TestVariantMetadata(t *testing.T) {
	if VariantAspect(VariantTikTok) != "9:16" {
		t.Error("TikTok variant must render 9:16")
	}
	if VariantSeconds(VariantTikTok) != 60 {
		t.Error("TikTok seconds should be 60")
	}
	if VariantKind(VariantTikTok) != "short_video" {
		t.Error("TikTok kind should be short_video")
	}

	it := PlanItem{FormatID: "truyen_ma", Topic: "truyện ma", SeriesEp: 1, Variant: VariantTikTok}
	title := VariantTitle(it)
	if title != "truyện ma" {
		t.Errorf("VariantTitle = %q, want 'truyện ma'", title)
	}

	caption := VariantCaption(it)
	if !strings.Contains(caption, "#ai") {
		t.Errorf("caption should carry #ai tag: %q", caption)
	}
	if !strings.Contains(caption, "tập 1") {
		t.Errorf("caption should carry series ep: %q", caption)
	}
}
