package products

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestParsePriceFromText(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"-39% ₫244.000", 244000},
		{"400.000₫", 400000},
		{"₫199.000", 199000},
		{"1.250.000 đ", 1250000},
		{"249,000 VND", 249000},
		{"", 0},
	}

	for _, tc := range cases {
		got := parsePriceFromText(tc.input)
		if got != tc.expected {
			t.Errorf("parsePriceFromText(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestLiveTikTokExtract(t *testing.T) {
	if os.Getenv("AICOS_TEST_LIVE_TIKTOK") != "1" {
		t.Skip("skipping live test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	url := "https://vt.tiktok.com/ZS9yyH8GaWmh4-dqNSg/"
	t.Log("Extracting from:", url)
	res, err := ExtractTikTokProductWithBrowser(ctx, url)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	t.Logf("Title: %s", res.Title)
	t.Logf("Price: %.0f", res.Price)
	t.Logf("Images count: %d", len(res.ImageURLs))
	for i, img := range res.ImageURLs {
		t.Logf("  [%d] %s", i+1, img)
	}
	t.Logf("Description length: %d", len(res.Description))
}
