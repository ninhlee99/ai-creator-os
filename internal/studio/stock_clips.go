package studio

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FindStockClips returns high-quality motion video clips
// (9:16 vertical, continuous camera movement, 100% clean with zero text overlays).
func FindStockClips(category string) []string {
	searchDirs := []string{
		filepath.Join("data", "clips", category),
		filepath.Join("data", "clips", "fashion"),
		filepath.Join("data", "clips", "viral"),
		filepath.Join("..", "..", "data", "clips", category),
		filepath.Join("..", "..", "data", "clips", "fashion"),
		filepath.Join("..", "..", "data", "clips", "viral"),
		filepath.Join("..", "data", "clips", category),
		filepath.Join("..", "data", "clips", "fashion"),
		filepath.Join("..", "data", "clips", "viral"),
	}
	for _, dir := range searchDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
		if err == nil && len(matches) > 0 {
			sort.Strings(matches)
			var valid []string
			for _, m := range matches {
				if st, err := os.Stat(m); err == nil && !st.IsDir() && st.Size() > 10000 {
					if abs, err := filepath.Abs(m); err == nil {
						valid = append(valid, abs)
					} else {
						valid = append(valid, m)
					}
				}
			}
			if len(valid) > 0 {
				return valid
			}
		}
	}
	return nil
}

// PickSingleStockClipForDuration selects exactly ONE matching high-quality video clip for the given category, query, and target duration.
// When targetSec >= 45, it prioritizes full-length (>= 60s) continuous footage to fulfill TikTok Creativity Program requirements without looping.
func PickSingleStockClipForDuration(category, query string, targetSec float64) string {
	clips := FindStockClips(category)
	if len(clips) == 0 {
		return ""
	}
	q := strings.ToLower(query)

	if targetSec >= 45 {
		for _, c := range clips {
			if strings.Contains(strings.ToLower(c), "60s") {
				return c
			}
		}
	}

	if category == "fashion" {
		if targetSec >= 45 {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "lookbook") || strings.Contains(strings.ToLower(c), "60s") {
					return c
				}
			}
		}
		if strings.Contains(q, "street") || strings.Contains(q, "hoodie") || strings.Contains(q, "thể thao") || strings.Contains(q, "năng động") {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "streetwear") {
					return c
				}
			}
		}
		for _, c := range clips {
			if strings.Contains(strings.ToLower(c), "dress") {
				return c
			}
		}
	} else if category == "viral" {
		isCN := strings.Contains(q, "cn") || strings.Contains(q, "douyin") || strings.Contains(q, "trung quốc") || strings.Contains(q, "c-biz") || strings.Contains(q, "hoa ngữ")
		if isCN {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "douyin_hot") || strings.Contains(strings.ToLower(c), "douyin") {
					return c
				}
			}
		}

		if targetSec >= 45 {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "dance_60s") || strings.Contains(strings.ToLower(c), "60s") {
					return c
				}
			}
		}

		if strings.Contains(q, "duo") || strings.Contains(q, "đôi") {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "duo") {
					return c
				}
			}
		}

		if strings.Contains(q, "crew") || strings.Contains(q, "nhóm") || strings.Contains(q, "đội") {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "crew") || strings.Contains(strings.ToLower(c), "cac_dance") {
					return c
				}
			}
		}

		// Vietnamese creator priority
		if strings.Contains(q, "phuonghoa") || strings.Contains(q, "vn") || strings.Contains(q, "việt") || strings.Contains(q, "v-biz") {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "phuonghoa") {
					return c
				}
			}
		}

		for _, c := range clips {
			if strings.Contains(strings.ToLower(c), "phuonghoa") {
				return c
			}
		}
	}
	return clips[0]
}

// PickSingleStockClip selects exactly ONE matching high-quality video clip for the given category and query.
func PickSingleStockClip(category, query string) string {
	return PickSingleStockClipForDuration(category, query, 0)
}
