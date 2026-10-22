package studio

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FindTrendingMusic returns a genuine TikTok trending music track for the given category.
// Category can be "dance", "viral", "fashion", "affiliate", or "".
func FindTrendingMusic(category string) string {
	searchDirs := []string{
		filepath.Join("data", "music", "trending"),
		"/Users/ninhlee/ninhlee/apps/ai-creator-os/data/music/trending",
	}
	cat := strings.ToLower(category)
	for _, dir := range searchDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.mp3"))
		if err == nil && len(matches) > 0 {
			sort.Strings(matches)
			for _, m := range matches {
				base := strings.ToLower(filepath.Base(m))
				// Nhạc chill / acoustic cho kể chuyện, kiến thức, bí ẩn, đời sống
				if (cat == "mystery" || cat == "story" || cat == "drama" || cat == "knowledge" || cat == "lifehack" || cat == "chill") && (strings.Contains(base, "chill") || strings.Contains(base, "acoustic")) {
					if abs, err := filepath.Abs(m); err == nil {
						return abs
					}
					return m
				}
				// Nhạc dance / remix sôi động cho nhảy, trendjack, challenge
				if (cat == "dance" || cat == "nhay" || cat == "sexy" || cat == "remix" || cat == "upbeat" || cat == "challenge") && strings.Contains(base, "dance") {
					if abs, err := filepath.Abs(m); err == nil {
						return abs
					}
					return m
				}
				// Nhạc thời trang, affiliate, review sản phẩm
				if (cat == "fashion" || cat == "affiliate" || cat == "review") && (strings.Contains(base, "fashion") || strings.Contains(base, "chill") || strings.Contains(base, "acoustic")) {
					if abs, err := filepath.Abs(m); err == nil {
						return abs
					}
					return m
				}
			}
			// Mặc định ưu tiên nhạc êm dịu nếu là video kể chuyện/thoại thay vì dance giật
			if cat == "mystery" || cat == "story" || cat == "drama" || cat == "knowledge" {
				for _, m := range matches {
					if strings.Contains(strings.ToLower(m), "chill") || strings.Contains(strings.ToLower(m), "acoustic") {
						if abs, err := filepath.Abs(m); err == nil {
							return abs
						}
						return m
					}
				}
			}
			if abs, err := filepath.Abs(matches[0]); err == nil {
				return abs
			}
			return matches[0]
		}
	}
	return findDefaultMusic()
}

// FindDefaultMusic returns a valid local commercial music track path if available.
func FindDefaultMusic() string {
	trendingDirs := []string{
		filepath.Join("data", "music", "trending"),
		filepath.Join("..", "..", "data", "music", "trending"),
		filepath.Join("..", "data", "music", "trending"),
	}
	for _, dir := range trendingDirs {
		if matches, err := filepath.Glob(filepath.Join(dir, "*.mp3")); err == nil && len(matches) > 0 {
			sort.Strings(matches)
			if abs, err := filepath.Abs(matches[0]); err == nil {
				return abs
			}
			return matches[0]
		}
	}

	candidates := []string{
		filepath.Join("data", "autopilot-music.m4a"),
		filepath.Join("data", "trending-audio.m4a"),
		"internal/web/static/preview.mp3",
		"docs/assets/affiliate-spot/music-just-chill.mp3",
		"../../internal/web/static/preview.mp3",
		"../../docs/assets/affiliate-spot/music-just-chill.mp3",
		"../web/static/preview.mp3",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Size() > 1000 {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	return ""
}

func findDefaultMusic() string {
	return FindDefaultMusic()
}
