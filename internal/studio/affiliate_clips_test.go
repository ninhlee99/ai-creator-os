package studio

import "testing"

func TestAffiliateClipsParams(t *testing.T) {
	p1 := AffiliateParams{}
	if p1.HasClips() {
		t.Errorf("expected HasClips false for empty params")
	}

	p2 := AffiliateParams{
		Clips: []string{"clip1.mp4", "clip2.mp4"},
	}
	if !p2.HasClips() {
		t.Errorf("expected HasClips true when clips provided")
	}

	p3 := AffiliateParams{
		Clips: []string{"   ", ""},
	}
	if p3.HasClips() {
		t.Errorf("expected HasClips false for whitespace clips")
	}
}
