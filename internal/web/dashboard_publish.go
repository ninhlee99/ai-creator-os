package web

import (
	"sort"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

type dashboardPublishOutcome struct {
	JobID        string
	Title        string
	Platform     string
	Source       string
	Status       string
	OutcomeLabel string
	RemoteID     string
	Note         string
	UpdatedAt    string
	TimeKnown    bool
	eventTime    time.Time
}

func (s *Server) dashboardPublishOutcomes(limit int) ([]dashboardPublishOutcome, error) {
	var legs []studio.PublishLeg
	if s.Studio != nil {
		var err error
		legs, err = s.Studio.RecentPublishLegs(limit)
		if err != nil {
			return nil, err
		}
	}
	return s.recentPublishOutcomes(legs, limit), nil
}

// recentPublishOutcomes shows TikTok Creator Inbox draft delivery only.
func (s *Server) recentPublishOutcomes(legs []studio.PublishLeg, limit int) []dashboardPublishOutcome {
	out := make([]dashboardPublishOutcome, 0, len(legs))
	for _, leg := range legs {
		if !strings.EqualFold(leg.Platform, "tiktok") {
			continue
		}
		at, shown := studioPublishTime(leg.UpdatedAt)
		label := "Lỗi"
		note := "Mở job Studio để xem trạng thái gửi TikTok."
		switch leg.Status {
		case studio.LegPublishing:
			label, note = "Đang gửi", "Chờ TikTok Creator Inbox xác nhận."
		case studio.LegNeedsReview:
			label, note = "Cần đối chiếu", "Kiểm tra Creator Inbox trước khi thử lại."
		case studio.LegDone:
			label, note = "Draft đã tải lên", "Mở TikTok để gắn link sản phẩm và sound rồi duyệt đăng."
		}
		out = append(out, dashboardPublishOutcome{
			JobID: leg.JobID, Title: leg.Title, Platform: "tiktok", Source: "Affiliate Autopilot",
			Status: leg.Status, OutcomeLabel: label, RemoteID: leg.RemoteID, Note: note,
			UpdatedAt: shown, TimeKnown: !at.IsZero(), eventTime: at,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].eventTime.Equal(out[j].eventTime) {
			return out[i].eventTime.After(out[j].eventTime)
		}
		return out[i].JobID > out[j].JobID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func studioPublishTime(raw string) (time.Time, string) {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local)
	if err != nil {
		return time.Time{}, raw
	}
	return t, t.UTC().Format(time.RFC3339)
}
