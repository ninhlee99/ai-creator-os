package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	KeyViralAutoProduce  = "viral.auto_produce_enabled"
	keyViralRadarLastRun = "viral.radar.last_run"
	KeyViralRadarSources = "viral.radar.sources"
	KeyViralRadarQuery   = "viral.radar.query"
)

const viralRadarInterval = 24 * time.Hour

// ViralRadarTick discovers and stores recent source candidates. Production
// waits for a user to pair a source with the correct channel model photo.
func (s *Service) ViralRadarTick(ctx context.Context, force bool) []string {
	if s.Settings == nil {
		return nil
	}
	enabled := true
	if value, ok := s.Settings.Get(KeyViralAutoProduce); ok {
		enabled = value == "1" || value == "true"
	}
	if !force && !enabled {
		return nil
	}
	if !force && !dueSince(s.Settings, keyViralRadarLastRun, viralRadarInterval) {
		return nil
	}
	if s.killed() {
		if force {
			return []string{"Kill switch is on; viral discovery paused."}
		}
		return nil
	}
	if s.ViralRadar == nil {
		if force {
			return []string{"YOUTUBE_API_KEY missing; add it at /settings/system to search source videos."}
		}
		return nil
	}

	query := "video viral Việt Nam"
	if value, ok := s.Settings.Get(KeyViralRadarQuery); ok && strings.TrimSpace(value) != "" {
		query = strings.TrimSpace(value)
	} else if s.Accounts != nil {
		if accounts, err := s.Accounts.List(); err == nil {
			for _, account := range accounts {
				if account != nil && strings.TrimSpace(account.Niche) != "" && account.Status != "paused" && account.Status != "retired" {
					query = strings.TrimSpace(account.Niche)
					break
				}
			}
		}
	}

	sources, err := s.ViralRadar.Discover(ctx, query)
	stampRun(s.Settings, keyViralRadarLastRun)
	if err != nil {
		return []string{"YouTube viral radar failed: " + err.Error()}
	}
	encoded, err := json.Marshal(sources)
	if err != nil {
		return []string{"YouTube viral radar could not store results: " + err.Error()}
	}
	if err := s.Settings.Set(KeyViralRadarSources, string(encoded)); err != nil {
		return []string{"YouTube viral radar could not store results: " + err.Error()}
	}
	if len(sources) == 0 {
		return []string{fmt.Sprintf("No recent source from channels with 100k+ subscribers for query %q.", query)}
	}
	if !enabled {
		return []string{fmt.Sprintf("Found %d high-subscriber source videos; automatic production is off.", len(sources))}
	}
	if s.dryRun() {
		return []string{fmt.Sprintf("Found %d source candidates; dry-run stored results without creating a video job.", len(sources))}
	}
	return []string{fmt.Sprintf("Found and stored %d source candidates. Choose source video and matching channel model photo at /viral before production.", len(sources))}
}

// RefreshViralRadarSetting stores a user-selected source query.
func (s *Service) RefreshViralRadarQuery(query string) error {
	if s.Settings == nil {
		return errNoLedger
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 100 {
		return fmt.Errorf("search query must be 100 characters or fewer")
	}
	if query == "" {
		return s.Settings.Delete(KeyViralRadarQuery)
	}
	return s.Settings.Set(KeyViralRadarQuery, query)
}
