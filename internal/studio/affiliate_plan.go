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
		p.VideoEngine = AffiliateVideoEngineGoogleFlow
	}
	if p.VideoEngine != AffiliateVideoEngineGoogleFlow && p.VideoEngine != AffiliateVideoEngineManual {
		return empty, fmt.Errorf("engine affiliate không được hỗ trợ: %q", p.VideoEngine)
	}
	if p.VideoEngine == AffiliateVideoEngineGoogleFlow {
		if ready, reason := s.WebAIVideoReady(AffiliateVideoEngineGoogleFlow); !ready {
			return empty, fmt.Errorf("Google Flow chưa sẵn sàng: %s", reason)
		}
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
	expiresBefore := time.Now().UTC().Add(-affiliatePreparationLifetime).Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`DELETE FROM studio_affiliate_plans WHERE created_at < ?`, expiresBefore); err != nil {
		return empty, fmt.Errorf("không dọn được shot plan cũ: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO studio_affiliate_plans(id,payload,created_at) VALUES(?,?,?)`, prep.ID, string(data), prep.CreatedAt); err != nil {
		return empty, fmt.Errorf("không lưu được shot plan: %w", err)
	}
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
		_, _ = s.db.Exec(`DELETE FROM studio_affiliate_plans WHERE id = ?`, id)
		return AffiliatePreparation{}, fmt.Errorf("shot plan đã hết hạn; hãy lập lại plan")
	}
	return prep, nil
}

// ReissueAffiliatePreparation makes a fresh one-use plan token for retrying an
// already reviewed job. The saved product brief and shot list remain unchanged.
func (s *Studio) ReissueAffiliatePreparation(p AffiliateParams) (string, error) {
	if p.PreparedProductBrief == nil || len(p.PreparedShots) == 0 {
		return "", fmt.Errorf("job cũ không lưu Product Analyst brief và Director shot list; cần lập plan mới")
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

func (s *Studio) consumeAffiliatePreparation(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("thiếu mã shot plan cần tiêu thụ")
	}
	result, err := s.db.Exec(`DELETE FROM studio_affiliate_plans WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("shot plan đã được dùng hoặc không còn tồn tại; hãy lập plan mới")
	}
	return nil
}

// AffiliateManualVideoPrompt turns one approved shot into a usable prompt for
// an operator-operated video tool or a live-action shoot.
func AffiliateManualVideoPrompt(productName string, shot ProShot) string {
	return fmt.Sprintf(`Create one vertical 9:16 video clip, exactly %d seconds. Use the selected channel character reference images; preserve the same person's face, hair, body, and identity. Show that character physically using the exact %s from the listing references. Required use mode for this shot: %s. Keep the product in that use mode for the full shot; a detail close-up must still show the item being worn or used, not isolated or merely held. Do not substitute another item or invent product details. Keep the visible product detail sharp: %s. Action: %s. Framing: %s; camera: %s; lens and light: %s. Emotion: %s. Wardrobe continuity: %s. Setting continuity: %s. Motion direction: %s. No captions, added text, or watermark.`,
		shot.Seconds, productName, shot.ProductUseMode, shot.ProductFocus, shot.ProductInteraction, shot.ShotSize,
		shot.CameraMove, shot.LensLight, shot.EmotionBeat,
		shot.WardrobeStyling, shot.SceneSetting, shot.VideoPrompt)
}
