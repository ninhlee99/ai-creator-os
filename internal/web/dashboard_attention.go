package web

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

type dashboardAttentionItem struct {
	Class  string
	Source string
	Title  string
	Detail string
	Href   string
	rank   int
}

// dashboardAttention combines persisted current states that still need
// operator attention. Historical alerts and publish events stay in their
// separate feeds because they have no resolution state.
func (s *Server) dashboardAttention(
	growthItems []growthItemRow,
	failedJobs []studioJobView,
	failedLegs []studio.PublishLeg,
	storyFailures []dashboardAttentionItem,
	ticks []TickBeatView,
	quota QuotaCircuitView,
) []dashboardAttentionItem {
	items := make([]dashboardAttentionItem, 0,
		len(growthItems)+len(failedJobs)+len(failedLegs)+len(storyFailures)+len(ticks)+1)
	failedJobIDs := make(map[string]struct{}, len(failedJobs))
	for _, job := range failedJobs {
		failedJobIDs[job.ID] = struct{}{}
		title := strings.TrimSpace(job.Title)
		if title == "" {
			title = job.ID
		}
		items = append(items, dashboardAttentionItem{
			Class: "err", Source: "Studio", Title: "Job tạo nội dung lỗi",
			Detail: joinAttentionDetail(title, job.FailureReason),
			Href:   "/studio/jobs?id=" + job.ID + "#job-" + job.ID,
			rank:   1,
		})
	}
	for _, leg := range failedLegs {
		title := strings.TrimSpace(leg.Title)
		if title == "" {
			title = leg.JobID
		}
		class, state, detail := "err", "lỗi", "Mở job để xem log và trạng thái thử lại."
		if leg.Status == studio.LegNeedsReview {
			class, state, detail = "warn", "cần đối chiếu", "Kiểm tra nền tảng trước khi thử lại; app đã dừng retry tự động."
		}
		items = append(items, dashboardAttentionItem{
			Class: class, Source: "Autopilot",
			Title:  "Chân đăng " + publisherPlatformLabel(leg.Platform) + " " + state,
			Detail: joinAttentionDetail(title, detail),
			Href:   "/studio/jobs?id=" + leg.JobID + "#job-" + leg.JobID,
			rank:   1,
		})
	}
	items = append(items, storyFailures...)
	for _, item := range growthItems {
		if item.Status == growth.ItemFailed && item.StudioJobID != "" {
			if _, exists := failedJobIDs[item.StudioJobID]; exists {
				continue
			}
		}
		class, title, rank := "warn", "", 3
		switch {
		case item.DashboardStatus == "Quá lịch":
			title, rank = "Mục Growth quá lịch", 3
		case item.Status == growth.ItemFailed:
			class, title, rank = "err", "Mục Growth lỗi", 1
		case item.Status == growth.ItemWaitingConnect:
			title, rank = "Growth chờ kết nối publisher", 2
		case item.Status == growth.ItemWaitingQuota:
			title, rank = "Growth chờ quota YouTube", 2
		case item.Status == growth.ItemNeedsReview:
			title, rank = "Kết quả đăng Growth cần đối chiếu", 1
		case item.Status == growth.ItemProduced:
			title, rank = "Video Growth đã dựng, chờ đăng", 3
		default:
			continue
		}
		subject := strings.TrimSpace(item.Username + " · " + growthVariantLabel(item.Variant))
		detail := joinAttentionDetail(subject, item.Topic, item.PubNote)
		items = append(items, dashboardAttentionItem{
			Class: class, Source: "Growth", Title: title, Detail: detail,
			Href: "/growth", rank: rank,
		})
	}
	for _, tick := range ticks {
		if !tick.Stale && tick.LastErr == "" {
			continue
		}
		title, detail := "Tick lỗi", tick.LastErr
		rank := 2
		if tick.Stale {
			title = "Tick quá hạn"
			detail = joinAttentionDetail("Chưa chạy đúng hạn.", tick.LastErr)
			rank = 1
		}
		items = append(items, dashboardAttentionItem{
			Class: "err", Source: "Worker", Title: title,
			Detail: joinAttentionDetail(tick.Label, detail),
			Href:   "/settings/system", rank: rank,
		})
	}
	if quota.Known && quota.Open {
		items = append(items, dashboardAttentionItem{
			Class: "warn", Source: "Quota", Title: "Quota Gemini đang ngắt",
			Detail: joinAttentionDetail("Mở tới", quota.Until),
			Href:   "/settings/system", rank: 2,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].rank != items[j].rank {
			return items[i].rank < items[j].rank
		}
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		return items[i].Title < items[j].Title
	})
	return items
}

// unresolvedStoryPublishFailures reduces Story publish decision history to
// each job's latest attempt, then uses its success marker as resolution state.
func (s *Server) unresolvedStoryPublishFailures(limit int) ([]dashboardAttentionItem, error) {
	if limit <= 0 || s.Studio == nil || s.Ledger == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT action, COALESCE(reason,''), COALESCE(inputs_json,'')
		FROM decisions
		WHERE agent='story_publisher' AND action IN ('publish_youtube','publish_youtube_failed')
		ORDER BY id DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	type attempt struct {
		action string
		reason string
		input  publishDecisionInput
	}
	seen := make(map[string]struct{})
	var latestFailures []attempt
	for rows.Next() {
		var action, reason, rawInputs string
		if err := rows.Scan(&action, &reason, &rawInputs); err != nil {
			rows.Close()
			return nil, err
		}
		var input publishDecisionInput
		if err := json.Unmarshal([]byte(rawInputs), &input); err != nil || input.JobID == "" {
			continue
		}
		if _, ok := seen[input.JobID]; ok {
			continue
		}
		seen[input.JobID] = struct{}{}
		if action == "publish_youtube_failed" {
			latestFailures = append(latestFailures, attempt{action: action, reason: reason, input: input})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	out := make([]dashboardAttentionItem, 0, limit)
	for _, failure := range latestFailures {
		job, ok := s.Studio.GetJob(failure.input.JobID)
		if !ok || job.Kind != studio.KindStory || job.Status != studio.StatusDone || job.Output == "" {
			continue
		}
		_, published, err := s.Ledger.GetSetting(automation.KeyStoryPublishedPrefix + job.ID)
		if err != nil {
			return nil, err
		}
		if published {
			continue
		}
		publishingMarker, uploading, err := s.Ledger.GetSetting(automation.KeyStoryPublishingPrefix + job.ID)
		if err != nil {
			return nil, err
		}
		if uploading && freshStoryPublishingMarker(publishingMarker) {
			continue
		}
		detail := failure.input.Error
		if detail == "" {
			detail = failure.reason
		}
		class, title := "err", "Lần đăng YouTube gần nhất lỗi"
		if failure.input.DeliveryUncertain {
			class, title = "warn", "Kết quả đăng YouTube cần đối chiếu"
		}
		out = append(out, dashboardAttentionItem{
			Class: class, Source: "Kể chuyện", Title: title,
			Detail: joinAttentionDetail(job.Title, detail),
			Href:   "/studio/jobs?id=" + job.ID + "#job-" + job.ID,
			rank:   1,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func freshStoryPublishingMarker(raw string) bool {
	_, timestamp, ok := strings.Cut(raw, "|")
	if !ok || timestamp == "" {
		return false
	}
	at, err := time.Parse(time.RFC3339, timestamp)
	return err == nil && time.Since(at) < 30*time.Minute
}

func joinAttentionDetail(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return truncateAttention(strings.Join(clean, " · "))
}

func truncateAttention(value string) string {
	runes := []rune(value)
	if len(runes) > 360 {
		return string(runes[:357]) + "..."
	}
	return value
}
