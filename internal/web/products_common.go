package web

// Affiliate product catalog: product search and discovery, manual add,
// the merged shelf tab, autopilot scheduling policy and music upload.

import (
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// parseBounded parses an optional form number: empty → 0 (ok); otherwise
// the value must be a valid number within [0, max]. Used to validate
// product input server-side (Đợt Y) — client-side min/max is not enough.
func parseBounded(s string, max float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > max {
		return 0, false
	}
	return v, true
}

// productErr redirects back to the product page with a visible error
// (Đợt Y: ?err= is now rendered by handleProducts — no more silent fails).
func productErr(w http.ResponseWriter, r *http.Request, tab, msg string) {
	u := "/products?err=" + url.QueryEscape(msg)
	if tab != "" {
		u = "/products?tab=" + tab + "&err=" + url.QueryEscape(msg)
	}
	seeOther(w, r, u)
}
