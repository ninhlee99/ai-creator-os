package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

const (
	KeyViralAutoProduce  = "viral.auto_produce_enabled"
	keyViralRadarLastRun = "viral.radar.last_run"
	KeyViralRadarSources = "viral.radar.sources"
	KeyViralRadarQuery   = "viral.radar.query"
)

const viralRadarInterval = 24 * time.Hour

// ViralRadarTick discovers recent videos from channels above 100k subscribers
// and queues at most one local, unpublished original video per daily run.
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
		return []string{fmt.Sprintf("Found %d high-subscriber source videos; dry-run blocks video production.", len(sources))}
	}
	if s.ViralRunner == nil {
		return []string{"Source videos saved; Studio is not ready to create a video."}
	}
	photo := s.ViralRunner.DefaultModelPhoto()
	if strings.TrimSpace(photo) == "" {
		return []string{"Source videos saved; upload a model photo before automatic video production."}
	}
	source := sources[0]
	duration := source.DurationSeconds
	if duration < 60 {
		duration = 60
	}
	jobID, err := s.ViralRunner.EnqueueViral(ctx, studio.ViralParams{
		Topic:          source.Title,
		Genre:          "trendjack",
		VisualMood:     "cinematic",
		ModelPhoto:     photo,
		ModelPhotos:    []string{photo},
		TargetDuration: duration,
		SourceVideoURL: source.URL,
		AIMode:         "local",
	})
	if err != nil {
		return []string{"Source videos saved; original video job failed to start: " + err.Error()}
	}
	if s.Ledger != nil {
		_ = s.Ledger.Decide("automation", "viral_source_video", nil,
			fmt.Sprintf("Queued original local video job=%s from source=%s channel=%s subscribers=%d duration=%ds; no source footage/audio reused.", jobID, source.URL, source.ChannelTitle, source.Subscribers, source.DurationSeconds), nil)
	}
	return []string{fmt.Sprintf("Queued local original video job %s from %q (%d subscribers, source duration %ds).", jobID, source.ChannelTitle, source.Subscribers, source.DurationSeconds)}
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
