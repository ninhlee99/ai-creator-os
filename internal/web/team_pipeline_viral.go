package web

import (
	"strings"
)

func (s *Server) viralTeam() pipeTeam {
	latest := s.latestJobByKind("viral")
	stages := []pipeStage{
		{Key: "input", Label: "Nguồn & kênh", Icon: "📥", Link: "/viral", RoleDesc: "Xác nhận video dọc, ảnh nhân vật và kênh đã chọn"},
		{Key: "identity", Label: "Thay toàn nhân vật", Icon: "👤", Link: "/viral", RoleDesc: "Edit toàn thân trên video nguồn; face-only không đạt yêu cầu"},
		{Key: "media", Label: "Kiểm tra video", Icon: "🎞️", Link: "/studio/jobs?tab=running", RoleDesc: "Ghép các đoạn edit, khôi phục audio nguồn, kiểm tra thời lượng và khung 9:16"},
		{Key: "qc", Label: "QC thị giác", Icon: "🛡️", Link: "/studio/jobs?tab=running", RoleDesc: "So sánh frame nguồn/đầu ra: nhân vật, cảnh, hành động và camera"},
		{Key: "output", Label: "Đầu ra", Icon: "📤", Link: "/studio/jobs?tab=done", RoleDesc: "Job hoàn tất sau khi kiểm tra media và visual QC đạt"},
	}
	if latest == nil {
		for i := range stages {
			stages[i].Status = tmIdle
			stages[i].Detail = "chưa có job"
		}
	} else {
		stages[0] = markedPipelineStage(stages[0], latest, "SOURCE_VALIDATION_START:", "SOURCE_VALIDATION_PASS:")
		stages[1] = markedPipelineStage(stages[1], latest, "CHARACTER_REPLACEMENT_START:", "CHARACTER_REPLACEMENT_PASS:")
		stages[2] = markedPipelineStage(stages[2], latest, "OUTPUT_VALIDATION_START:", "OUTPUT_VALIDATION_PASS:")
		stages[3] = visualQCStage(stages[3].Label, stages[3].Icon, latest, stages[3].Link, stages[3].RoleDesc)
		if latest.Status == "done" && latest.Output != "" && latest.QCStatus == "pass" {
			stages[4].Status = tmOK
			stages[4].Detail = "video đầu ra đã qua visual QC"
		} else if latest.Status == "done" && latest.Output != "" {
			stages[4].Status = tmWaiting
			stages[4].Detail = "render xong; chưa có bằng chứng visual QC"
		} else if latest.Status == "failed" {
			stages[4].Status = tmFail
			stages[4].Detail = "job lỗi; không có đầu ra hoàn chỉnh"
		} else {
			stages[4].Status = tmWaiting
			stages[4].Detail = "chờ các bước kiểm tra"
		}
	}
	return pipeTeam{
		Key:         "viral",
		Name:        "Video Viral",
		Icon:        "🔥",
		Description: "Các chặng lấy trạng thái từ log job; không hiển thị agent, voice hoặc đăng kênh nếu không có worker tương ứng.",
		ActionLink:  "/viral",
		ActionLabel: "🔥 Xưởng Video Viral",
		Stages:      stages,
	}
}

func markedPipelineStage(stage pipeStage, job *studioJobLite, startMarker, passMarker string) pipeStage {
	failMarker := strings.TrimSuffix(passMarker, "PASS:") + "FAIL:"
	state, _ := pipelineMarkerState(job.Log, startMarker, passMarker, failMarker)
	if state == "pass" {
		stage.Status = tmOK
		stage.Detail = visualQCLogDetail(job.Log, passMarker, "đã hoàn tất")
		return stage
	}
	if state == "fail" {
		stage.Status = tmFail
		stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "job lỗi tại bước này")
		return stage
	}
	if state == "running" {
		switch job.Status {
		case "running":
			stage.Status = tmWorking
			stage.Detail = "đang chạy"
		case "failed":
			stage.Status = tmFail
			stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "job lỗi tại bước này")
		default:
			stage.Status = tmWaiting
			stage.Detail = "đang chờ"
		}
		return stage
	}
	if job.Status == "failed" && !strings.Contains(job.Log, "SOURCE_VALIDATION_PASS:") {
		stage.Status = tmFail
		stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "đầu vào không hợp lệ")
		return stage
	}
	stage.Status = tmWaiting
	stage.Detail = "chờ bước trước"
	return stage
}
