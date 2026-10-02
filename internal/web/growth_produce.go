package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ------------------------------------------------------------ settings

// Growth automation settings (ledger settings table). Production is OFF
// by default: the toggle in /growth is the one deliberate switch, after
// which the system runs zero-touch inside the dry-run/kill-switch gates.
const (
	SettingGrowthProduction = "growth.production_enabled"
	SettingGrowthLastSync   = "growth.last_sync"
)

// maxGrowthProductionsPerTick caps how many fresh renders one account may
// start per automation tick — cadence discipline (doc §4.1), and it keeps
// a backlog after downtime from binge-posting.
const maxGrowthProductionsPerTick = 3

// staleItemDays: plan items left unproduced longer than this are dropped
// instead of rendered late — posting a two-week-old plan in bulk is worse
// than dropping it; the weekly replan writes fresh ones.
const staleItemDays = 14

// dedupWindowDays: how far back produced concepts count for the guard.
const dedupWindowDays = 45

func (s *Server) growthSetting(key string) string {
	if s.Ledger == nil {
		return ""
	}
	v, ok, err := s.Ledger.GetSetting(key)
	if err != nil || !ok {
		return ""
	}
	return v
}

func (s *Server) setGrowthSetting(key, val string) {
	if s.Ledger != nil {
		_ = s.Ledger.SetSetting(key, val)
	}
}

// GrowthProductionEnabled reports the dashboard toggle state.
func (s *Server) GrowthProductionEnabled() bool {
	return s.growthSetting(SettingGrowthProduction) == "1"
}

// ------------------------------------------------------- backend ports

// growthProduceRequest is one due plan item to render.
type growthProduceRequest struct {
	Account *network.Account
	Item    growth.PlanItem
}

// growthProducer renders plan items and reports render state. JobState
// returns ("", "") when the job is unknown.
type growthProducer interface {
	Enqueue(ctx context.Context, req growthProduceRequest) (string, error)
	JobState(jobID string) (status, output string)
}

// growthYTState is the honest YouTube-upload readiness of one account.
type growthYTState struct {
	HasClient  bool // OAuth app (client id + secret) configured in Settings
	HasToken   bool // per-account refresh token file present
	HasChannel bool // channel mapping attached (metrics + display)
	Privacy    string
}

// Ready reports whether an upload could actually be attempted.
func (st growthYTState) Ready() bool { return st.HasClient && st.HasToken }

// Label is the compact badge text for account rows.
func (st growthYTState) Label() string {
	switch {
	case st.Ready():
		return "sẵn sàng đăng"
	case !st.HasClient:
		return "thiếu OAuth app"
	default:
		return "thiếu token tài khoản"
	}
}

// Missing lists what blocks uploads, in Vietnamese, for item notes.
func (st growthYTState) Missing() []string {
	var m []string
	if !st.HasClient {
		m = append(m, "chưa nhập YOUTUBE_CLIENT_ID/YOUTUBE_CLIENT_SECRET trong Cài đặt")
	}
	if !st.HasToken {
		m = append(m, "tài khoản chưa hoàn tất OAuth YouTube (chưa có token)")
	}
	return m
}

// growthYTUploader uploads rendered videos to YouTube.
type growthYTUploader interface {
	State(a *network.Account) growthYTState
	Upload(ctx context.Context, a *network.Account, videoPath, title, description, kind string) publishers.PublishResult
}

func (s *Server) producer() growthProducer {
	if s.GrowthProducer != nil {
		return s.GrowthProducer
	}
	return studioGrowthProducer{s: s}
}

func (s *Server) yt() growthYTUploader {
	if s.GrowthYT != nil {
		return s.GrowthYT
	}
	return youtubeGrowthUploader{s: s}
}

// ------------------------------------------------- real backend adapters

type studioGrowthProducer struct{ s *Server }

// Enqueue renders one plan item. Product-theme accounts go through the
// affiliate autopilot (theme -> product -> 30s format, per-account model
// photos); persona accounts get a film job whose length follows the
// variant. Either way every variant is its OWN job and its own file.
func (p studioGrowthProducer) Enqueue(ctx context.Context, req growthProduceRequest) (string, error) {
	a := req.Account
	if a.Theme != "" {
		if p.s.Autopilot == nil {
			return "", fmt.Errorf("autopilot chưa sẵn sàng (Studio/kho sản phẩm chưa được nối)")
		}
		res, err := p.s.Autopilot.Run(ctx, a.ID)
		if err != nil {
			return "", err
		}
		if res.JobID == "" {
			return "", fmt.Errorf("autopilot không tạo được job")
		}
		return res.JobID, nil
	}
	if p.s.Studio == nil {
		return "", fmt.Errorf("Studio chưa sẵn sàng")
	}
	return p.s.Studio.CreateFilmJob(studio.FilmParams{
		Topic:   req.Item.Topic,
		Seconds: growth.VariantSeconds(req.Item.Variant),
	})
}

func (p studioGrowthProducer) JobState(jobID string) (string, string) {
	if p.s.Studio == nil || jobID == "" {
		return "", ""
	}
	j, ok := p.s.Studio.GetJob(jobID)
	if !ok {
		return "", ""
	}
	return j.Status, j.Output
}

type youtubeGrowthUploader struct{ s *Server }

func (u youtubeGrowthUploader) State(a *network.Account) growthYTState {
	_, cidOK := u.s.effectiveEnv("YOUTUBE_CLIENT_ID")
	_, csOK := u.s.effectiveEnv("YOUTUBE_CLIENT_SECRET")
	priv, _ := u.s.effectiveEnv("YOUTUBE_DEFAULT_PRIVACY")
	if priv == "" {
		priv = "private"
	}
	return growthYTState{
		HasClient:  cidOK && csOK,
		HasToken:   fileExists(publishers.YouTubeTokenPath(a.Username)),
		HasChannel: a.YoutubeChannel != "",
		Privacy:    priv,
	}
}

func (u youtubeGrowthUploader) Upload(ctx context.Context, a *network.Account, videoPath, title, description, kind string) publishers.PublishResult {
	pub := publishers.NewYouTubePublisher(a.Username, a.YoutubeContentTypes, a.YoutubeChannel)
	pub.Synthetic = true // AI disclosure is mandatory (docs §3.6), never off
	return pub.Publish(ctx, videoPath, title, description, kind)
}

// ------------------------------------------------------------ the tick

// GrowthAutomationTick runs one zero-touch production pass: due plan
// items -> Studio renders -> YouTube uploads (quota-guarded), plus an
// hourly metrics sync + decision loop through the existing engine. It is
// driven by the /growth toggle (default OFF); the cmd daemon calls it on
// a timer. Returned notes are for logs/UI — the pass also writes alerts
// and decision rows itself.
func (s *Server) GrowthAutomationTick(ctx context.Context) []string {
	return s.growthTick(ctx, false)
}

// growthTick is the shared pass; force=true is the manual "chạy ngay"
// button (an explicit human command — still bounded by kill switch and
// dry-run).
func (s *Server) growthTick(ctx context.Context, force bool) []string {
	if s.Growth == nil {
		return nil
	}
	if s.Cfg != nil && s.Cfg.KillSwitch() {
		return []string{"Kill switch đang bật — vòng growth đứng yên."}
	}
	if !force && !s.GrowthProductionEnabled() {
		return nil
	}
	var notes []string

	if s.growthSyncDue() {
		if accounts, err := s.Mgr.List(); err == nil {
			for _, a := range accounts {
				notes = append(notes, s.syncOneAccount(ctx, a)...)
			}
			s.setGrowthSetting(SettingGrowthLastSync, time.Now().Format(time.RFC3339))
		}
	}

	accounts, err := s.Mgr.List()
	if err != nil {
		return append(notes, "Không đọc được danh sách tài khoản: "+err.Error())
	}
	for _, a := range accounts {
		switch a.Status {
		case "paused", "penalized", "retired":
			continue // never produce for stopped accounts (doc §4.2)
		}
		notes = append(notes, s.growthProduceAccount(ctx, a)...)
		notes = append(notes, s.growthAdvanceAccount(ctx, a)...)
	}
	return notes
}

func (s *Server) growthSyncDue() bool {
	raw := s.growthSetting(SettingGrowthLastSync)
	if raw == "" {
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

// -------------------------------------------------------- produce pass

func (s *Server) growthProduceAccount(ctx context.Context, a *network.Account) []string {
	var notes []string
	if _, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a)); err != nil {
		return notes
	}
	today := s.today()
	staleCutoff := time.Now().In(s.location()).AddDate(0, 0, -staleItemDays).Format("2006-01-02")
	since := time.Now().In(s.location()).AddDate(0, 0, -dedupWindowDays).Format("2006-01-02")
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
				"Quá hạn "+fmt.Sprint(staleItemDays)+" ngày chưa sản xuất — hệ thống loại để không đăng dồn nội dung cũ")
			_ = s.Growth.Decide("growth_director", "drop_stale_item", a.Username,
				"mục quá hạn: "+it.Topic, map[string]any{"item": it.ID, "planned_for": it.PlannedFor})
			notes = append(notes, a.Username+": loại mục quá hạn \""+it.Topic+"\"")
			continue
		}
		if made >= maxGrowthProductionsPerTick {
			continue
		}
		it = s.growthDedupCheck(a, it, recents)

		concept := growth.ConceptOf(it.FormatID, it.Topic)
		_ = s.Growth.SetItemConcept(it.ID, concept, growth.ConceptHash(concept))

		if s.Cfg != nil && s.Cfg.DryRun() {
			notes = append(notes, a.Username+": DRY-RUN — chưa sản xuất thật cho \""+it.Topic+"\"")
			continue
		}
		jobID, err := s.producer().Enqueue(ctx, growthProduceRequest{Account: a, Item: it})
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

// growthDedupCheck runs the anti-duplicate guard for one due item. On a
// collision with another account's produced concept it forces a
// re-generation with a different angle — production continues, the script
// cannot be a near-copy (doc §3.1).
func (s *Server) growthDedupCheck(a *network.Account, it growth.PlanItem, recents []growth.PlanItem) growth.PlanItem {
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

func (s *Server) growthAdvanceAccount(ctx context.Context, a *network.Account) []string {
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
		notes = append(notes, s.growthPublishItem(ctx, a, it)...)
	}
	return notes
}

// growthPublishItem attempts the YouTube upload of one rendered item,
// fail-closed at every gate: credentials, quota, dry-run. States stay
// honest ("chờ kết nối"/"chờ quota") until a real upload returns a video
// id.
func (s *Server) growthPublishItem(ctx context.Context, a *network.Account, it growth.PlanItem) []string {
	up := s.yt()
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
	status, output := s.producer().JobState(it.StudioJobID)
	if status != studio.StatusDone || output == "" {
		return nil // render not actually finished; leave the state alone
	}
	if s.Cfg != nil && s.Cfg.DryRun() {
		if it.PubNote != "DRY-RUN: đã render xong — chưa tải lên YouTube thật" {
			_ = s.Growth.SetItemStatus(it.ID, growth.ItemProduced,
				"DRY-RUN: đã render xong — chưa tải lên YouTube thật")
		}
		return nil
	}

	desc := it.PubCaption
	if sib := s.growthSiblingRef(a, it); sib != "" {
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
	s.growthLinkRelated(a, it)
	note := a.Username + ": đã đăng YouTube \"" + it.PubTitle + "\""
	if res.URL != "" {
		note += " (" + res.URL + ")"
	}
	if res.Draft {
		note += " [đang để riêng tư theo YOUTUBE_DEFAULT_PRIVACY]"
	}
	return []string{note}
}

// growthSiblingRef returns the published YouTube video id of the item's
// variant sibling (same group, different variant), "" when none.
func (s *Server) growthSiblingRef(a *network.Account, it growth.PlanItem) string {
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

// growthLinkRelated records Short <-> long-form sibling links inside the
// item's variant group (Related Video lineage, doc §5.2).
func (s *Server) growthLinkRelated(a *network.Account, it growth.PlanItem) {
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

// ------------------------------------------------------------- handlers

func (s *Server) renderGrowth(w http.ResponseWriter, notes []string) {
	data := s.ctx()
	for k, v := range s.growthView() {
		data[k] = v
	}
	if len(notes) > 0 {
		data["SyncNotes"] = notes
	}
	s.render(w, "growth", data)
}

// handleGrowthProductionToggle flips the zero-touch production switch.
func (s *Server) handleGrowthProductionToggle(w http.ResponseWriter, r *http.Request) {
	if s.Growth == nil {
		s.renderGrowth(w, []string{"Module Phát triển kênh chưa sẵn sàng."})
		return
	}
	on := r.FormValue("enabled") == "1"
	val := "0"
	msg := "Đã tắt sản xuất & đăng tự động theo kế hoạch."
	if on {
		val = "1"
		msg = "Đã bật sản xuất & đăng tự động: hệ thống tự sản xuất theo plan, tự đăng YouTube trong quota (TikTok vẫn nháp trước audit). DRY-RUN/kill switch vẫn chặn như thường."
	}
	s.setGrowthSetting(SettingGrowthProduction, val)
	_ = s.Ledger.Decide("growth", "production_toggle", nil, msg, map[string]any{"enabled": on})
	s.renderGrowth(w, []string{msg})
}

// handleGrowthProductionRun runs one automation pass immediately (the
// same pass the timer runs; the toggle is not required for an explicit
// run, kill switch and dry-run still apply).
func (s *Server) handleGrowthProductionRun(w http.ResponseWriter, r *http.Request) {
	notes := s.growthTick(r.Context(), true)
	if len(notes) == 0 {
		notes = []string{"Vòng chạy xong: không có mục nào đến hạn hoặc đang chờ."}
	}
	s.renderGrowth(w, notes)
}
