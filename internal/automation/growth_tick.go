package automation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// Growth setting keys in the ledger settings table (kept identical to
// the web era so existing databases keep working).
const (
	growthProductionKey = "growth.production_enabled"
	growthLastSyncKey   = "growth.last_sync"
)

// productionEnabled is the service-side toggle check.
func (s *Service) productionEnabled() bool {
	if s.Settings == nil {
		return true
	}
	v, ok := s.Settings.Get(growthProductionKey)
	return !ok || v != "0"
}

// ProductionEnabled is the exported toggle check for the dashboard UI.
func (s *Service) ProductionEnabled() bool { return s.productionEnabled() }

// SetProductionEnabled persists the operator's choice ("1"/"0").
func (s *Service) SetProductionEnabled(on bool) error {
	if s.Settings == nil {
		return errNoLedger
	}
	if on {
		return s.Settings.Set(growthProductionKey, "1")
	}
	return s.Settings.Set(growthProductionKey, "0")
}

// ------------------------------------------------------------ the tick

// GrowthTick runs one zero-touch production pass: due plan items ->
// Studio renders -> YouTube uploads (quota-guarded), plus an hourly
// metrics sync + decision loop through the existing engine. It runs by
// default (a stored "0" opts out); the cmd daemon calls it on a timer.
// Returned notes are for logs/UI — the pass also writes alerts and
// decision rows itself. force=true is the manual "chạy ngay" button (an
// explicit human command — still bounded by kill switch and dry-run).
func (s *Service) GrowthTick(ctx context.Context, force bool) []string {
	if s.Growth == nil {
		return nil
	}
	if s.killed() {
		return []string{"Kill switch đang bật — vòng growth đứng yên."}
	}
	if !force && !s.productionEnabled() {
		return nil
	}
	var notes []string

	if s.growthSyncDue() {
		// Affiliate money reconcile rides the hourly sync — real API
		// payload only, fail-closed (no source -> honest decision,
		// numbers stay 0).
		notes = append(notes, s.ReconcileCommissions(ctx)...)
		if accounts, err := s.Accounts.List(); err == nil {
			for _, a := range accounts {
				notes = append(notes, s.SyncOneAccount(ctx, a)...)
			}
			if s.Settings != nil {
				_ = s.Settings.Set(growthLastSyncKey, time.Now().Format(time.RFC3339))
			}
		}
	}

	accounts, err := s.Accounts.List()
	if err != nil {
		return append(notes, "Không đọc được danh sách tài khoản: "+err.Error())
	}
	for _, a := range accounts {
		switch a.Status {
		case "paused", "penalized", "retired":
			continue // never produce for stopped accounts (doc §4.2)
		}
		notes = append(notes, s.produceAccount(ctx, a)...)
		notes = append(notes, s.advanceAccount(ctx, a)...)
	}
	return notes
}

func (s *Service) growthSyncDue() bool {
	if s.Settings == nil {
		return true
	}
	raw, ok := s.Settings.Get(growthLastSyncKey)
	if !ok || raw == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, raw)
	return err != nil || time.Since(t) >= time.Hour
}

// effectiveDate applies the per-variant schedule stagger (doc §5.2) to a
// plan date.
func effectiveDate(it growth.PlanItem) string {
	t, err := time.Parse("2006-01-02", it.PlannedFor)
	if err != nil {
		return it.PlannedFor
	}
	return t.AddDate(0, 0, growth.VariantLagDays(it.Variant)).Format("2006-01-02")
}

func variantLabel(v string) string {
	switch v {
	case growth.VariantShorts:
		return "YouTube Shorts"
	case growth.VariantYouTube:
		return "YouTube dài"
	default:
		return "TikTok"
	}
}

func primaryPlatform(a *network.Account) string {
	if len(a.YoutubeContentTypes) > 0 {
		return "youtube"
	}
	return "tiktok"
}

func hasYouTube(a *network.Account) bool {
	return a.YoutubeChannel != "" || len(a.YoutubeContentTypes) > 0
}

// -------------------------------------------------------- produce pass

func (s *Service) produceAccount(ctx context.Context, a *network.Account) []string {
	var notes []string
	if _, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a)); err != nil {
		return notes
	}
	today := s.today()
	loc := s.location()
	staleCutoff := time.Now().In(loc).AddDate(0, 0, -s.StaleItemDays).Format("2006-01-02")
	since := time.Now().In(loc).AddDate(0, 0, -s.DedupWindowDays).Format("2006-01-02")
	recents, _ := s.Growth.RecentNetworkItems(since, 500)

	due, err := s.Growth.PlannedItemsDue(a.ID, today, 60)
	if err != nil {
		return append(notes, a.Username+": lỗi đọc kế hoạch — "+err.Error())
	}
	made := 0
	for _, it := range due {
		if effectiveDate(it) > today {
			continue // variant stagger: not its day yet
		}
		if it.PlannedFor < staleCutoff {
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemDropped,
				fmt.Sprintf("Quá hạn %d ngày chưa sản xuất — hệ thống loại để không đăng dồn nội dung cũ", s.StaleItemDays))
			_ = s.Growth.Decide("growth_director", "drop_stale_item", a.Username,
				"mục quá hạn: "+it.Topic, map[string]any{"item": it.ID, "planned_for": it.PlannedFor})
			notes = append(notes, a.Username+": loại mục quá hạn \""+it.Topic+"\"")
			continue
		}
		if made >= s.MaxProductionsPerTick {
			continue
		}
		it = s.dedupCheck(a, it, recents)

		concept := growth.ConceptOf(it.FormatID, it.Topic)
		_ = s.Growth.SetItemConcept(it.ID, concept, growth.ConceptHash(concept))

		if s.dryRun() {
			notes = append(notes, a.Username+": DRY-RUN — chưa sản xuất thật cho \""+it.Topic+"\"")
			continue
		}
		p := s.producer()
		if p == nil {
			n, _ := s.Growth.NoteItemFailure(it.ID, "Chưa sản xuất được: backend sản xuất chưa được nối")
			if n == 1 {
				_ = s.Growth.InsertAlert(growth.Alert{
					AccountID: a.ID, Severity: "warn", Kind: "plan",
					Message:     "Chưa sản xuất được mục \"" + it.Topic + "\": backend sản xuất chưa được nối",
					ActionTaken: "Giữ mục ở trạng thái chờ và tự thử lại ở vòng sau",
				})
			}
			notes = append(notes, a.Username+": chưa sản xuất được \""+it.Topic+"\" — backend sản xuất chưa được nối")
			continue
		}
		jobID, err := p.Enqueue(ctx, ProduceRequest{Account: a, Item: it})
		if err != nil {
			n, _ := s.Growth.NoteItemFailure(it.ID, "Chưa sản xuất được: "+err.Error())
			if n == 1 {
				_ = s.Growth.InsertAlert(growth.Alert{
					AccountID: a.ID, Severity: "warn", Kind: "plan",
					Message:     "Chưa sản xuất được mục \"" + it.Topic + "\": " + err.Error(),
					ActionTaken: "Giữ mục ở trạng thái chờ và tự thử lại ở vòng sau",
				})
			}
			notes = append(notes, a.Username+": chưa sản xuất được \""+it.Topic+"\" — "+err.Error())
			continue
		}
		title := growth.VariantTitle(it)
		if err := s.Growth.SetItemProduction(it.ID, jobID, title, growth.VariantCaption(it)); err != nil {
			notes = append(notes, a.Username+": lỗi ghi job cho \""+it.Topic+"\" — "+err.Error())
			continue
		}
		// The freshly-stamped concept joins this pass's comparison set so
		// a second account processed later in the same tick sees it.
		recents = append(recents, growth.PlanItem{
			ID: it.ID, AccountID: a.ID, FormatID: it.FormatID,
			VariantGroup: it.VariantGroup, ConceptText: concept,
		})
		made++
		notes = append(notes, a.Username+": đã xếp sản xuất \""+it.Topic+"\" ("+variantLabel(it.Variant)+", job "+jobID+")")
	}
	return notes
}

// dedupCheck runs the anti-duplicate guard for one due item. On a
// collision with another account's produced concept it forces a
// re-generation with a different angle — production continues, the script
// cannot be a near-copy (doc §3.1).
func (s *Service) dedupCheck(a *network.Account, it growth.PlanItem, recents []growth.PlanItem) growth.PlanItem {
	concept := growth.ConceptOf(it.FormatID, it.Topic)
	cand := growth.TokensOf(concept)
	var sets []map[string]bool
	for _, r := range recents {
		if r.ID == it.ID || growth.SameLine(it, r) {
			continue
		}
		sets = append(sets, growth.TokensOf(r.ConceptText))
	}
	hit, sim := growth.DecideDedup(cand, sets)
	if !hit {
		return it
	}
	newTopic := growth.ApplyAngle(it.Topic, growth.AngleFor(it.ID, it.Attempts))
	newConcept := growth.ConceptOf(it.FormatID, newTopic)
	if err := s.Growth.SetItemDedup(it.ID, newTopic, newConcept, growth.ConceptHash(newConcept)); err != nil {
		return it
	}
	it.Topic = newTopic
	_ = s.Growth.Decide("growth_director", "dedup_regenerate_angle", a.Username,
		fmt.Sprintf("concept trùng %.0f%% với nội dung đã sản xuất trên mạng — đổi góc triển khai", sim*100),
		map[string]any{"item": it.ID, "similarity": sim, "new_topic": newTopic})
	_ = s.Growth.InsertAlert(growth.Alert{
		AccountID: a.ID, Severity: "info", Kind: "dedup",
		Message:     fmt.Sprintf("Mục %q trùng ý tưởng (%.0f%%) với nội dung đã sản xuất trên mạng", concept, sim*100),
		ActionTaken: "Hệ thống tự đổi góc viết: \"" + newTopic + "\"",
	})
	return it
}

// -------------------------------------------------------- advance pass

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
		[]string{growth.ItemProduced, growth.ItemWaitingConnect, growth.ItemWaitingQuota}, 50)
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
	today := s.today()
	used, _ := s.Growth.QuotaUsed(today)
	if !growth.QuotaCanUpload(used) {
		if it.Status != growth.ItemWaitingQuota {
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemWaitingQuota,
				fmt.Sprintf("Chờ quota YouTube: hôm nay đã dùng %d/%d units (1 lượt tải = %d)", used, growth.DailyQuotaUnits, growth.UploadCostUnits))
			_ = s.Growth.InsertAlert(growth.Alert{
				AccountID: a.ID, Severity: "warn", Kind: "quota",
				Message:     fmt.Sprintf("Quota YouTube hôm nay đã chạm trần (%d/%d units)", used, growth.DailyQuotaUnits),
				ActionTaken: "Dời lượt đăng sang ngày mai; cần nhiều hơn thì tách project theo kênh (bootstrap)",
			})
			_ = s.Growth.Decide("growth_publisher", "defer_quota", a.Username,
				"quota ngày đã cạn", map[string]any{"used": used, "item": it.ID})
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

	desc := it.PubCaption
	if sib := s.siblingRef(a, it); sib != "" {
		desc += "\n\n▶ Xem thêm: https://youtu.be/" + sib
	}
	desc += "\n\n" + growth.DisclosureLine
	res := up.Upload(ctx, a, output, it.PubTitle, desc, growth.VariantKind(it.Variant))
	if !res.Ok {
		if strings.Contains(res.Error, "not enabled for this channel") {
			// Permanent until the channel's content types change — park
			// it as a connection problem instead of retrying forever.
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemWaitingConnect,
				"Chờ kết nối YouTube: kênh chưa bật loại nội dung này — thêm loại nội dung cho kênh ở trang chi tiết tài khoản")
			return nil
		}
		_ = s.Growth.SetItemStatus(it.ID, growth.ItemProduced,
			"Đăng YouTube thất bại: "+res.Error+" — hệ thống tự thử lại ở vòng sau")
		return []string{a.Username + ": đăng YouTube thất bại cho \"" + it.Topic + "\" — " + res.Error}
	}
	_ = s.Growth.AddQuota(today, growth.UploadCostUnits)
	_ = s.Growth.SetItemPublished(it.ID, res.RemoteID)
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
