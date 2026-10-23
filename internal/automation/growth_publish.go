package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"strings"
)

func (s *Service) advanceAccount(ctx context.Context, a *network.Account) []string {
	var notes []string
	producing, _ := s.Growth.ItemsByStatus(a.ID, []string{growth.ItemProducing}, 50)
	for _, it := range producing {
		status, _ := s.producer().JobState(it.StudioJobID)
		switch status {
		case studio.StatusDone:
			if it.Variant == growth.VariantTikTok {
				_ = s.Growth.SetItemProduced(it.ID,
					"Đã render xong. Trước audit Direct Post, TikTok chỉ đăng nháp (qua luồng tự đăng nếu đang bật) — hệ thống không tự nhận là đã đăng công khai.")
				notes = append(notes, a.Username+": render xong \""+it.Topic+"\" (TikTok — chờ đăng nháp)")
			} else {
				_ = s.Growth.SetItemProduced(it.ID, "Đã render xong, chờ đăng YouTube")
			}
		case studio.StatusFailed:
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemFailed,
				"Studio job thất bại — xem chi tiết ở trang Studio AI")
			notes = append(notes, a.Username+": sản xuất thất bại \""+it.Topic+"\"")
		case "":
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemFailed,
				"Không tìm thấy Studio job tương ứng (job đã bị dọn?)")
		}
	}

	pending, _ := s.Growth.ItemsByStatus(a.ID,
		[]string{growth.ItemProduced, growth.ItemWaitingConnect, growth.ItemWaitingQuota, growth.ItemNeedsReview}, 50)
	for _, it := range pending {
		if it.Variant == growth.VariantTikTok {
			continue // TikTok stays draft-only until the Direct Post audit
		}
		notes = append(notes, s.publishItem(ctx, a, it)...)
	}
	return notes
}

// publishItem attempts the YouTube upload of one rendered item,
// fail-closed at every gate: credentials, quota, dry-run. States stay
// honest ("chờ kết nối"/"chờ quota") until a real upload returns a video
// id.
func (s *Service) publishItem(ctx context.Context, a *network.Account, it growth.PlanItem) []string {
	if it.Status == growth.ItemNeedsReview {
		if s.JobStore != nil && it.StudioJobID != "" && s.JobStore.PublishLegStatus(it.StudioJobID, "youtube") == studio.LegDone {
			rid := s.JobStore.PublishLegRemoteID(it.StudioJobID, "youtube")
			_ = s.Growth.SetItemPublished(it.ID, rid)
		}
		return nil
	}
	up := s.uploader()
	if up == nil {
		return nil // no upload backend wired; leave the item waiting
	}
	state := up.State(a)
	if !state.Ready() {
		note := "Chờ kết nối YouTube: " + strings.Join(state.Missing(), "; ")
		if it.Status != growth.ItemWaitingConnect || it.PubNote != note {
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemWaitingConnect, note)
		}
		return nil
	}
	p := s.producer()
	status, output := "", ""
	if p != nil {
		status, output = p.JobState(it.StudioJobID)
	}
	if status != studio.StatusDone || output == "" {
		return nil // render not actually finished; leave the state alone
	}
	if s.dryRun() {
		if it.PubNote != "DRY-RUN: đã render xong — chưa tải lên YouTube thật" {
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemProduced,
				"DRY-RUN: đã render xong — chưa tải lên YouTube thật")
		}
		return nil
	}

	// Đợt AA: chân YouTube của job đã done (do autopilot OnDone đăng trước)
	// thì không đăng lại — đánh dấu item đã đăng từ leg.
	if s.JobStore != nil && it.StudioJobID != "" {
		legStatus := s.JobStore.PublishLegStatus(it.StudioJobID, "youtube")
		if legStatus == studio.LegNeedsReview {
			note := "Kết quả upload Autopilot chưa xác định; kiểm tra YouTube Studio trước khi thử lại."
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemNeedsReview, note)
			return []string{a.Username + ": " + note}
		}
		if legStatus == studio.LegFailed {
			return []string{a.Username + ": chờ Autopilot thử lại chân YouTube đã lỗi; Growth không tải lần nữa để tránh trùng."}
		}
		if legStatus == studio.LegDone {
			rid := s.JobStore.PublishLegRemoteID(it.StudioJobID, "youtube")
			_ = s.Growth.SetItemPublished(it.ID, rid)
			_ = s.Growth.Decide("growth_publisher", "skip_duplicate_youtube", a.Username,
				"chân YouTube đã đăng xong bởi autopilot — bỏ qua để tránh trùng",
				map[string]any{"item": it.ID, "video": rid})
			return []string{a.Username + ": \"" + it.Topic + "\" đã được đăng YouTube trước đó — bỏ qua (tránh trùng)"}
		}
	}
	if s.Growth == nil {
		return []string{a.Username + ": chưa đăng YouTube vì không đọc được bộ đếm quota; cần kiểm tra máy chủ."}
	}
	quotaDay := s.quotaDay()
	used, reserved, err := s.Growth.ReserveUploadQuota(quotaDay)
	if err != nil {
		_ = s.Growth.SetItemStatus(it.ID, growth.ItemWaitingQuota,
			"Chờ quota YouTube: không ghi được bộ đếm quota; chưa gửi yêu cầu tải lên.")
		return []string{a.Username + ": chưa đăng YouTube vì lỗi bộ đếm quota: " + err.Error()}
	}
	if !reserved {
		note := fmt.Sprintf("Chờ quota YouTube: đã dùng %d/%d lượt trong ngày reset Pacific.", used, growth.DailyQuotaUnits)
		if it.Status != growth.ItemWaitingQuota || it.PubNote != note {
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemWaitingQuota, note)
			_ = s.Growth.InsertAlert(growth.Alert{
				AccountID: a.ID, Severity: "warn", Kind: "quota",
				Message:     note,
				ActionTaken: "Chờ reset theo giờ Pacific; dừng đăng tới ngày quota kế tiếp.",
			})
			_ = s.Growth.Decide("growth_publisher", "defer_quota", a.Username,
				"quota ngày đã cạn", map[string]any{"used": used, "item": it.ID, "quota_day": quotaDay})
		}
		return nil
	}

	desc := it.PubCaption
	if sib := s.siblingRef(a, it); sib != "" {
		desc += "\n\n▶ Xem thêm: https://youtu.be/" + sib
	}
	desc += "\n\n" + growth.DisclosureLine
	res := up.Upload(ctx, a, output, it.PubTitle, desc, growth.VariantKind(it.Variant))
	if !res.Ok {
		if !res.UploadRequestAttempted {
			_ = s.Growth.ReleaseUploadQuota(quotaDay)
		}
		if isYouTubeQuotaExceeded(res.Error) {
			_ = s.Growth.ExhaustUploadQuota(quotaDay)
			note := "Chờ quota YouTube: Google báo quota project đã cạn; app dừng tải tới lần reset Pacific kế tiếp."
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemWaitingQuota, note)
			_ = s.Growth.InsertAlert(growth.Alert{AccountID: a.ID, Severity: "warn", Kind: "quota",
				Message: note, ActionTaken: "Dừng thử lại tới ngày quota kế tiếp."})
			return []string{a.Username + ": YouTube báo hết quota; đã dừng tới lần reset tiếp theo."}
		}
		if strings.Contains(res.Error, "not enabled for this channel") {
			// Permanent until the channel's content types change — park
			// it as a connection problem instead of retrying forever.
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemWaitingConnect,
				"Chờ kết nối YouTube: kênh chưa bật loại nội dung này — thêm loại nội dung cho kênh ở trang chi tiết tài khoản")
			return nil
		}
		if res.DeliveryUncertain {
			note := "Kết quả upload YouTube chưa xác định; dừng retry để tránh đăng trùng. Kiểm tra YouTube Studio trước khi xử lý tiếp."
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemNeedsReview, note)
			_ = s.Growth.Decide("growth_publisher", "publish_youtube_failed", a.Username,
				"upload có thể đã được YouTube nhận; cần đối chiếu trước khi retry.", map[string]any{
					"item": it.ID, "job_id": it.StudioJobID, "title": it.PubTitle,
					"error": res.Error, "delivery_uncertain": true, "quota_day": quotaDay,
				})
			return []string{a.Username + ": " + note}
		}
		_ = s.Growth.SetItemStatus(it.ID, growth.ItemProduced,
			"Đăng YouTube thất bại: "+res.Error+" — hệ thống tự thử lại ở vòng sau")
		return []string{a.Username + ": đăng YouTube thất bại cho \"" + it.Topic + "\" — " + res.Error}
	}
	_ = s.Growth.SetItemPublished(it.ID, res.RemoteID)
	// Đánh dấu story job đã đăng để digest "bản nháp chờ đăng" không đếm nhầm
	// video này mỗi ngày (đợt AC).
	if it.StudioJobID != "" && s.Settings != nil {
		_ = s.Settings.Set(KeyStoryPublishedPrefix+it.StudioJobID, "1")
	}
	_ = s.Growth.Decide("growth_publisher", "publish_youtube", a.Username,
		"đã đăng \""+it.PubTitle+"\"", map[string]any{
			"item": it.ID, "video": res.RemoteID, "draft": res.Draft,
			"quota_units": growth.UploadCostUnits,
		})
	s.linkRelated(a, it)
	note := a.Username + ": đã đăng YouTube \"" + it.PubTitle + "\""
	if res.URL != "" {
		note += " (" + res.URL + ")"
	}
	if res.Draft {
		note += " [đang để riêng tư theo YOUTUBE_DEFAULT_PRIVACY]"
	}
	return []string{note}
}

func isYouTubeQuotaExceeded(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "quotaexceeded") ||
		strings.Contains(message, "dailylimitexceeded") ||
		strings.Contains(message, "quota exceeded")
}

// siblingRef returns the published YouTube video id of the item's variant
// sibling (same group, different variant), "" when none.
func (s *Service) siblingRef(a *network.Account, it growth.PlanItem) string {
	items, err := s.Growth.ActivePlanItems(a.ID)
	if err != nil {
		return ""
	}
	for _, o := range items {
		if o.ID != it.ID && o.VariantGroup == it.VariantGroup &&
			o.Variant != it.Variant && o.PublishedRef != "" {
			return o.PublishedRef
		}
	}
	return ""
}

// linkRelated records Short <-> long-form sibling links inside the item's
// variant group (Related Video lineage, doc §5.2).
func (s *Service) linkRelated(a *network.Account, it growth.PlanItem) {
	items, err := s.Growth.ActivePlanItems(a.ID)
	if err != nil {
		return
	}
	for _, o := range items {
		if o.ID == it.ID || o.VariantGroup != it.VariantGroup || o.Variant == it.Variant {
			continue
		}
		if (it.Variant == growth.VariantShorts && o.Variant == growth.VariantYouTube) ||
			(it.Variant == growth.VariantYouTube && o.Variant == growth.VariantShorts) {
			_ = s.Growth.LinkRelatedItems(it.ID, o.ID)
		}
	}
}
