package web

import (
	"fmt"
	"strings"
)

func (s *Server) affiliateTeam() pipeTeam {
	stages := []pipeStage{}
	// Hunter: quét AT datafeed → kho sản phẩm.
	if s.AT == nil || s.Products == nil {
		stages = append(stages, pipeStage{Key: "hunter", Label: "Hunter", Icon: "🔎", Status: tmIdle, Detail: "chưa cấu hình Accesstrade", Link: "/products", RoleDesc: "Cào & săn deal sản phẩm hoa hồng cao từ sàn"})
	} else {
		n, _ := s.Products.Count()
		camps, _ := s.AT.CachedCampaigns()
		st := tmWaiting
		if n > 0 {
			st = tmOK
		}
		stages = append(stages, pipeStage{Key: "hunter", Label: "Hunter", Icon: "🔎", Status: st,
			Detail: formatCount("sản phẩm", n) + " · " + formatCount("chiến dịch", int64(len(camps))),
			Link:   "/products", RoleDesc: "Cào & săn deal sản phẩm hoa hồng cao từ sàn"})
	}
	// These roles map to real, sequential job artifacts and log markers.
	// They are pipeline stages, not independent agents running in parallel.
	affiliateJob := s.latestJobByKind("affiliate")
	stages = append(stages, affiliateMarkedStage("Product Analyst", "🔬", affiliateJob,
		"PRODUCT_ANALYSIS_START:", "PRODUCT_ANALYSIS_PASS:", "PRODUCT_ANALYSIS_FAIL:",
		"Đọc ảnh listing, phân loại mặt hàng và ghi lại chi tiết nhìn thấy; không bịa claims."))
	stages = append(stages, affiliateMarkedStage("Director", "🎥", affiliateJob,
		"DIRECTOR_START:", "DIRECTOR_PASS:", "DIRECTOR_FAIL:",
		"Lập shot list theo cách dùng, focus sản phẩm, coverage và story arc."))
	stages = append(stages, affiliateMarkedStage("Producer / Editor", "🎬", affiliateJob,
		"PRODUCTION_START:", "PRODUCTION_PASS:", "PRODUCTION_FAIL:",
		"Chuẩn hóa clip chuyển động người vận hành cung cấp, nối cảnh và hòa âm; không gọi video API tính phí."))
	stages = append(stages, visualQCStage("QC thị giác", "🛡️", affiliateJob, "/studio/jobs?tab=running", "Chặn footage đứng hình ở 2 fps; đối chiếu sản phẩm, nhân vật, cảm xúc, nét, bố cục và continuity trên ba frame mỗi shot"))
	// Output: video đã render; publish không được coi là đã xong nếu chưa có post record.
	done := s.countJobsByKind("affiliate", "done")
	verified := s.countQCVerifiedJobsByKind("affiliate")
	outputStatus := tmWaiting
	if verified > 0 {
		outputStatus = tmOK
	}
	stages = append(stages, pipeStage{Key: "output", Label: "Đầu ra", Icon: "📤", Status: outputStatus,
		Detail: fmt.Sprintf("%d video qua QC · %d chưa xác minh", verified, done-verified), Link: "/studio/jobs?tab=done",
		RoleDesc: "Job hoàn tất chưa có nghĩa video đã được đăng lên kênh"})
	stages = append(stages, s.growthAnalyticsStage())
	// Analyst: đối soát hoa hồng AT.
	if s.AT == nil {
		stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: tmIdle, Detail: "chưa cấu hình Accesstrade", Link: "/products?tab=orders", RoleDesc: "Đối soát đơn hàng, hoa hồng và doanh thu"})
	} else {
		os_, _ := s.AT.GetOrderStats()
		st := tmWaiting
		if os_.ApprovedCount+os_.PendingCount > 0 {
			st = tmOK
		}
		stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: st,
			Detail: formatCount("đơn duyệt", os_.ApprovedCount) + " · " + formatCount("đơn chờ", os_.PendingCount),
			Link:   "/products?tab=orders", RoleDesc: "Đối soát đơn hàng, hoa hồng và doanh thu"})
	}
	return pipeTeam{
		Key:         "affiliate",
		Name:        "Affiliate",
		Icon:        "🛒",
		Description: "Trạng thái từng vai trò lấy từ artifact/log job thật. Đây là các chặng tuần tự, không giả lập agent độc lập hoặc kết quả tăng trưởng.",
		ActionLink:  "/studio",
		ActionLabel: "🎬 Dựng video mới",
		Stages:      stages,
	}
}

func affiliateMarkedStage(label, icon string, job *studioJobLite, startMarker, passMarker, failMarker, roleDesc string) pipeStage {
	key := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(label, " / ", "-"), " ", "-"))
	stage := pipeStage{Key: key, Label: label, Icon: icon,
		Link: "/studio/jobs?tab=running", RoleDesc: roleDesc, Status: tmWaiting, Detail: "chờ job affiliate"}
	if job == nil {
		stage.Detail = "chưa có job"
		return stage
	}
	if strings.Contains(job.Log, passMarker) {
		stage.Status = tmOK
		stage.Detail = visualQCLogDetail(job.Log, passMarker, "đã hoàn tất")
		return stage
	}
	if strings.Contains(job.Log, failMarker) {
		stage.Status = tmFail
		stage.Detail = visualQCLogDetail(job.Log, failMarker, "bước này bị chặn")
		return stage
	}
	if strings.Contains(job.Log, startMarker) {
		if job.Status == "failed" {
			stage.Status = tmFail
			stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "job lỗi tại bước này")
		} else if job.Status == "running" {
			stage.Status = tmWorking
			stage.Detail = "đang chạy"
		}
		return stage
	}
	if job.Status == "done" {
		stage.Detail = "job cũ; chưa có bằng chứng cho chặng này"
	} else if job.Status == "failed" {
		stage.Detail = "job lỗi trước khi chặng này bắt đầu"
	}
	return stage
}
