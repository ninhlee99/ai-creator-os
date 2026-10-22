package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func (s *Server) executeAffiliateMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 40
	m.CurrentStep = "Chọn sản phẩm có thật và kiểm tra đầu vào"
	goalLower := strings.ToLower(m.Goal)
	fail := func(stepID, title, detail string) {
		m.Steps = append(m.Steps, MissionStep{ID: stepID, Title: title, Status: "failed", Detail: detail, Timestamp: now})
		m.Progress = 40
		m.Status = "failed"
		m.CurrentStep = title
		m.ResultSummary = detail
	}
	if s.Products == nil {
		fail("step-2", "Chọn sản phẩm thật", "Kho sản phẩm chưa sẵn sàng; chưa tạo video.")
		return
	}
	if s.Studio == nil {
		fail("step-2", "Kiểm tra đầu vào Studio", "Studio chưa sẵn sàng; chưa tạo video.")
		return
	}
	if !m.FlowCreditsAcknowledged {
		m.Steps = append(m.Steps, MissionStep{
			ID: "step-2", Title: "Xác nhận credits Google Flow", Status: "pending",
			Detail: "Google Flow có thể dùng credits. Gửi lại yêu cầu với confirm_flow_credits=1 để tiếp tục; chưa tải ảnh hoặc tạo video.", Timestamp: now,
		})
		m.Status = "waiting_approval"
		m.Progress = 40
		m.CurrentStep = "Chờ xác nhận credits Google Flow"
		m.ResultSummary = "Chưa tạo video. Cần xác nhận Google Flow có thể dùng credits."
		return
	}
	if ready, reason := s.Studio.AutoAffiliateProductionReady(); !ready {
		fail("step-2", "Affiliate cần shot plan và footage", reason+"; mission chưa tải ảnh sản phẩm hoặc tạo job.")
		return
	}
	if m.AccountID <= 0 {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Chưa có kênh đích để khóa ảnh nhân vật; chưa tạo video.")
		return
	}

	shelf, err := s.Products.Shelf(100)
	if err != nil {
		fail("step-2", "Chọn sản phẩm thật", "Không đọc được Kệ hàng: "+err.Error())
		return
	}
	var selected *products.Product
	var exactMatches []*products.Product
	var categoryMatches []*products.Product
	evidenceBlocked := false
	for i := range shelf {
		candidate := &shelf[i]
		if strings.TrimSpace(candidate.Title) == "" || len(candidate.ImageURLs) == 0 || strings.TrimSpace(candidate.ImageURLs[0]) == "" {
			continue
		}
		matchesGoal := missionProductMatchesGoal(*candidate, goalLower)
		title := strings.ToLower(strings.TrimSpace(candidate.Title))
		exactMatch := title != "" && strings.Contains(goalLower, title)
		if !matchesGoal && !exactMatch {
			continue
		}
		if !candidate.HasDemandSignals() {
			evidenceBlocked = true
			continue
		}
		if matchesGoal {
			categoryMatches = append(categoryMatches, candidate)
		}
		if exactMatch {
			exactMatches = append(exactMatches, candidate)
		}
	}
	if len(exactMatches) == 1 {
		selected = exactMatches[0]
	} else if len(exactMatches) > 1 {
		fail("step-2", "Chọn sản phẩm cụ thể", "Tên sản phẩm khớp nhiều mục trong Kệ hàng; chỉnh tên hoặc yêu cầu cụ thể hơn.")
		return
	} else if len(categoryMatches) == 1 {
		selected = categoryMatches[0]
	} else if len(categoryMatches) > 1 {
		fail("step-2", "Chọn sản phẩm cụ thể", "Có nhiều sản phẩm cùng khớp ngành hàng; nêu chính xác tên sản phẩm để tránh chọn nhầm.")
		return
	}
	if selected == nil {
		if evidenceBlocked {
			fail("step-2", "Chờ dữ liệu sản phẩm", "Sản phẩm khớp yêu cầu nhưng thiếu lượt bán hoặc rating; chưa tạo video affiliate.")
			return
		}
		fail("step-2", "Chọn sản phẩm thật", "Không có sản phẩm phù hợp kèm ảnh thật trong Kệ hàng; thêm sản phẩm rồi thử lại.")
		return
	}

	library, err := s.Studio.ModelLibrary()
	if err != nil {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Không đọc được thư viện nhân vật của kênh: "+err.Error())
		return
	}
	modelPhotos, err := library.List(m.AccountID)
	if err != nil {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Không đọc được ảnh nhân vật của kênh: "+err.Error())
		return
	}
	var modelPhoto string
	var channelModelPhotos []string
	for _, photo := range modelPhotos {
		if fileExists(photo.Path) {
			channelModelPhotos = append(channelModelPhotos, photo.Path)
			if modelPhoto == "" {
				modelPhoto = photo.Path
			}
		}
	}
	if modelPhoto == "" {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Kênh chưa có ảnh nhân vật đã lưu; thêm ảnh đúng kênh rồi thử lại.")
		return
	}

	uploadDir := s.Studio.UploadDir()
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		fail("step-2", "Tải ảnh sản phẩm thật", "Không tạo được thư mục ảnh sản phẩm: "+err.Error())
		return
	}
	productPhotos := make([]string, 0, 4)
	for i, imageURL := range selected.ImageURLs {
		if i >= 4 {
			break
		}
		dst := filepath.Join(uploadDir, fmt.Sprintf("mission-product-%d-%d.jpg", time.Now().UnixNano(), i))
		downloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		downloadErr := studio.DownloadImage(downloadCtx, imageURL, dst)
		cancel()
		if downloadErr == nil && fileExists(dst) {
			productPhotos = append(productPhotos, dst)
		}
	}
	if len(productPhotos) == 0 {
		fail("step-2", "Tải ảnh sản phẩm thật", "Không tải được ảnh listing sản phẩm; chưa tạo video.")
		return
	}

	productName := selected.Title
	niche := selected.Category
	commissionDetail := "hoa hồng chưa có dữ liệu"
	if selected.CommissionRate > 0 {
		commissionDetail = fmt.Sprintf("hoa hồng theo kho %.1f%%", selected.CommissionRate*100)
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Chọn sản phẩm và khóa ảnh tham chiếu", Status: "done",
		Detail: fmt.Sprintf("Sản phẩm trong Kệ hàng: '%s' · %s · ảnh sản phẩm và ảnh nhân vật kênh đã xác nhận.", productName, commissionDetail), Timestamp: now,
	})

	m.Progress = 70
	m.CurrentStep = "Xếp job Studio với sản phẩm và nhân vật đã xác nhận"
	params := studio.AffiliateParams{
		Mode:                    studio.AffiliateModeShots,
		VideoEngine:             studio.AffiliateVideoEngineGoogleFlow,
		FlowCreditsAcknowledged: m.FlowCreditsAcknowledged,
		AccountID:               m.AccountID,
		ModelPhoto:              modelPhoto,
		ModelPhotos:             channelModelPhotos,
		Niche:                   niche,
		ProductName:             productName,
		ProductPhoto:            productPhotos[0],
		ExtraPhotos:             productPhotos[1:],
		Seconds:                 30,
		BPM:                     120,
		AspectRatio:             "9:16",
		AffLink:                 selected.ProductURL,
	}
	if selected.Price > 0 {
		params.ProductPrice = fmt.Sprintf("%.0f ₫", selected.Price)
	}
	preparation, planErr := s.Studio.PrepareAffiliatePlan(ctx, params)
	if planErr != nil {
		fail("step-3", "Lập Product Analyst + Director shot plan", planErr.Error())
		return
	}
	params.PreparedPlanID = preparation.ID
	studioJobID, queueErr := s.Studio.CreateAffiliateJob(params)
	if queueErr != nil {
		detail := "Không xếp được job Studio: " + queueErr.Error()
		fail("step-3", "Xếp job Studio", detail)
		return
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-3",
		Title:     "Xếp job Affiliate vào Studio",
		Status:    "done",
		Detail:    fmt.Sprintf("Đã xếp job %s; video chưa hoàn tất và chưa đăng.", studioJobID),
		Timestamp: now,
	})

	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/jobs?tab=running"
	m.ResultSummary = fmt.Sprintf("Đã đưa job video cho '%s' vào hàng đợi (job %s). Video chưa hoàn tất; visual QC sẽ chạy trước khi job được đánh dấu xong.", productName, studioJobID)
}

func missionProductMatchesGoal(p products.Product, goal string) bool {
	haystack := strings.ToLower(strings.TrimSpace(p.Title + " " + p.Category))
	if title := strings.ToLower(strings.TrimSpace(p.Title)); title != "" && strings.Contains(goal, title) {
		return true
	}
	categories := [][]string{
		{"mỹ phẩm", "serum", "skincare", "son", "kem dưỡng", "beauty", "cosmetic"},
		{"công nghệ", "tai nghe", "điện thoại", "laptop", "tech", "electronics", "headphone"},
		{"gia dụng", "nồi", "bếp", "máy hút", "home", "kitchen", "appliance"},
		{"thời trang", "áo", "quần", "váy", "cardigan", "túi", "fashion", "apparel"},
	}
	for _, terms := range categories {
		requested := false
		matchesProduct := false
		for _, term := range terms {
			requested = requested || strings.Contains(goal, term)
			matchesProduct = matchesProduct || strings.Contains(haystack, term)
		}
		if requested {
			return matchesProduct
		}
	}
	return false
}
