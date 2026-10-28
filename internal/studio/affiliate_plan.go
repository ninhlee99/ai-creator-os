package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// AffiliatePreparation is a reviewed pre-production brief saved before the
// operator records or generates motion clips.
type AffiliatePreparation struct {
	ID           string                  `json:"id"`
	Params       AffiliateParams         `json:"params"`
	ProductBrief AffiliateProductProfile `json:"product_brief"`
	Shots        []ProShot               `json:"shots"`
	CreatedAt    string                  `json:"created_at"`
}

const affiliatePreparationLifetime = 7 * 24 * time.Hour

// PrepareAffiliatePlan analyzes product references and writes the exact shot
// list that the later clip upload must follow.
func (s *Studio) PrepareAffiliatePlan(ctx context.Context, p AffiliateParams) (AffiliatePreparation, error) {
	var empty AffiliatePreparation
	keepUploads := false
	defer func() {
		if !keepUploads {
			s.cleanupAffiliatePlanInputs(p)
		}
	}()
	source, err := s.ResolveTikTokRadarDanceVideo(ctx, p.SourceMusicVideoURL)
	if err != nil {
		return empty, fmt.Errorf("chọn video nhảy có sound trong Radar TikTok: %w", err)
	}
	if !source.DanceVerified {
		return empty, fmt.Errorf("video Radar chưa được xác nhận là video nhảy")
	}
	p.SourceMusicTitle = source.SoundTitle()
	p.SourceMusicVideoURL = source.VideoURL
	p.SourceMusicVideoID = source.VideoID
	p.SourceMusicAuthor = source.Author
	p.SourceMusicCollectedAt = source.CollectedAt
	p.ChannelProfile = normalizeChannelCreativeProfile(p.ChannelProfile)
	if p.Seconds <= 0 {
		p.Seconds = 60
	}
	if p.Seconds < 30 {
		p.Seconds = 30
	}
	if p.Seconds > 180 {
		p.Seconds = 180
	}
	if p.Mode == "" {
		p.Mode = AffiliateModeShots
	}
	if p.Mode != AffiliateModeShots {
		return empty, fmt.Errorf("affiliate plan chỉ hỗ trợ video theo shot chuyển động")
	}
	p.ProductName = strings.TrimSpace(p.ProductName)
	if p.ProductName == "" {
		return empty, fmt.Errorf("thiếu tên sản phẩm")
	}
	if p.AccountID <= 0 {
		return empty, fmt.Errorf("cần chọn kênh để khóa đúng ảnh nhân vật")
	}
	if !fileExists(p.ProductPhoto) {
		return empty, fmt.Errorf("thiếu ảnh listing sản phẩm thật")
	}
	if p.VideoEngine == "" {
		p.VideoEngine = AffiliateVideoEngineMuseAI
	}
	if p.VideoEngine == AffiliateVideoEngineGoogleFlow {
		p.VideoEngine = AffiliateVideoEngineMuseAI
	}
	if p.VideoEngine == AffiliateVideoEngineManual {
		p.VideoEngine = AffiliateVideoEngineMuseAI
	}
	if p.VideoEngine != AffiliateVideoEngineMuseAI {
		return empty, fmt.Errorf("engine affiliate không được hỗ trợ: %q", p.VideoEngine)
	}
	accountPhotos := s.queryAccountModelPhotos(p.AccountID)
	allowed := make(map[string]bool, len(accountPhotos))
	for _, photo := range accountPhotos {
		allowed[filepath.Clean(photo)] = true
	}
	if p.ModelPhoto == "" {
		for _, photo := range p.ModelPhotos {
			if allowed[filepath.Clean(photo)] {
				p.ModelPhoto = photo
				break
			}
		}
	}
	if !fileExists(p.ModelPhoto) || !allowed[filepath.Clean(p.ModelPhoto)] {
		return empty, fmt.Errorf("thiếu ảnh nhân vật thuộc thư viện của kênh đã chọn")
	}
	seen := map[string]bool{filepath.Clean(p.ModelPhoto): true}
	selected := []string{p.ModelPhoto}
	for _, photo := range p.ModelPhotos {
		clean := filepath.Clean(photo)
		if !allowed[clean] {
			return empty, fmt.Errorf("ảnh nhân vật không thuộc thư viện của kênh đã chọn")
		}
		if !seen[clean] {
			seen[clean] = true
			selected = append(selected, photo)
		}
	}
	p.ModelPhotos = selected
	if len(affiliateModelReferences(p, 3)) == 0 || len(affiliateProductReferences(p)) == 0 {
		return empty, fmt.Errorf("thiếu ảnh tham chiếu của nhân vật hoặc sản phẩm")
	}
	if s.mg == nil || !s.mg.Healthy(ctx) {
		return empty, fmt.Errorf("media engine chưa sẵn sàng để phân tích listing")
	}
	analyzer, ok := s.mg.(visualImageAnalyzer)
	if !ok {
		return empty, fmt.Errorf("media engine chưa hỗ trợ phân tích ảnh listing")
	}
	if s.llm == nil || !s.llm.Healthy(ctx) {
		return empty, fmt.Errorf("LLM chưa sẵn sàng để viết shot list")
	}

	brief, err := analyzeAffiliateProduct(ctx, analyzer, p)
	if err != nil {
		return empty, err
	}
	learning := ""
	if s.PromptEnhancer != nil {
		learning = s.PromptEnhancer(ctx, p.Niche, p.ChannelMemoryKey)
	}
	shots, err := WriteAffiliateShotListForChannelWithLearning(ctx, s.llm, p.ProductName, p.Niche, p.Seconds, brief, p.ChannelProfile, learning)
	if err != nil {
		return empty, fmt.Errorf("shot list: %w", err)
	}
	p.PreparedProductBrief = &brief
	p.PreparedShots = shots
	prep := AffiliatePreparation{
		ID:           "aff-" + newID(),
		Params:       p,
		ProductBrief: brief,
		Shots:        shots,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(prep)
	if err != nil {
		return empty, fmt.Errorf("không lưu được shot plan: %w", err)
	}
	if err := s.pruneExpiredAffiliatePlans(time.Now().UTC()); err != nil {
		return empty, fmt.Errorf("không dọn được shot plan cũ: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO studio_affiliate_plans(id,payload,created_at) VALUES(?,?,?)`, prep.ID, string(data), prep.CreatedAt); err != nil {
		return empty, fmt.Errorf("không lưu được shot plan: %w", err)
	}
	keepUploads = true
	return prep, nil
}

// GetAffiliatePreparation loads a pre-production plan that has not yet been
// consumed by a job.
func (s *Studio) GetAffiliatePreparation(id string) (AffiliatePreparation, error) {
	var prep AffiliatePreparation
	if strings.TrimSpace(id) == "" {
		return prep, fmt.Errorf("thiếu mã shot plan")
	}
	var payload string
	if err := s.db.QueryRow(`SELECT payload FROM studio_affiliate_plans WHERE id = ?`, id).Scan(&payload); err != nil {
		return prep, fmt.Errorf("shot plan không tồn tại hoặc đã được dùng")
	}
	if err := json.Unmarshal([]byte(payload), &prep); err != nil {
		return AffiliatePreparation{}, fmt.Errorf("không đọc được shot plan: %w", err)
	}
	created, err := time.Parse(time.RFC3339Nano, prep.CreatedAt)
	if err != nil || time.Since(created) > affiliatePreparationLifetime {
		if result, deleteErr := s.db.Exec(`DELETE FROM studio_affiliate_plans WHERE id = ?`, id); deleteErr == nil {
			if deleted, _ := result.RowsAffected(); deleted == 1 {
				s.cleanupAffiliatePlanInputs(prep.Params)
			}
		}
		return AffiliatePreparation{}, fmt.Errorf("shot plan đã hết hạn; hãy lập lại plan")
	}
	return prep, nil
}

func (s *Studio) pruneExpiredAffiliatePlans(now time.Time) error {
	cutoff := now.Add(-affiliatePreparationLifetime).Format(time.RFC3339Nano)
	rows, err := s.db.Query(`SELECT id,payload FROM studio_affiliate_plans WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}
	type stalePlan struct {
		id    string
		prep  AffiliatePreparation
		valid bool
	}
	var stale []stalePlan
	for rows.Next() {
		var item stalePlan
		var payload string
		if err := rows.Scan(&item.id, &payload); err != nil {
			rows.Close()
			return err
		}
		item.valid = json.Unmarshal([]byte(payload), &item.prep) == nil
		stale = append(stale, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(stale) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var expired []AffiliateParams
	for _, item := range stale {
		result, err := tx.Exec(`DELETE FROM studio_affiliate_plans WHERE id=? AND created_at < ?`, item.id, cutoff)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if deleted == 1 && item.valid {
			expired = append(expired, item.prep.Params)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, params := range expired {
		s.cleanupAffiliatePlanInputs(params)
	}
	return nil
}

func (s *Studio) cleanupAffiliatePlanInputs(params AffiliateParams) {
	data, err := json.Marshal(params)
	if err != nil {
		return
	}
	s.removeJobInputUploads(Job{Kind: KindAffiliate, Params: string(data)})
}

// ReissueAffiliatePreparation makes a fresh one-use plan token for retrying an
// already reviewed job. It rebuilds the shot list when requested duration changed.
func (s *Studio) ReissueAffiliatePreparation(ctx context.Context, p AffiliateParams) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p.PreparedProductBrief == nil || len(p.PreparedShots) == 0 {
		return "", fmt.Errorf("job cũ không lưu Product Analyst brief và Director shot list; cần lập plan mới")
	}
	if p.Seconds < 30 {
		p.Seconds = 30
	} else if p.Seconds > 180 {
		p.Seconds = 180
	}
	if !affiliateShotPlanMatchesDuration(p.Seconds, p.PreparedShots) {
		if s.llm == nil || !s.llm.Healthy(ctx) {
			return "", fmt.Errorf("LLM chưa sẵn sàng để lập lại shot plan cho thời lượng %d giây", p.Seconds)
		}
		shots, err := WriteAffiliateShotListForChannel(ctx, s.llm, p.ProductName, p.Niche, p.Seconds, *p.PreparedProductBrief, p.ChannelProfile)
		if err != nil {
			return "", fmt.Errorf("không lập lại được shot plan cho %d giây: %w", p.Seconds, err)
		}
		p.PreparedShots = shots
	}
	p.PreparedPlanID = ""
	prep := AffiliatePreparation{
		ID:           "aff-" + newID(),
		Params:       p,
		ProductBrief: *p.PreparedProductBrief,
		Shots:        append([]ProShot(nil), p.PreparedShots...),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(prep)
	if err != nil {
		return "", fmt.Errorf("không mã hóa được shot plan mới: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO studio_affiliate_plans(id,payload,created_at) VALUES(?,?,?)`, prep.ID, string(data), prep.CreatedAt); err != nil {
		return "", fmt.Errorf("không lưu được shot plan mới: %w", err)
	}
	return prep.ID, nil
}

// AffiliateManualVideoPrompt turns one approved shot into a usable prompt for
// an operator-operated video tool or a live-action shoot.
func AffiliateManualVideoPrompt(productName string, shot ProShot) string {
	return fmt.Sprintf(`Create one vertical 9:16 video clip, exactly %d seconds. Use the selected channel character reference images; preserve the same person's face, hair, body, and identity. Show that character physically using the exact %s from the listing references. Required use mode for this shot: %s. Keep the product in that use mode for the full shot; a detail close-up must still show the item being worn or used, not isolated or merely held. Do not substitute another item or invent product details. Keep the visible product detail sharp: %s. Action: %s. Framing: %s; camera: %s; lens and light: %s. Emotion: %s. Wardrobe continuity: %s. Setting continuity: %s. Motion direction: %s. No captions, added text, or watermark.`,
		shot.Seconds, productName, shot.ProductUseMode, shot.ProductFocus, shot.ProductInteraction, shot.ShotSize,
		shot.CameraMove, shot.LensLight, shot.EmotionBeat,
		shot.WardrobeStyling, shot.SceneSetting, shot.VideoPrompt)
}
