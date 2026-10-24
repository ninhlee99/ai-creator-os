package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"log"
	"os"
	"path/filepath"
	"time"
)

func (s *Service) ATHunterTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyATHunterEnabled, true) {
		return nil
	}
	cl, ok := s.atClient()
	if !ok {
		log.Printf("accesstrade hunter: chưa có access_key — bỏ qua")
		return nil
	}
	if s.Products == nil {
		return nil
	}
	interval := time.Duration(atInt(s.Settings, KeyATHunterIntervalHrs, defaultATHunterHours)) * time.Hour
	if interval <= 0 {
		interval = defaultATHunterHours * time.Hour
	}
	if !dueSince(s.Settings, keyATHunterLastRun, interval) {
		return nil
	}
	res, err := cl.HuntTikTokShop(ctx, s.Products, accesstrade.HunterFilter{MaxAgeDays: 180})
	if err != nil {
		return []string{"săn sản phẩm thất bại: " + err.Error()}
	}
	if res.Note != "" && res.New == 0 && res.Updated == 0 && len(res.ProductionCandidates) == 0 {
		stampRun(s.Settings, keyATHunterLastRun)
		_ = s.decide("at_hunter", "hunt_blocked", nil, res.Note, nil)
		return []string{"săn sản phẩm: " + res.Note}
	}
	if len(res.ProductionCandidates) == 0 {
		stampRun(s.Settings, keyATHunterLastRun)
		note := res.Note
		if note == "" {
			note = "không có offer nào đã duyệt và đủ giá, hoa hồng, link hợp lệ để tạo video affiliate"
		}
		if !atOn(s.Settings, KeyATHunterFlowCredits, false) {
			note += " Google Flow đang tắt vì chưa bật quyền dùng credits."
		}
		_ = s.decide("at_hunter", "hunt_blocked_evidence", nil, note, nil)
		return []string{"săn sản phẩm: " + note}
	}
	videos := 0
	want := atInt(s.Settings, KeyATHunterVideos, defaultATHunterVideos)
	flowCreditsDisabled := !atOn(s.Settings, KeyATHunterFlowCredits, false)
	if flowCreditsDisabled {
		want = 0
	}
	if s.StudioRunner != nil && want > 0 {
		if readiness, ok := s.StudioRunner.(interface{ AutoAffiliateProductionReady() (bool, string) }); ok {
			ready, reason := readiness.AutoAffiliateProductionReady()
			if !ready {
				stampRun(s.Settings, keyATHunterLastRun)
				return []string{fmt.Sprintf("săn sản phẩm: %d mới, %d cập nhật, %d bỏ qua → 0 video; %s", res.New, res.Updated, res.Skipped, reason)}
			}
		}
	}
	var accountID int64
	var accountErr error
	if s.StudioRunner != nil && want > 0 {
		accountID, accountErr = s.HunterAccountID()
	}
	if s.StudioRunner != nil && want > 0 && accountErr == nil {
		for _, p := range res.ProductionCandidates {
			if videos >= want {
				break
			}
			// Đợt Y: claim trước khi dựng video (như Autopilot.Run) —
			// nút "Săn ngay" bấm tay có thể đua với tick định kỳ.
			claimed, cerr := s.Products.RecordUse(p.ID, accountID, "")
			if cerr != nil || !claimed {
				continue
			}
			jobID, jerr := s.MakeHunterVideoForAccount(ctx, p, accountID)
			if jerr != nil {
				_ = s.Products.ReleaseUseClaim(p.ID, accountID)
				log.Printf("accesstrade hunter: tạo video cho %q thất bại: %v", p.Title, jerr)
				continue
			}
			_ = s.Products.SetUsageJobID(p.ID, accountID, jobID)
			videos++
		}
	}
	stampRun(s.Settings, keyATHunterLastRun)
	note := fmt.Sprintf("săn sản phẩm: %d mới, %d watchlist thiếu sales/rating, %d cập nhật, %d bỏ qua → %d video",
		res.New, res.Watchlist, res.Updated, res.Skipped, videos)
	if flowCreditsDisabled {
		note += "; video tự động đang tắt đến khi bật quyền dùng Google Flow credits"
	}
	if accountErr != nil && s.StudioRunner != nil && want > 0 {
		note += "; chưa tạo video: " + accountErr.Error()
	}
	return []string{note}
}

// MakeHunterVideo tạo 1 studio job affiliate từ sản phẩm hunter:
// ảnh listing thật (product lock) + nhạc nền, không chữ không voiceover
// (đúng format Ninh chốt). Không viết lại renderer — gọi CreateAffiliateJob.
// Export để web handler "Săn sản phẩm" dùng chung logic với tick.
func (s *Service) MakeHunterVideo(ctx context.Context, p products.Product) (string, error) {
	accountID, err := s.HunterAccountID()
	if err != nil {
		return "", err
	}
	return s.MakeHunterVideoForAccount(ctx, p, accountID)
}

// MakeHunterVideoForAccount renders with identity photos owned by accountID.
func (s *Service) MakeHunterVideoForAccount(ctx context.Context, p products.Product, accountID int64) (string, error) {
	if accountID <= 0 {
		return "", fmt.Errorf("cần chọn kênh đích cho video hunter")
	}
	if s.StudioRunner == nil {
		return "", fmt.Errorf("Studio chưa sẵn sàng")
	}
	if !atOn(s.Settings, KeyATHunterFlowCredits, false) {
		return "", fmt.Errorf("chưa bật quyền dùng Google Flow credits cho video hunter tự động")
	}
	if readiness, ok := s.StudioRunner.(interface{ AutoAffiliateProductionReady() (bool, string) }); ok {
		ready, reason := readiness.AutoAffiliateProductionReady()
		if !ready {
			return "", fmt.Errorf("%s", reason)
		}
	}
	modelSource, ok := s.StudioRunner.(interface{ ModelPhotosForAccount(int64) []string })
	if !ok {
		return "", fmt.Errorf("Studio chưa hỗ trợ thư viện nhân vật theo kênh")
	}
	modelPhotos := modelSource.ModelPhotosForAccount(accountID)
	if len(modelPhotos) == 0 {
		return "", fmt.Errorf("kênh %d chưa có ảnh nhân vật riêng", accountID)
	}
	if len(p.ImageURLs) == 0 {
		return "", fmt.Errorf("sản phẩm %q không có ảnh", p.Title)
	}
	work := s.ATWorkDir
	if work == "" {
		work = filepath.Join(os.TempDir(), "aicos-at-hunter")
	}
	dir := filepath.Join(work, fmt.Sprint(p.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "prod.jpg")
	if err := studio.DownloadImage(ctx, p.ImageURLs[0], dst); err != nil {
		return "", fmt.Errorf("tải ảnh: %w", err)
	}
	// Ảnh listing thêm (góc khác) → video nhiều ảnh thật, không lặp 1 ảnh.
	var extra []string
	for i, u := range p.ImageURLs[1:] {
		if len(extra) >= 5 {
			break
		}
		e := filepath.Join(dir, fmt.Sprintf("prod-%d.jpg", i+1))
		if studio.DownloadImage(ctx, u, e) == nil {
			extra = append(extra, e)
		}
	}
	niche := p.Category
	if niche == "" {
		niche = p.Theme
	}
	params := studio.AffiliateParams{
		Mode:                    studio.AffiliateModeShots,
		VideoEngine:             studio.AffiliateVideoEngineGoogleFlow,
		FlowCreditsAcknowledged: true,
		AccountID:               accountID,
		ProductID:               p.ID,
		Niche:                   niche,
		ProductName:             p.Title,
		ModelPhoto:              modelPhotos[0],
		ModelPhotos:             modelPhotos,
		ProductPhoto:            dst,
		ExtraPhotos:             extra,
		Seconds:                 30,
		AffLink:                 p.ProductURL, // affiliate link used by the TikTok campaign
	}
	soundResolver, ok := s.StudioRunner.(interface {
		ResolveTikTokRadarMusic(context.Context) (studio.TikTokTrendVideo, error)
	})
	if !ok {
		return "", fmt.Errorf("Studio chưa hỗ trợ nguồn nhạc đã xác nhận từ Radar TikTok")
	}
	soundSource, soundErr := soundResolver.ResolveTikTokRadarMusic(ctx)
	if soundErr != nil {
		return "", fmt.Errorf("lấy sound TikTok từ Radar: %w", soundErr)
	}
	params.SourceMusicTitle = soundSource.SoundTitle()
	params.SourceMusicVideoURL = soundSource.VideoURL
	planner, ok := s.StudioRunner.(interface {
		PrepareAffiliatePlan(context.Context, studio.AffiliateParams) (studio.AffiliatePreparation, error)
	})
	if !ok {
		return "", fmt.Errorf("Studio chưa hỗ trợ Product Analyst + Director shot plan")
	}
	plan, err := planner.PrepareAffiliatePlan(ctx, params)
	if err != nil {
		return "", fmt.Errorf("lập shot plan: %w", err)
	}
	params.PreparedPlanID = plan.ID
	if p.Source == accesstrade.HunterSource {
		client, ok := s.atClient()
		if !ok {
			return "", fmt.Errorf("AccessTrade chưa sẵn sàng để tạo link tracking cho video")
		}
		link, err := client.CreateTikTokVideoTrackingLink(ctx, p, plan.ID)
		if err != nil {
			return "", fmt.Errorf("tạo link tracking cho video: %w", err)
		}
		params.AffLink = link.TrackingURL
		params.AffiliateAttributionID = plan.ID
	}
	return s.StudioRunner.CreateAffiliateJob(params)
}

// -------------------------------------------------------- order sync tick

// ATOrderSyncTick đồng bộ đơn 30 phút/lần: order-list trong cửa sổ rolling
// 7 ngày → upsert + UPDATE pending tại chỗ → ghi watermark.
