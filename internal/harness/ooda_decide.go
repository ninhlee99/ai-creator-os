package harness

import (
	"context"
	"fmt"
	"strings"
)

func (e *OODAEngine) OrientAndDecide(ctx context.Context, obs OODAPerception, _ AutonomyLevel) (string, []OODADecisionTask) {
	var tasks []OODADecisionTask

	// Tạo phân tích tình huống
	var diag strings.Builder
	diag.WriteString(fmt.Sprintf("Hệ thống đang quan sát %d kênh. Sức khỏe dây chuyền: %s. ", obs.TotalChannels, obs.SystemHealth))
	videoEvidence := describeVideoProductionReadiness(obs.VideoProduction)
	diag.WriteString(videoEvidence)
	diag.WriteString("OODA chưa nối offer feed hoặc nguồn xu hướng; không thể kết luận có hay không có candidate sản phẩm/âm thanh. ")
	var channelEvidence strings.Builder
	var formatEvidence strings.Builder
	for _, ch := range obs.Channels {
		if ch.MetricsStatus == "available" {
			fmt.Fprintf(&channelEvidence, "@%s: metrics mới cho %d video trong 7 ngày, trung bình %.0f views/video, cập nhật %s. ",
				ch.Username, ch.RecentMetricVideos, ch.AvgViews, ch.MetricsUpdatedAt)
			diag.WriteString(fmt.Sprintf("@%s có metrics mới cho %d video trong 7 ngày; trung bình %.0f views/video. ",
				ch.Username, ch.RecentMetricVideos, ch.AvgViews))
		} else {
			status := ch.MetricsStatus
			if status == "" {
				status = "unknown"
			}
			fmt.Fprintf(&channelEvidence, "@%s: chưa có metrics video dùng được trong 7 ngày (trạng thái %s). ", ch.Username, status)
			diag.WriteString(fmt.Sprintf("@%s chưa có metrics video dùng được trong 7 ngày. ", ch.Username))
		}
		formatSummary := describeChannelFormatEvidence(ch)
		formatEvidence.WriteString(formatSummary)
		diag.WriteString(formatSummary)
	}

	if len(obs.Channels) == 0 {
		diag.WriteString("Chưa có kênh nào được cấu hình trong hệ thống.")
		return diag.String(), nil
	}

	// Do not turn aggregate channel metrics into a production task. A viral
	// decision needs a concrete source video; an affiliate decision needs a
	// verified product and an available motion-production path. Neither input
	// is part of this observation yet.
	if obs.HighROIProduct != "" && !obs.VideoProduction.AffiliateAutomaticReady {
		diag.WriteString("Có tên sản phẩm nhưng chưa đủ bằng chứng listing và footage để tạo job affiliate. ")
	}

	// Nếu LLM sẵn sàng, cho LLM trau chuốt và làm sắc nét chiến lược
	if e.llm != nil && e.llm.Healthy(ctx) {
		evidence := diag.String()
		metricsPromptEvidence := boundLLMEvidence(channelEvidence.String())
		formatPromptEvidence := boundLLMEvidence(formatEvidence.String())
		prompt := fmt.Sprintf(`Bạn là điều phối viên vận hành nội dung.
Chỉ tóm tắt dữ kiện và task đã có trong danh sách; không tự tạo task hoặc gợi ý hành động ngoài danh sách. Nêu rõ trường nào đang thiếu; không suy luận doanh thu, độ an toàn kênh, hiệu quả hay xu hướng nếu dữ liệu không chứng minh. Offer feed và nguồn xu hướng chưa nối vào OODA; chuỗi trống không có nghĩa là đã xác nhận không có candidate. Verdict format là bản lưu; chỉ tường thuật verdict hiện có, không tự tính lại. Nếu updated_at quá 7 ngày, ghi là dữ liệu cũ. Evidence có thể đã bị cắt theo giới hạn; không khái quát phần bị lược.
Khả năng production: %s
Dữ liệu: %d kênh, job hoạt động %d, job hoàn tất %d, candidate offer/trend được truyền vào OODA: %s.
Metrics theo kênh: %s
Verdict format đã lưu theo kênh: %s
Nhiệm vụ sơ bộ: %d. Tối đa 3 câu tiếng Việt.`,
			videoEvidence, obs.TotalChannels, obs.ActiveJobsCount, obs.DoneJobsCount, obs.HighROIProduct, metricsPromptEvidence, formatPromptEvidence, len(tasks))

		if resp, err := e.llm.Complete(ctx, "Báo cáo điều phối dữ kiện; không phóng đại năng lực.", prompt); err == nil && len(resp) > 20 {
			diag.Reset()
			diag.WriteString(evidence)
			diag.WriteString("Tóm tắt LLM: ")
			diag.WriteString(strings.TrimSpace(resp))
		}
	}

	return diag.String(), tasks
}

func boundLLMEvidence(value string) string {
	runes := []rune(value)
	if len(runes) <= maxLLMEvidenceRunes {
		return value
	}
	return string(runes[:maxLLMEvidenceRunes]) + "… [đã cắt theo giới hạn context]"
}

func describeChannelFormatEvidence(channel ChannelState) string {
	switch channel.FormatStatsStatus {
	case "available":
		if len(channel.FormatStats) == 0 {
			return fmt.Sprintf("@%s: trạng thái format available nhưng không có hàng dữ liệu. ", channel.Username)
		}
	case "missing":
		return fmt.Sprintf("@%s: chưa có verdict format lưu trong Growth. ", channel.Username)
	case "error":
		return fmt.Sprintf("@%s: lỗi đọc verdict format. ", channel.Username)
	default:
		return fmt.Sprintf("@%s: Growth store chưa được nối để đọc verdict format. ", channel.Username)
	}
	var summary strings.Builder
	for _, format := range channel.FormatStats {
		fmt.Fprintf(&summary, "@%s format=%q verdict=%s videos=%d median_views=%.0f avg_views=%.0f updated=%s",
			channel.Username, format.FormatID, format.Verdict, format.Videos, format.MedianViews, format.AvgViews, format.UpdatedAt)
		if format.VerdictReason != "" {
			fmt.Fprintf(&summary, " reason=%q", format.VerdictReason)
		}
		summary.WriteString(". ")
	}
	if channel.FormatStatsOmitted > 0 {
		fmt.Fprintf(&summary, "@%s còn %d format khác không đưa vào evidence rút gọn. ", channel.Username, channel.FormatStatsOmitted)
	}
	return summary.String()
}

// ExecuteCycle chạy một chu kỳ OODA; production action vẫn cần đầu vào/phê duyệt.
