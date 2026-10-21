package studio

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const viralPaidVideoDisabledMessage = "free-only: Gemini Omni video edit cần billing nên job viral đang tạm dừng"

// ViralCharacterVideoEditor is retained as a capability seam for a future
// free video editor. Production stays disabled while the only adapter is paid.
type ViralCharacterVideoEditor interface {
	EditCharacterVideoSegment(ctx context.Context, sourceVideo string, characterPhotos []string, outputPath string) error
}

func (g *GeminiMediaGen) EditCharacterVideoSegment(context.Context, string, []string, string) error {
	return fmt.Errorf("%s", viralPaidVideoDisabledMessage)
}

func validCharacterPhotos(paths []string, max int) []string {
	photos := make([]string, 0, min(len(paths), max))
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		clean := filepath.Clean(path)
		if path == "" || !fileExists(path) || seen[clean] {
			continue
		}
		seen[clean] = true
		photos = append(photos, path)
		if len(photos) == max {
			break
		}
	}
	return photos
}
