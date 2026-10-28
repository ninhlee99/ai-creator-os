package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"html/template"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"net/url"
	"strings"
	"time"
)

var templateFuncs = template.FuncMap{
	"assetVer": func() string { return assetVersion },
	"sub": func(a, b int) int { return a - b },
	// fmtTime renders slot minutes-since-midnight as HH:MM.
	"fmtTime": func(min int) string {
		return fmt.Sprintf("%02d:%02d", min/60, min%60)
	},
	// fmtVN renders an ISO timestamp ("2006-01-02T15:04:05") as Vietnamese
	// "02/01/2006 15:04". Đợt AH: người Việt đọc ngày/tháng/năm tự nhiên hơn.
	"fmtVN": func(s string) string {
		for _, layout := range []string{"2006-01-02T15:04:05", time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				if layout == "2006-01-02" {
					return t.Format("02/01/2006")
				}
				return t.Format("02/01/2006 15:04")
			}
		}
		return s
	},
	// fmtVND renders a VND amount with dot thousands separators:
	// 1234500 → "1.234.500 ₫".
	"fmtVND": func(v float64) string { return formatVND(v) },
	"productMoney": func(v float64, currency string) string {
		if v <= 0 {
			return "Chưa có dữ liệu"
		}
		switch strings.ToUpper(strings.TrimSpace(currency)) {
		case "VND":
			return formatVND(v)
		case "":
			return fmt.Sprintf("%.2f · chưa rõ tiền tệ", v)
		default:
			return fmt.Sprintf("%.2f %s", v, strings.ToUpper(strings.TrimSpace(currency)))
		}
	},
	// fmtUSD renders a USD amount with comma separators: "$1,234.50".
	"fmtUSD": func(v float64) string { return formatUSD(v) },
	// statusLabel renders a raw status slug as its Vietnamese label.
	"statusLabel":            statusLabel,
	"publisherPlatformLabel": publisherPlatformLabel,
	"growthVariantLabel":     growthVariantLabel,
	// reasonLabel dịch lý do dạng "slug_a -> slug_b" sang tiếng Việt.
	"reasonLabel": reasonLabel,
	// statusClass maps a raw status slug to a semantic badge class suffix.
	"statusClass": statusClass,
	"mul":         func(a, b float64) float64 { return a * b },
	"add":         func(a, b int) int { return a + b },
	"pct":         func(f float64) float64 { return f * 100 },
	"join": func(sep string, xs []string) string {
		return strings.Join(xs, sep)
	},
	"hasPrefix":             strings.HasPrefix,
	"isProtectedRadarTopic": studio.IsTikTokRadarProtectedDefaultTopic,
	"urlQuery":              url.QueryEscape,
	// themeLabel renders a theme slug as its catalog label.
	"themeLabel": func(slug string) string {
		if t, ok := products.DescribeTheme(slug); ok {
			return t.Label
		}
		if slug == "" {
			return "—"
		}
		return slug
	},
	// agentLabel renders a raw agent slug as its Vietnamese label + icon
	// for the decision feed (R2-11: no raw slugs in the UI).
	"agentLabel": agentLabel,
	// variantAspect and variantSeconds render the planned TikTok target.
	"variantAspect":  growth.VariantAspect,
	"variantSeconds": growth.VariantSeconds,
	// dict builds a string-keyed map for passing named arguments to
	// partial templates ({{template "x" (dict "A" .B)}}).
	"dict": func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			if k, ok := kv[i].(string); ok {
				m[k] = kv[i+1]
			}
		}
		return m
	},
}
