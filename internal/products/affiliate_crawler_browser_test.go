package products

import "testing"

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
