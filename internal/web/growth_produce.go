package web

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/viralradar"
)

// ------------------------------------------------------------ settings

// SettingGrowthProduction mirrors the automation facade's
// growth.production_enabled key (behavior owned by
// internal/automation). Production is ON by default when unset
// (Đợt 3 zero-touch): a stored "0" is an explicit operator choice and
// always wins. DRY-RUN stays the global safety gate — while it is on
// nothing real is produced or published.
const SettingGrowthProduction = "growth.production_enabled"

// GrowthProductionEnabled reports the dashboard toggle state: ON unless
// the operator explicitly stored "0".
func (s *Server) GrowthProductionEnabled() bool {
	return s.automation().ProductionEnabled()
}

// ------------------------------------------------------- backend ports

// studioGrowthProducer adapts the Studio to automation.Producer.
type studioGrowthProducer struct{ s *Server }

// Enqueue renders one plan item. Product-theme accounts go through the
// affiliate autopilot (theme -> product -> 30s format, per-account model
// photos). Persona accounts go through the "Kể chuyện" YouTube pipeline
// (story job) — fail-closed with an honest error when the Studio is not
// ready or the item has no topic.
func (p studioGrowthProducer) Enqueue(ctx context.Context, req automation.ProduceRequest) (string, error) {
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
	// Tài khoản persona: tạo job kể chuyện YouTube từ chủ đề của plan item.
	if p.s.Studio == nil {
		return "", fmt.Errorf("studio chưa sẵn sàng — không tạo được job kể chuyện")
	}
	topic := strings.TrimSpace(req.Item.Topic)
	if topic == "" {
		return "", fmt.Errorf("plan item thiếu chủ đề — không tạo được truyện")
	}
	mp := ""
	if p.s.Cfg != nil {
		if m := filepath.Join(filepath.Dir(p.s.Cfg.DatabasePath), "autopilot-music.m4a"); fileExists(m) {
			mp = m
		}
	}
	id, err := p.s.Studio.CreateStoryJob(studio.StoryParams{
		Topic:     topic,
		Genre:     "tâm lý",
		Words:     600,
		Scenes:    6,
		MusicOn:   mp != "",
		MusicPath: mp,
		AccountID: a.ID,
		GrowthItemID: req.Item.ID,
	})
	if err != nil {
		return "", fmt.Errorf("tạo job kể chuyện lỗi: %w", err)
	}
	return id, nil
}

func (p studioGrowthProducer) JobState(jobID string) (string, string) {
	if p.s.Studio == nil || jobID == "" {
		return "", ""
	}
	j, ok := p.s.Studio.GetJob(jobID)
	if !ok {
		return "", ""
	}
	outPath := j.Output
	if p.s.Studio != nil {
		outPath = p.s.Studio.OutputPath(j.Output)
	}
	return j.Status, outPath
}

// youtubeGrowthUploader adapts the YouTube publisher to
// automation.Uploader.
type youtubeGrowthUploader struct{ s *Server }

func (u youtubeGrowthUploader) State(a *network.Account) automation.YTState {
	_, cidOK := u.s.effectiveEnv("YOUTUBE_CLIENT_ID")
	_, csOK := u.s.effectiveEnv("YOUTUBE_CLIENT_SECRET")
	priv, _ := u.s.effectiveEnv("YOUTUBE_DEFAULT_PRIVACY")
	if priv == "" {
		priv = "private"
	}
	return automation.YTState{
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

// UploadMeta (Đợt M1): đăng kèm tags + thumbnail tùy chỉnh.
func (u youtubeGrowthUploader) UploadMeta(ctx context.Context, a *network.Account, videoPath string, meta publishers.VideoMeta, kind string) publishers.PublishResult {
	pub := publishers.NewYouTubePublisher(a.Username, a.YoutubeContentTypes, a.YoutubeChannel)
	pub.Synthetic = true
	return pub.PublishMeta(ctx, videoPath, meta, kind)
}

// HasRecentUpload (Đợt V — DuplicateChecker): kiểm tra video cùng tiêu đề
// đã có trên kênh YouTube trong 24h qua chưa (chống đăng trùng khi retry).
func (u youtubeGrowthUploader) HasRecentUpload(ctx context.Context, a *network.Account, title string, within time.Duration) (bool, error) {
	pub := publishers.NewYouTubePublisher(a.Username, a.YoutubeContentTypes, a.YoutubeChannel)
	return pub.FindRecentByTitle(ctx, title, within)
}
func (u youtubeGrowthUploader) UpdatePrivacy(ctx context.Context, a *network.Account, videoID, privacy string) error {
	pub := publishers.NewYouTubePublisher(a.Username, a.YoutubeContentTypes, a.YoutubeChannel)
	return pub.UpdatePrivacy(videoID, privacy)
}

func (s *Server) producer() automation.Producer {
	if s.GrowthProducer != nil {
		return s.GrowthProducer
	}
	return studioGrowthProducer{s: s}
}

func (s *Server) yt() automation.Uploader {
	if s.GrowthYT != nil {
		return s.GrowthYT
	}
	return youtubeGrowthUploader{s: s}
}

// ------------------------------------------------- automation service

// automation wires the hands-off execution layer from the dashboard's
// live state: every tick reads fresh gates, env and settings — nothing
// is snapshotted at startup (R2-W4, R2-05).
func (s *Server) automation() *automation.Service {
	svc := automation.NewService()
	svc.Growth = s.Growth
	svc.Ledger = s.Ledger
	svc.Accounts = s.Mgr
	svc.Settings = automation.LedgerSettings{L: s.Ledger}
	svc.Env = s
	svc.Producer = s.producer()
	svc.Uploader = s.yt()
	svc.Autopilot = s.Autopilot
	if s.Studio != nil {
		svc.ViralRunner = s.Studio
	}
	if key, ok := s.effectiveEnv("YOUTUBE_API_KEY"); ok && strings.TrimSpace(key) != "" {
		svc.ViralRadar = viralradar.NewYouTube(key)
	}
	// Đợt AA: chỉ gán JobStore khi Studio thật sự tồn tại — gán nil
	// *studio.Studio vào interface tạo interface non-nil chứa nil pointer,
	// gọi method sẽ panic.
	if s.Studio != nil {
		svc.JobStore = s.Studio
	}
	svc.CommissionSource = s.CommissionSource
	// Accesstrade automation (Đợt C): hunter + order sync + campaign check.
	svc.AT = s.AT
	svc.Products = s.Products
	if s.Studio != nil {
		svc.StudioRunner = s.Studio
	}
	if s.Cfg != nil {
		dataDir := filepath.Dir(s.Cfg.DatabasePath)
		if dataDir == "" || dataDir == "." {
			dataDir = "."
		}
		svc.ATWorkDir = filepath.Join(dataDir, "at-hunter")
		// Disk guard (Đợt L): đo % đĩa đã dùng tại thư mục dữ liệu.
		svc.DataDir = dataDir
		// Nhạc nền cho video hunter: dùng chung file nhạc autopilot
		// Ninh đã upload ("" = video câm, job log ghi rõ).
		if mp := filepath.Join(dataDir, "autopilot-music.m4a"); fileExists(mp) {
			svc.ATMusicPath = mp
		}
		// Story automation (Đợt F): kể chuyện ngôi thứ nhất. Nhạc nền dùng
		// chung file autopilot Ninh đã upload.
		if s.Studio != nil {
			svc.Story = s.Studio
		}
		if mp := filepath.Join(dataDir, "autopilot-music.m4a"); fileExists(mp) {
			svc.StoryMusicPath = mp
		}
	}
	if s.Cfg != nil {
		svc.Gate = s.Cfg
		svc.Timezone = s.Cfg.Timezone
	}
	return svc
}

// AutomationService exposes the hands-off execution layer (R2-W4) for
// the cmd daemon wiring: timers call the service, never web internals.
func (s *Server) AutomationService() *automation.Service { return s.automation() }

// settings is the single configuration facade (R2-W4, R2-05): everything
// the UI or automation reads/writes as a setting lands in the ledger
// settings table.
func (s *Server) settings() automation.LedgerSettings {
	return automation.LedgerSettings{L: s.Ledger}
}

// ------------------------------------------------------------ the tick

// GrowthAutomationTick runs one zero-touch production pass through the
// automation service (R2-W4). Returned notes are for logs/UI — the pass
// also writes alerts and decision rows itself.
func (s *Server) GrowthAutomationTick(ctx context.Context) []string {
	return s.automation().GrowthTick(ctx, false)
}

// syncOneAccount delegates to the automation service.
func (s *Server) syncOneAccount(ctx context.Context, a *network.Account) []string {
	return s.automation().SyncOneAccount(ctx, a)
}

// reconcileCommissions delegates to the automation service (R2-W1 money
// writer; reads credentials live from the env provider — no stale
// startup config).
func (s *Server) reconcileCommissions(ctx context.Context) []string {
	return s.automation().ReconcileCommissions(ctx)
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
	msg := "Đã tắt sản xuất & đăng tự động theo kế hoạch."
	if on {
		msg = "Đã bật sản xuất & đăng tự động: hệ thống tự sản xuất theo plan, tự đăng YouTube trong quota (TikTok vẫn nháp trước audit). DRY-RUN/kill switch vẫn chặn như thường."
	}
	if err := s.automation().SetProductionEnabled(on); err != nil {
		s.renderGrowth(w, []string{"Không lưu được cài đặt: " + err.Error()})
		return
	}
	_ = s.Ledger.Decide("growth", "production_toggle", nil, msg, map[string]any{"enabled": on})
	s.renderGrowth(w, []string{msg})
}

// handleGrowthProductionRun runs one automation pass immediately (the
// same pass the timer runs; the toggle is not required for an explicit
// run, kill switch and dry-run still apply).
func (s *Server) handleGrowthProductionRun(w http.ResponseWriter, r *http.Request) {
	notes := s.automation().GrowthTick(r.Context(), true)
	if len(notes) == 0 {
		notes = []string{"Vòng chạy xong: không có mục nào đến hạn hoặc đang chờ."}
	}
	s.renderGrowth(w, notes)
}

// handleGrowthThresholds saves the UI-tuned growth governance thresholds
// (R2-W4): kill / double-down / breakout / penalty knobs no longer live
// only as hard-coded defaults.
func (s *Server) handleGrowthThresholds(w http.ResponseWriter, r *http.Request) {
	if s.Growth == nil {
		s.renderGrowth(w, []string{"Module Phát triển kênh chưa sẵn sàng."})
		return
	}
	if r.FormValue("reset") == "1" {
		if err := automation.ResetThresholds(s.settings()); err != nil {
			s.renderGrowth(w, []string{"Không đặt lại được ngưỡng: " + err.Error()})
			return
		}
		_ = s.Ledger.Decide("growth", "thresholds_reset", nil, "ngưỡng growth về mặc định", nil)
		s.renderGrowth(w, []string{"Đã đặt lại ngưỡng về mặc định."})
		return
	}
	cfg := automation.LoadThresholds(s.settings()) // current values as base
	num := func(name string, cur float64) float64 {
		if raw := r.FormValue(name); raw != "" {
			var f float64
			if _, err := fmt.Sscanf(raw, "%f", &f); err == nil && f >= 0 {
				return f
			}
		}
		return cur
	}
	intg := func(name string, cur int) int {
		if raw := r.FormValue(name); raw != "" {
			var n int
			if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n >= 0 {
				return n
			}
		}
		return cur
	}
	cfg.KillMinVideos = intg("kill_min_videos", cfg.KillMinVideos)
	cfg.KillMinDays = intg("kill_min_days", cfg.KillMinDays)
	cfg.KillCompletionFloor = num("kill_completion_floor", cfg.KillCompletionFloor)
	cfg.KillProxyFloor = num("kill_proxy_floor", cfg.KillProxyFloor)
	cfg.DoubleDownFactor = num("double_down_factor", cfg.DoubleDownFactor)
	cfg.BreakoutFactor = num("breakout_factor", cfg.BreakoutFactor)
	cfg.StalledDays = intg("stalled_days", cfg.StalledDays)
	if err := automation.SaveThresholds(s.settings(), cfg); err != nil {
		s.renderGrowth(w, []string{"Không lưu được ngưỡng: " + err.Error()})
		return
	}
	_ = s.Ledger.Decide("growth", "thresholds_updated", nil, "ngưỡng growth cập nhật qua UI", nil)
	s.renderGrowth(w, []string{"Đã lưu ngưỡng growth."})
}
