package web

import (
	"fmt"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"strconv"
	"strings"
)

// formatVND renders v as a Vietnamese dong amount: 1234500 → "1.234.500 ₫".
func formatVND(v float64) string {
	n := int64(v + 0.5)
	if v < 0 {
		n = int64(v - 0.5)
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String() + " ₫"
	}
	return b.String() + " ₫"
}

// formatUSD renders v as a US dollar amount: 1234.5 → "$1,234.50".
// Sub-cent values (API micro-costs) keep 4 decimals instead of "$0.00".
func formatUSD(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	if v > 0 && v < 0.01 {
		if neg {
			return fmt.Sprintf("-$%.4f", v)
		}
		return fmt.Sprintf("$%.4f", v)
	}
	whole := int64(v)
	frac := int64((v-float64(whole))*100 + 0.5)
	if frac == 100 {
		whole++
		frac = 0
	}
	digits := strconv.FormatInt(whole, 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return fmt.Sprintf("-$%s.%02d", b.String(), frac)
	}
	return fmt.Sprintf("$%s.%02d", b.String(), frac)
}
