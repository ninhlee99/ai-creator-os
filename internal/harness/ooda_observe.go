package harness

import (
	"context"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"sort"
	"time"
)

func (e *OODAEngine) Observe(ctx context.Context, accounts []*network.Account, activeJobs, doneJobs int) OODAPerception {
	now := time.Now().Format("2006-01-02 15:04:05")
	p := OODAPerception{
		Timestamp:       now,
		TotalChannels:   len(accounts),
		ActiveJobsCount: activeJobs,
		DoneJobsCount:   doneJobs,
		SystemHealth:    "unknown",
	}

	if activeJobs > 5 {
		p.SystemHealth = "busy"
	}

	for _, a := range accounts {
		if a == nil {
			continue
		}
		status := "unknown"
		if a.Status == "paused" || a.Status == "stalled" || a.Status == "penalized" {
			status = "stalled"
		}

		niche := a.Niche

		channel := ChannelState{
			AccountID:         a.ID,
			Username:          a.Username,
			Platform:          "unknown",
			Followers:         a.Followers,
			Stage:             a.Status,
			BrandNiche:        niche,
			MetricsStatus:     "unavailable",
			FormatStatsStatus: "unavailable",
			HealthStatus:      status,
		}
		if e.growth != nil {
			metrics, err := e.growth.RecentVideoMetrics(ctx, a.ID, time.Now().Add(-7*24*time.Hour))
			if err != nil {
				channel.MetricsStatus = "error"
			} else if metrics.VideoCount > 0 {
				channel.MetricsStatus = "available"
				channel.RecentMetricVideos = metrics.VideoCount
				if metrics.AverageViews != nil {
					channel.AvgViews = *metrics.AverageViews
				}
				if !metrics.LatestAt.IsZero() {
					channel.MetricsUpdatedAt = metrics.LatestAt.Local().Format("2006-01-02 15:04:05")
				}
			} else {
				channel.MetricsStatus = "missing"
			}
			formatStats, err := e.growth.ListFormatStats(a.ID)
			if err != nil {
				channel.FormatStatsStatus = "error"
			} else if len(formatStats) == 0 {
				channel.FormatStatsStatus = "missing"
			} else {
				channel.FormatStatsStatus = "available"
				sort.SliceStable(formatStats, func(i, j int) bool {
					pi, pj := formatEvidencePriority(formatStats[i].Verdict), formatEvidencePriority(formatStats[j].Verdict)
					if pi != pj {
						return pi < pj
					}
					if formatStats[i].Videos != formatStats[j].Videos {
						return formatStats[i].Videos > formatStats[j].Videos
					}
					return formatStats[i].FormatID < formatStats[j].FormatID
				})
				if len(formatStats) > maxFormatEvidencePerChannel {
					channel.FormatStatsOmitted = len(formatStats) - maxFormatEvidencePerChannel
					formatStats = formatStats[:maxFormatEvidencePerChannel]
				}
				for _, stat := range formatStats {
					channel.FormatStats = append(channel.FormatStats, ChannelFormatEvidence{
						FormatID:         stat.FormatID,
						Videos:           stat.Videos,
						Verdict:          stat.Verdict,
						VerdictReason:    stat.VerdictReason,
						MedianViews:      stat.MedianViews,
						AvgViews:         stat.AvgViews,
						MedianCompletion: stat.MedianCompletion,
						MedianProxyRate:  stat.MedianProxyRate,
						FollowsPer1k:     stat.FollowsPer1k,
						UpdatedAt:        stat.UpdatedAt,
					})
				}
			}
		}
		p.Channels = append(p.Channels, channel)
	}

	return p
}

func formatEvidencePriority(verdict string) int {
	switch verdict {
	case growth.VerdictWinner:
		return 0
	case growth.VerdictKilled:
		return 1
	case growth.VerdictTesting:
		return 2
	default:
		return 3
	}
}

// OrientAndDecide writes an evidence-bound diagnosis. Autonomy is retained in
// the signature for API compatibility; production actions stay empty until
// source-grounded viral candidates and verified affiliate inputs are wired.
