package growth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ChannelDiagnostic represents one diagnostic analysis of video performance.
type ChannelDiagnostic struct {
	ID              int64          `json:"id"`
	AccountID       int64          `json:"account_id"`
	FlowType        string         `json:"flow_type"` // "affiliate" | "viral"
	TargetID        string         `json:"target_id"` // job_id or platform_video_id
	FunnelStage     string         `json:"funnel_stage"` // "hook", "retention", "click", "purchase", "payout", "healthy"
	Diagnosis       string         `json:"diagnosis"`
	MetricsJSON     string         `json:"metrics_json"`
	SuggestedAction string         `json:"suggested_action"`
	AutoTuned       bool           `json:"auto_tuned"`
	CreatedAt       string         `json:"created_at"`
}

// TuningHistory records an autonomous setting calibration with 1-click rollback support.
type TuningHistory struct {
	ID           int64   `json:"id"`
	AccountID    int64   `json:"account_id"`
	ParamKey     string  `json:"param_key"`
	OldValue     string  `json:"old_value"`
	NewValue     string  `json:"new_value"`
	ChangePct    float64 `json:"change_pct"`
	Reason       string  `json:"reason"`
	AppliedAt    string  `json:"applied_at"`
	RolledBack   bool    `json:"rolled_back"`
	RolledBackAt string  `json:"rolled_back_at,omitempty"`
}

// AffiliateMetricsInput captures facts observed from video views, clicks, and orders.
type AffiliateMetricsInput struct {
	Views              int64   `json:"views"`
	Clicks             int64   `json:"clicks"`
	Orders             int64   `json:"orders"`
	ApprovedCommission float64 `json:"approved_commission"`
	CanceledOrders     int64   `json:"canceled_orders"`
	ProductPrice       float64 `json:"product_price"`
	Currency           string  `json:"currency"`
	CommissionRate     float64 `json:"commission_rate"`
}

// ViralMetricsInput captures facts observed from TikTok display and video analytics.
type ViralMetricsInput struct {
	Views                int64   `json:"views"`
	WatchTimeSec         float64 `json:"watch_time_sec"`
	DurationSec          float64 `json:"duration_sec"`
	ThreeSecRetentionRate float64 `json:"three_sec_retention_rate"` // 0..1
	CompletionRate       float64 `json:"completion_rate"`          // 0..1
	Likes                int64   `json:"likes"`
	Comments             int64   `json:"comments"`
	Shares               int64   `json:"shares"`
	FollowersGained      int64   `json:"followers_gained"`
}

// DiagnoseAffiliateVideo analyzes the conversion funnel:
// Views -> Clicks -> Orders -> Payout, and evaluates youth price suitability.
func DiagnoseAffiliateVideo(accountID int64, targetID string, in AffiliateMetricsInput) ChannelDiagnostic {
	diag := ChannelDiagnostic{
		AccountID: accountID,
		FlowType:  "affiliate",
		TargetID:  targetID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	bytes, _ := json.Marshal(in)
	diag.MetricsJSON = string(bytes)

	if in.Views < 300 {
		diag.FunnelStage = "hook"
		diag.Diagnosis = "Video không lên xu hướng, lượt xem thấp (< 300 views). Mở đầu chưa kích thích tò mò hoặc âm thanh chưa đúng trend."
		diag.SuggestedAction = "Đổi sang âm thanh Radar có vận tốc tăng trưởng cao nhất 48h qua, rút ngắn shot mở đầu dưới 1.5s."
		return diag
	}

	ctr := 0.0
	if in.Views > 0 {
		ctr = float64(in.Clicks) / float64(in.Views)
	}
	if ctr < 0.01 && in.Views >= 500 {
		diag.FunnelStage = "click"
		diag.Diagnosis = fmt.Sprintf("Khán giả chỉ xem giải trí mà không bấm giỏ hàng (CTR = %.2f%% < 1.0%%). Sản phẩm xuất hiện mờ nhạt hoặc thiếu CTA rõ ràng.", ctr*100)
		diag.SuggestedAction = "Kịch bản mới bắt buộc nhân vật cầm/trải nghiệm sản phẩm từ giây thứ 2; thêm animation chỉ tay vào giỏ hàng ở giây 8-10."
		return diag
	}

	cvr := 0.0
	if in.Clicks > 0 {
		cvr = float64(in.Orders) / float64(in.Clicks)
	}
	if in.Clicks >= 20 && cvr < 0.02 {
		diag.FunnelStage = "purchase"
		currency := in.Currency
		if currency == "" {
			currency = "VND"
		}
		if in.ProductPrice > 350000 {
			diag.Diagnosis = fmt.Sprintf("Người dùng bấm xem nhưng không mua (CVR = %.2f%%). Giá sản phẩm (%.0f %s) quá cao so với túi tiền của giới trẻ/học sinh sinh viên.",
				cvr*100, in.ProductPrice, currency)
			diag.SuggestedAction = "Tự động hạ trần giá Radar affiliate (-15%), ưu tiên sản phẩm dưới 300k và shop có Freeship."
		} else {
			diag.Diagnosis = fmt.Sprintf("Người dùng bấm xem nhưng không mua (CVR = %.2f%%) dù giá bình dân. Có thể do phí vận chuyển cao hoặc uy tín shop chưa cao.", cvr*100)
			diag.SuggestedAction = "Điều chỉnh tiêu chuẩn lọc Radar: chỉ chọn shop có đánh giá >= 4.5 sao và nhãn Freeship Extra."
		}
		return diag
	}

	totalOrders := in.Orders + in.CanceledOrders
	if totalOrders >= 5 {
		cancelRate := float64(in.CanceledOrders) / float64(totalOrders)
		if cancelRate > 0.25 {
			diag.FunnelStage = "payout"
			diag.Diagnosis = fmt.Sprintf("Tỷ lệ hủy/hoàn đơn cao (%.1f%% > 25%%), làm thất thoát hoa hồng thực nhận.", cancelRate*100)
			diag.SuggestedAction = "Tạm dừng sản phẩm từ shop này trong 30 ngày; nâng sàn hoa hồng tối thiểu an toàn thêm +2%."
			return diag
		}
	}

	diag.FunnelStage = "healthy"
	diag.Diagnosis = "Phễu chuyển đổi affiliate hoạt động tốt, tỷ lệ xem và mua hàng đạt kỳ vọng."
	diag.SuggestedAction = "Nhân rộng kịch bản và góc quay sang các sản phẩm cùng ngành hàng."
	return diag
}

// DiagnoseViralVideo analyzes audience retention, pacing, and follower growth.
func DiagnoseViralVideo(accountID int64, targetID string, in ViralMetricsInput) ChannelDiagnostic {
	diag := ChannelDiagnostic{
		AccountID: accountID,
		FlowType:  "viral",
		TargetID:  targetID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	bytes, _ := json.Marshal(in)
	diag.MetricsJSON = string(bytes)

	if in.ThreeSecRetentionRate > 0 && in.ThreeSecRetentionRate < 0.60 {
		diag.FunnelStage = "hook"
		diag.Diagnosis = fmt.Sprintf("Rớt đài %.1f%% người xem ngay trong 3 giây đầu (tỷ lệ giữ chân 3s: %.1f%% < 60%%). Khung hình đầu quá tĩnh hoặc thiếu điểm nhấn thị giác.",
			(1.0-in.ThreeSecRetentionRate)*100, in.ThreeSecRetentionRate*100)
		diag.SuggestedAction = "Chuyển prompt dựng hình sang góc quay cận cảnh biểu cảm ngạc nhiên của nhân vật; chèn SFX Whoosh tại 0.2s."
		return diag
	}

	if in.CompletionRate > 0 && in.CompletionRate < 0.35 {
		diag.FunnelStage = "retention"
		diag.Diagnosis = fmt.Sprintf("Người xem rời đi ở giữa video (tỷ lệ xem hết: %.1f%% < 35%%). Nhịp dựng (pacing) bị chùng, thiếu cao trào bất ngờ.",
			in.CompletionRate*100)
		diag.SuggestedAction = "Rút ngắn thời lượng mỗi shot (< 2.5s/shot), chèn biến đổi camera (zoom in/out) mỗi 3 giây."
		return diag
	}

	if in.Views >= 1500 {
		followRate := float64(in.FollowersGained) / float64(in.Views)
		if followRate < 0.001 {
			diag.FunnelStage = "growth"
			diag.Diagnosis = fmt.Sprintf("Lượt xem đạt %d nhưng tỷ lệ follow thấp (%.3f%% < 0.1%%). Khán giả xem xong trôi tuột, thiếu liên kết thương hiệu cá nhân.",
				in.Views, followRate*100)
			diag.SuggestedAction = "Thêm câu chốt gợi mở thảo luận ở cuối video ('Bạn thuộc team nào?'), tăng sự đồng cảm của nhân vật."
			return diag
		}
	}

	diag.FunnelStage = "healthy"
	diag.Diagnosis = "Video đạt độ lan truyền và tỷ lệ giữ chân người xem tích cực."
	diag.SuggestedAction = "Giữ nguyên cấu trúc nhịp điệu và phong cách nhân vật cho các chủ đề tiếp theo."
	return diag
}

// RecordDiagnostic stores a diagnostic result in SQLite.
func (s *Store) RecordDiagnostic(d ChannelDiagnostic) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO channel_diagnostics
		(account_id, flow_type, target_id, funnel_stage, diagnosis, metrics_json, suggested_action, auto_tuned, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.AccountID, d.FlowType, d.TargetID, d.FunnelStage, d.Diagnosis, d.MetricsJSON, d.SuggestedAction, d.AutoTuned, d.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListDiagnostics returns recent diagnostics for an account.
func (s *Store) ListDiagnostics(accountID int64, limit int) ([]ChannelDiagnostic, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id, account_id, flow_type, target_id, funnel_stage, diagnosis,
		metrics_json, suggested_action, auto_tuned, created_at
		FROM channel_diagnostics WHERE account_id=? ORDER BY id DESC LIMIT ?`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChannelDiagnostic
	for rows.Next() {
		var d ChannelDiagnostic
		var autoTunedInt int
		if err := rows.Scan(&d.ID, &d.AccountID, &d.FlowType, &d.TargetID, &d.FunnelStage, &d.Diagnosis,
			&d.MetricsJSON, &d.SuggestedAction, &autoTunedInt, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.AutoTuned = autoTunedInt == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

// MaxAdjustmentPercent is the hard boundary for automated calibrations (+-15%).
const MaxAdjustmentPercent = 15.0

// ApplyBoundedAdjustment calibrates a parameter with safety limits and audit history.
func (s *Store) ApplyBoundedAdjustment(accountID int64, paramKey string, oldVal, targetVal float64, reason string) (*TuningHistory, error) {
	paramKey = strings.TrimSpace(paramKey)
	if paramKey == "" {
		return nil, fmt.Errorf("param_key không được để trống")
	}
	if oldVal <= 0 {
		oldVal = targetVal
	}

	changePct := ((targetVal - oldVal) / oldVal) * 100.0
	// Bounded Clamp: Max +- 15%
	clampedNewVal := targetVal
	if changePct > MaxAdjustmentPercent {
		changePct = MaxAdjustmentPercent
		clampedNewVal = oldVal * (1.0 + MaxAdjustmentPercent/100.0)
	} else if changePct < -MaxAdjustmentPercent {
		changePct = -MaxAdjustmentPercent
		clampedNewVal = oldVal * (1.0 - MaxAdjustmentPercent/100.0)
	}

	oldStr := fmt.Sprintf("%.2f", oldVal)
	newStr := fmt.Sprintf("%.2f", clampedNewVal)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	res, err := s.db.Exec(`INSERT INTO settings_tuning_history
		(account_id, param_key, old_value, new_value, change_pct, reason, applied_at, rolled_back, rolled_back_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, '')`,
		accountID, paramKey, oldStr, newStr, changePct, reason, nowStr)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &TuningHistory{
		ID:         id,
		AccountID:  accountID,
		ParamKey:   paramKey,
		OldValue:   oldStr,
		NewValue:   newStr,
		ChangePct:  changePct,
		Reason:     reason,
		AppliedAt:  nowStr,
		RolledBack: false,
	}, nil
}

// RollbackSetting restores the parameter to its previous value and marks history as rolled back.
func (s *Store) RollbackSetting(accountID int64, historyID int64) (*TuningHistory, error) {
	row := s.db.QueryRow(`SELECT id, account_id, param_key, old_value, new_value, change_pct, reason, applied_at, rolled_back
		FROM settings_tuning_history WHERE id=? AND account_id=?`, historyID, accountID)
	var h TuningHistory
	var rolledBackInt int
	if err := row.Scan(&h.ID, &h.AccountID, &h.ParamKey, &h.OldValue, &h.NewValue, &h.ChangePct, &h.Reason, &h.AppliedAt, &rolledBackInt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("không tìm thấy bản ghi cấu hình #%d", historyID)
		}
		return nil, err
	}
	if rolledBackInt == 1 {
		return nil, fmt.Errorf("bản ghi #%d đã được hoàn tác trước đó", historyID)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE settings_tuning_history SET rolled_back=1, rolled_back_at=? WHERE id=?`, nowStr, historyID); err != nil {
		return nil, err
	}
	h.RolledBack = true
	h.RolledBackAt = nowStr

	// Create reverse audit entry
	oldF, _ := strconv.ParseFloat(h.NewValue, 64)
	newF, _ := strconv.ParseFloat(h.OldValue, 64)
	revChange := 0.0
	if oldF > 0 {
		revChange = ((newF - oldF) / oldF) * 100.0
	}
	revReason := fmt.Sprintf("Hoàn tác cài đặt về phiên bản trước #%d: %s", historyID, h.Reason)
	_, _ = s.db.Exec(`INSERT INTO settings_tuning_history
		(account_id, param_key, old_value, new_value, change_pct, reason, applied_at, rolled_back, rolled_back_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, '')`,
		accountID, h.ParamKey, h.NewValue, h.OldValue, revChange, revReason, nowStr)

	return &h, nil
}

// ListTuningHistory returns settings tuning history for an account.
func (s *Store) ListTuningHistory(accountID int64, limit int) ([]TuningHistory, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT id, account_id, param_key, old_value, new_value, change_pct, reason,
		applied_at, rolled_back, rolled_back_at
		FROM settings_tuning_history WHERE account_id=? ORDER BY id DESC LIMIT ?`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TuningHistory
	for rows.Next() {
		var h TuningHistory
		var rolledBackInt int
		if err := rows.Scan(&h.ID, &h.AccountID, &h.ParamKey, &h.OldValue, &h.NewValue, &h.ChangePct, &h.Reason,
			&h.AppliedAt, &rolledBackInt, &h.RolledBackAt); err != nil {
			return nil, err
		}
		h.RolledBack = rolledBackInt == 1
		out = append(out, h)
	}
	return out, rows.Err()
}
