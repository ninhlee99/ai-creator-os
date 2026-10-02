package growth

import (
	"strings"
	"testing"
)

func TestVariantSchedulesAndKinds(t *testing.T) {
	if VariantLagDays(VariantTikTok) != 0 || VariantLagDays(VariantShorts) != 1 || VariantLagDays(VariantYouTube) != 3 {
		t.Error("variant stagger must be 0/1/3 days")
	}
	if VariantKind(VariantYouTube) != "short_film" || VariantKind(VariantShorts) != "short_video" || VariantKind(VariantTikTok) != "short_video" {
		t.Error("variant kind mapping wrong")
	}
	if VariantSeconds(VariantYouTube) <= VariantSeconds(VariantShorts) {
		t.Error("long-form must be longer than the Short")
	}
}

func TestVariantMetadataDifferentiates(t *testing.T) {
	base := PlanItem{FormatID: "truyen_ma", Topic: "truyện ma — tập 2", SeriesEp: 2}
	tk, sh, lg := base, base, base
	tk.Variant, sh.Variant, lg.Variant = VariantTikTok, VariantShorts, VariantYouTube

	titles := map[string]bool{
		VariantTitle(tk): true, VariantTitle(sh): true, VariantTitle(lg): true,
	}
	if len(titles) != 3 {
		t.Errorf("variant titles must all differ: %v", titles)
	}
	if !strings.HasSuffix(VariantTitle(sh), "#Shorts") {
		t.Errorf("shorts title should carry #Shorts: %q", VariantTitle(sh))
	}
	captions := map[string]bool{
		VariantCaption(tk): true, VariantCaption(sh): true, VariantCaption(lg): true,
	}
	if len(captions) != 3 {
		t.Errorf("variant captions must all differ: %v", captions)
	}
	if !strings.Contains(VariantCaption(tk), "#ai") {
		t.Errorf("caption should carry the AI tag: %q", VariantCaption(tk))
	}
}

func TestVariantAspect(t *testing.T) {
	// Ninh 2026-10-02 (final): every film is 16:9 (cinematic standard);
	// vertical trailers are center-cropped, never re-shot.
	if VariantAspect(VariantYouTube) != "16:9" {
		t.Error("youtube_long variant must render 16:9")
	}
	if VariantAspect(VariantTikTok) != "9:16" || VariantAspect(VariantShorts) != "9:16" {
		t.Error("tiktok/shorts variants must stay 9:16")
	}
	if VariantAspect("unknown") != "9:16" {
		t.Error("unknown variant must fall back to 9:16")
	}
}
