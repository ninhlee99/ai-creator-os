package web

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

type dashboardPublishOutcome struct {
	JobID        string
	Title        string
	Platform     string
	Source       string
	Target       string
	Status       string
	OutcomeLabel string
	RemoteID     string
	Note         string
	UpdatedAt    string
	TimeKnown    bool
	eventTime    time.Time
	eventOrder   int64
}

type publishDecisionInput struct {
	JobID             string `json:"job_id"`
	ItemID            int64  `json:"item"`
	Title             string `json:"title"`
	Video             string `json:"video"`
	RemoteID          string `json:"remote_id"`
	Error             string `json:"error"`
	Draft             bool   `json:"draft"`
	Privacy           string `json:"privacy"`
	DeliveryUncertain bool   `json:"delivery_uncertain"`
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
	return s.recentPublishOutcomes(legs, limit)
}

// recentPublishOutcomes merges Studio's latest autopilot leg state with
// timestamped direct Growth/Story publish decisions. It reports API outcomes,
// not public visibility or platform-side confirmation after upload.
func (s *Server) recentPublishOutcomes(legs []studio.PublishLeg, limit int) ([]dashboardPublishOutcome, error) {
	if limit <= 0 {
		return nil, nil
	}
	out := make([]dashboardPublishOutcome, 0, len(legs)+limit)
	for _, leg := range legs {
		at, shown := studioPublishTime(leg.UpdatedAt)
		out = append(out, dashboardPublishOutcome{
			JobID: leg.JobID, Title: leg.Title, Platform: leg.Platform,
			Source: "Affiliate Autopilot", Status: leg.Status,
			OutcomeLabel: publishOutcomeLabel(leg.Platform, leg.Status, "", false, leg.RemoteID),
			RemoteID:     leg.RemoteID,
			Note:         publishLegNote(leg.Platform, leg.Status, leg.RemoteID), UpdatedAt: shown,
			TimeKnown: !at.IsZero(), eventTime: at,
		})
	}

	rows, err := s.db.Query(`SELECT id, agent, action, COALESCE(target,''),
		COALESCE(reason,''), COALESCE(inputs_json,''), created_at
		FROM decisions
		WHERE agent IN ('growth_publisher','story_publisher')
		  AND action IN ('publish_youtube','publish_youtube_failed')
		ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var agent, action, target, reason, rawInputs, createdAt string
		if err := rows.Scan(&id, &agent, &action, &target, &reason, &rawInputs, &createdAt); err != nil {
			return nil, err
		}
		var input publishDecisionInput
		_ = json.Unmarshal([]byte(rawInputs), &input)
		status := "done"
		if action == "publish_youtube_failed" {
			status = "failed"
		}
		if input.DeliveryUncertain {
			status = studio.LegNeedsReview
		}
		resolvedLabel := ""
		if status == studio.LegNeedsReview && input.ItemID > 0 && s.Growth != nil {
			if item, itemErr := s.Growth.GetItem(input.ItemID); itemErr == nil && item != nil {
				switch item.Status {
				case "published":
					status = studio.LegDone
					resolvedLabel = "Đã xác nhận có video"
					if item.PublishedRef != "" {
						input.Video = item.PublishedRef
					}
				case "produced":
					status = "produced"
					resolvedLabel = "Đã xác nhận chưa có · chờ thử lại"
				}
			}
		}
		if status == studio.LegNeedsReview && agent == "story_publisher" && input.JobID != "" && s.Ledger != nil {
			if remoteID, published, stateErr := s.Ledger.GetSetting(automation.KeyStoryPublishedPrefix + input.JobID); stateErr == nil && published {
				status = studio.LegDone
				resolvedLabel = "Đã xác nhận có video"
				if remoteID != "1" { // Legacy Story marker means published, not a platform ID.
					input.Video = remoteID
				}
			} else {
				_, needsReview, reviewErr := s.Ledger.GetSetting(automation.KeyStoryNeedsReviewPrefix + input.JobID)
				_, publishing, markerErr := s.Ledger.GetSetting(automation.KeyStoryPublishingPrefix + input.JobID)
				if reviewErr == nil && markerErr == nil && !needsReview && !publishing {
					status = "produced"
					resolvedLabel = "Đã xác nhận chưa có · đã cho phép thử lại"
				}
			}
		}
		source := "Growth"
		if agent == "story_publisher" {
			source = "Kể chuyện"
		}
		title := input.Title
		if title == "" && status == "done" {
			// Existing Growth decisions predate title/job_id in inputs_json;
			// their reason still carries the successful title.
			title = reason
		}
		note := input.Error
		if resolvedLabel != "" {
			note = "Đã đối chiếu thủ công theo trạng thái hiện tại của mục Growth."
			if agent == "story_publisher" {
				note = "Đã đối chiếu thủ công theo trạng thái hiện tại của job Story."
			}
		}
		if note == "" && status == studio.LegNeedsReview {
			note = "Upload có thể đã được YouTube nhận; kiểm tra trong YouTube Studio trước khi thử lại."
		}
		if note == "" && status == studio.LegDone {
			switch strings.ToLower(strings.TrimSpace(input.Privacy)) {
			case "private":
				note = "Video được tải lên ở chế độ riêng tư."
			case "unlisted":
				note = "Video được tải lên ở chế độ không công khai."
			default:
				if input.Draft {
					note = "API báo video chưa ở chế độ công khai."
				}
			}
		}
		if note == "" && status == "failed" {
			note = reason
		}
		at, shown := decisionPublishTime(createdAt)
		remoteID := input.Video
		if remoteID == "" {
			remoteID = input.RemoteID
		}
		outcomeLabel := publishOutcomeLabel("youtube", status, input.Privacy, input.Draft, remoteID)
		if resolvedLabel != "" {
			outcomeLabel = resolvedLabel
		}
		out = append(out, dashboardPublishOutcome{
			JobID: input.JobID, Title: title, Platform: "youtube", Source: source,
			Target: target, Status: status,
			OutcomeLabel: outcomeLabel,
			RemoteID:     remoteID, Note: note,
			UpdatedAt: shown, TimeKnown: !at.IsZero(), eventTime: at, eventOrder: id,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].eventTime.Equal(out[j].eventTime) {
			return out[i].eventTime.After(out[j].eventTime)
		}
		if out[i].eventOrder != out[j].eventOrder {
			return out[i].eventOrder > out[j].eventOrder
		}
		return out[i].JobID > out[j].JobID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func studioPublishTime(raw string) (time.Time, string) {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local)
	if err != nil {
		return time.Time{}, raw
	}
	return t, t.UTC().Format(time.RFC3339)
}

func decisionPublishTime(raw string) (time.Time, string) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		var t time.Time
		var err error
		if layout == time.RFC3339 {
			t, err = time.Parse(layout, raw)
		} else {
			t, err = time.ParseInLocation(layout, raw, time.UTC)
		}
		if err == nil {
			return t, t.UTC().Format(time.RFC3339)
		}
	}
	return time.Time{}, raw
}

func publishOutcomeLabel(platform, status, privacy string, draft bool, remoteID string) string {
	if status == studio.LegNeedsReview {
		return "Cần đối chiếu"
	}
	if status != studio.LegDone {
		return "Lỗi"
	}
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "tiktok":
		return "API đã nhận draft"
	case "threads":
		if strings.TrimSpace(remoteID) == "" {
			return "API thành công, thiếu post ID"
		}
		return "Đã đăng bài chữ"
	case "youtube":
		switch strings.ToLower(strings.TrimSpace(privacy)) {
		case "private":
			return "Đã tải riêng tư"
		case "unlisted":
			return "Đã tải không công khai"
		}
		if draft {
			return "Đã tải, chưa công khai"
		}
	}
	return "API thành công"
}

func publishLegNote(platform, status, remoteID string) string {
	if status == studio.LegNeedsReview {
		return "Upload có thể đã tới nền tảng; kiểm tra trước khi thử lại."
	}
	if status == studio.LegFailed {
		return "Mở job Studio để xem lỗi đầy đủ."
	}
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "tiktok":
		return "Bản nháp chờ trong TikTok; mở app để kiểm tra, gắn giỏ hàng/sound rồi tự đăng."
	case "threads":
		if strings.TrimSpace(remoteID) == "" {
			return "Threads API báo thành công nhưng thiếu post ID; kiểm tra bài trên Threads trước khi thử lại."
		}
		return "Adapter hiện đăng bài chữ kèm link; video không được tải lên Threads."
	}
	return ""
}
