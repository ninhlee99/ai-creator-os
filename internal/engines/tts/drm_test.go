package tts

import (
	"testing"
)

func TestGenerateSecMsGecToken(t *testing.T) {
	// Reference value computed by Python rany2/edge-tts drm.generate_sec_ms_gec_v2(1700000000.0)
	const expected = "2CD23A2CA0B2595A-F3C846C370E05F9CC83ACC5F995C5F5D87A4D0F14CE8936A0F902E64099E38A2"
	got := GenerateSecMsGecToken(1700000000.0)
	if got != expected {
		t.Fatalf("mismatch:\n  got:      %s\n  expected: %s", got, expected)
	}
}
