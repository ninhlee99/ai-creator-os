package web

import (
	"sort"
	"strings"

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
	ticks []TickBeatView,
	quota QuotaCircuitView,
) []dashboardAttentionItem {
	items := make([]dashboardAttentionItem, 0,
		len(growthItems)+len(failedJobs)+len(failedLegs)+len(ticks)+1)
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
		if !strings.EqualFold(leg.Platform, "tiktok") {
			continue
		}
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
		case item.Status == growth.ItemProduced:
			title, rank = "Video Growth đã dựng, chờ gắn sản phẩm và đăng TikTok", 3
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
