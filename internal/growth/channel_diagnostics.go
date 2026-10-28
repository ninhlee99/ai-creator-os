package growth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ChannelDiagnostic represents one diagnostic analysis of video performance.
type ChannelDiagnostic struct {
	ID              int64  `json:"id"`
	AccountID       int64  `json:"account_id"`
	FlowType        string `json:"flow_type"`    // "affiliate" | "viral"
	TargetID        string `json:"target_id"`    // job_id or platform_video_id
	FunnelStage     string `json:"funnel_stage"` // "unknown" until evidence includes provenance, period, and baseline.
	Diagnosis       string `json:"diagnosis"`
	MetricsJSON     string `json:"metrics_json"`
	SuggestedAction string `json:"suggested_action"`
	AutoTuned       bool   `json:"auto_tuned"`
	CreatedAt       string `json:"created_at"`
}

// TuningHistory is a persisted setting change record. It is not proof of an applied change.
type TuningHistory struct {
	ID           int64   `json:"id"`
	AccountID    int64   `json:"account_id"`
	ParamKey     string  `json:"param_key"`
	OldValue     string  `json:"old_value"`
	NewValue     string  `json:"new_value"`
	ChangePct    float64 `json:"change_pct"`
	Reason       string  `json:"reason"`
	Applied      bool    `json:"applied"`
	EvidenceJSON string  `json:"evidence_json"`
	AppliedAt    string  `json:"applied_at"`
	RolledBack   bool    `json:"rolled_back"`
	RolledBackAt string  `json:"rolled_back_at,omitempty"`
}

// AffiliateMetricsInput captures facts observed from video views, clicks, and orders.
type AffiliateMetricsInput struct {
	Views              *int64  `json:"views"`
	Clicks             *int64  `json:"clicks"`
	Orders             *int64  `json:"orders"`
	ApprovedCommission float64 `json:"approved_commission"`
	CanceledOrders     *int64  `json:"canceled_orders"`
	ProductPrice       float64 `json:"product_price"`
	Currency           string  `json:"currency"`
	CommissionRate     float64 `json:"commission_rate"`
}

// ViralMetricsInput captures facts observed from TikTok display and video analytics.
type ViralMetricsInput struct {
	Views                 *int64   `json:"views"`
	WatchTimeSec          *float64 `json:"watch_time_sec"`
	DurationSec           *float64 `json:"duration_sec"`
	ThreeSecRetentionRate *float64 `json:"three_sec_retention_rate"` // 0..1
	CompletionRate        *float64 `json:"completion_rate"`          // 0..1
	Likes                 *int64   `json:"likes"`
	Comments              *int64   `json:"comments"`
	Shares                *int64   `json:"shares"`
	FollowersGained       *int64   `json:"followers_gained"`
}

// TuningEvidence binds a configuration change to source-addressable,
// time-windowed evidence and a same-account baseline.
type TuningEvidence struct {
	Source      string   `json:"source"`
	EvidenceIDs []string `json:"evidence_ids"`
	WindowStart string   `json:"window_start"`
	WindowEnd   string   `json:"window_end"`
	Baseline    float64  `json:"baseline"`
	Observed    float64  `json:"observed"`
	SampleSize  int      `json:"sample_size"`
	Confidence  float64  `json:"confidence"`
	Explanation string   `json:"explanation"`
}

type tunableParameter struct {
	min, max float64
	integer  bool
	get      func(Config) float64
	set      func(*Config, float64)
}

var tunableGrowthParameters = map[string]tunableParameter{
	"growth.kill_min_videos":       {1, 100, true, func(c Config) float64 { return float64(c.KillMinVideos) }, func(c *Config, v float64) { c.KillMinVideos = int(math.Round(v)) }},
	"growth.kill_min_days":         {1, 180, true, func(c Config) float64 { return float64(c.KillMinDays) }, func(c *Config, v float64) { c.KillMinDays = int(math.Round(v)) }},
	"growth.kill_completion_floor": {0.05, 0.9, false, func(c Config) float64 { return c.KillCompletionFloor }, func(c *Config, v float64) { c.KillCompletionFloor = v }},
	"growth.kill_proxy_floor":      {0.001, 0.3, false, func(c Config) float64 { return c.KillProxyFloor }, func(c *Config, v float64) { c.KillProxyFloor = v }},
	"growth.double_down_factor":    {1, 20, false, func(c Config) float64 { return c.DoubleDownFactor }, func(c *Config, v float64) { c.DoubleDownFactor = v }},
	"growth.breakout_factor":       {1, 50, false, func(c Config) float64 { return c.BreakoutFactor }, func(c *Config, v float64) { c.BreakoutFactor = v }},
	"growth.penalty_views_drop":    {0.1, 0.95, false, func(c Config) float64 { return c.PenaltyViewsDrop }, func(c *Config, v float64) { c.PenaltyViewsDrop = v }},
	"growth.stalled_days":          {1, 180, true, func(c Config) float64 { return float64(c.StalledDays) }, func(c *Config, v float64) { c.StalledDays = int(math.Round(v)) }},
	"growth.stalled_growth_max":    {0.001, 0.2, false, func(c Config) float64 { return c.StalledGrowthMax }, func(c *Config, v float64) { c.StalledGrowthMax = v }},
}

// DiagnoseAffiliateVideo refuses causal classification until evidence provenance,
// measurement window, and account-matched baseline are part of the input contract.
func DiagnoseAffiliateVideo(accountID int64, targetID string, in AffiliateMetricsInput) ChannelDiagnostic {
	diag := ChannelDiagnostic{
		AccountID: accountID,
		FlowType:  "affiliate",
		TargetID:  targetID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	bytes, _ := json.Marshal(in)
	diag.MetricsJSON = string(bytes)

	diag.FunnelStage = "unknown"
	diag.Diagnosis = "Chưa đủ bằng chứng để chẩn đoán phễu affiliate: cần nguồn và ID attribution, kỳ đo đồng nhất, trạng thái đơn/hoa hồng, cùng baseline của kênh. Các metric hiện có (nếu có) chỉ là số liệu quan sát, không xác định nguyên nhân."
	diag.SuggestedAction = "Không tự đổi giá, lọc sản phẩm hoặc kịch bản. Bổ sung provenance, measurement window và baseline cùng kênh; sau đó hiển thị so sánh kèm cỡ mẫu để người dùng quyết định thử nghiệm."
	return diag
}

// DiagnoseViralVideo does not infer unavailable retention or follower metrics.
func DiagnoseViralVideo(accountID int64, targetID string, in ViralMetricsInput) ChannelDiagnostic {
	diag := ChannelDiagnostic{
		AccountID: accountID,
		FlowType:  "viral",
		TargetID:  targetID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	bytes, _ := json.Marshal(in)
	diag.MetricsJSON = string(bytes)

	diag.FunnelStage = "unknown"
	diag.Diagnosis = "Chưa đủ bằng chứng để chẩn đoán retention, pacing hoặc follower growth. TikTok Display API hiện chỉ cung cấp một số public counters; retention/watch-time/follows-per-video chưa có nguồn xác nhận. Counters không chứng minh nguyên nhân hay mức độ lan truyền."
	diag.SuggestedAction = "Không tự sửa hook/pacing hoặc kết luận video tốt/xấu. Đồng bộ metric có provider, account/video ID, timestamp, measurement window và baseline cùng kênh; giữ trường không được API trả là unknown."
	return diag
}

// RecordDiagnostic stores a diagnostic result in SQLite.
func (s *Store) RecordDiagnostic(d ChannelDiagnostic) (int64, error) {
	// This schema has no evidence provenance/window/baseline fields. Until that
	// contract exists, caller-supplied classifications cannot be trusted.
	d.FunnelStage = "unknown"
	d.AutoTuned = false
	d.Diagnosis = "Chưa đủ dữ liệu để xác minh chẩn đoán; cần nguồn, kỳ đo và baseline cùng kênh."
	d.SuggestedAction = "Không tự thay đổi cấu hình. Bổ sung dữ liệu có nguồn/kỳ đo/baseline rồi đánh giá lại."
	if strings.TrimSpace(d.MetricsJSON) == "" {
		d.MetricsJSON = "{}"
	}
	if strings.TrimSpace(d.CreatedAt) == "" {
		d.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
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
		// No row can be verified while schema lacks provenance/window/baseline.
		// Preserve raw metrics but never expose stored causal claims as verified.
		d.FunnelStage = "unknown"
		d.Diagnosis = "Bản ghi chưa có provenance, kỳ đo hoặc baseline; kết luận chưa được xác minh."
		d.SuggestedAction = "Không áp dụng hành động tự động. Đồng bộ dữ liệu có nguồn/kỳ đo/baseline trước khi đánh giá lại."
		d.AutoTuned = false
		out = append(out, d)
	}
	return out, rows.Err()
}

// MaxAdjustmentPercent bounds each account-scoped adjustment.
const MaxAdjustmentPercent = 15.0

func (s *Store) configWithAccountTuning(accountID int64, cfg Config) (Config, error) {
	var raw string
	if err := s.db.QueryRow(`SELECT tuning_json FROM growth_profiles WHERE account_id=?`, accountID).Scan(&raw); err != nil {
		return cfg, fmt.Errorf("đọc tuning growth của account %d: %w", accountID, err)
	}
	overrides := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return cfg, fmt.Errorf("tuning growth của account %d không hợp lệ: %w", accountID, err)
	}
	for key, value := range overrides {
		parameter, ok := tunableGrowthParameters[key]
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < parameter.min || value > parameter.max {
			return cfg, fmt.Errorf("tuning growth không hợp lệ cho %q", key)
		}
		parameter.set(&cfg, value)
	}
	return cfg, nil
}

// ConfigForAccount returns base config with validated account-scoped overrides.
func (s *Store) ConfigForAccount(accountID int64, cfg Config) (Config, error) {
	return s.configWithAccountTuning(accountID, cfg)
}

func validateTuningEvidence(e TuningEvidence) error {
	if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Explanation) == "" {
		return fmt.Errorf("evidence cần source và explanation")
	}
	start, err := time.Parse(time.RFC3339, e.WindowStart)
	if err != nil {
		return fmt.Errorf("window_start phải là RFC3339")
	}
	end, err := time.Parse(time.RFC3339, e.WindowEnd)
	if err != nil || !end.After(start) {
		return fmt.Errorf("window_end phải là RFC3339 sau window_start")
	}
	ids := map[string]struct{}{}
	for _, id := range e.EvidenceIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = struct{}{}
		}
	}
	if e.SampleSize < 3 || len(ids) < 3 {
		return fmt.Errorf("tuning cần ít nhất 3 evidence ID khác nhau và sample_size >= 3")
	}
	if math.IsNaN(e.Baseline) || math.IsInf(e.Baseline, 0) || e.Baseline <= 0 || math.IsNaN(e.Observed) || math.IsInf(e.Observed, 0) || e.Observed < 0 {
		return fmt.Errorf("baseline/observed không hợp lệ")
	}
	if math.IsNaN(e.Confidence) || math.IsInf(e.Confidence, 0) || e.Confidence < 0.8 || e.Confidence > 1 {
		return fmt.Errorf("confidence phải nằm trong [0.8, 1]")
	}
	return nil
}

func formatTuningValue(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

func accountTuningValue(accountID int64, key string, overrides map[string]float64) (float64, error) {
	parameter, ok := tunableGrowthParameters[key]
	if !ok {
		return 0, fmt.Errorf("param_key %q không được phép tune", key)
	}
	if accountID <= 0 {
		return 0, fmt.Errorf("account_id không hợp lệ")
	}
	cfg := DefaultConfig()
	if value, ok := overrides[key]; ok {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < parameter.min || value > parameter.max {
			return 0, fmt.Errorf("tuning hiện tại không hợp lệ cho %q", key)
		}
		parameter.set(&cfg, value)
	}
	return parameter.get(cfg), nil
}

func (s *Store) ApplyBoundedAdjustment(accountID int64, paramKey string, oldVal, targetVal float64, reason string, evidence TuningEvidence) (*TuningHistory, error) {
	paramKey = strings.TrimSpace(paramKey)
	parameter, ok := tunableGrowthParameters[paramKey]
	if !ok {
		return nil, fmt.Errorf("param_key %q không được phép tune", paramKey)
	}
	if math.IsNaN(oldVal) || math.IsInf(oldVal, 0) || oldVal <= 0 || math.IsNaN(targetVal) || math.IsInf(targetVal, 0) || targetVal <= 0 {
		return nil, fmt.Errorf("giá trị cấu hình phải hữu hạn và lớn hơn 0")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("reason không được để trống")
	}
	if err := validateTuningEvidence(evidence); err != nil {
		return nil, err
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("marshal tuning evidence: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRow(`SELECT tuning_json FROM growth_profiles WHERE account_id=?`, accountID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("account %d chưa có hồ sơ growth: %w", accountID, err)
	}
	overrides := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return nil, fmt.Errorf("tuning growth không hợp lệ: %w", err)
	}
	for key, value := range overrides {
		configured, exists := tunableGrowthParameters[key]
		if !exists || math.IsNaN(value) || math.IsInf(value, 0) || value < configured.min || value > configured.max {
			return nil, fmt.Errorf("tuning growth không hợp lệ cho %q", key)
		}
	}
	effectiveOld, err := accountTuningValue(accountID, paramKey, overrides)
	if err != nil {
		return nil, err
	}
	if math.Abs(effectiveOld-oldVal) > 1e-9 {
		return nil, fmt.Errorf("setting đã đổi: giá trị hiện tại %s khác old_value %s", formatTuningValue(effectiveOld), formatTuningValue(oldVal))
	}
	changePct := (targetVal - effectiveOld) / effectiveOld * 100
	if changePct > MaxAdjustmentPercent {
		changePct = MaxAdjustmentPercent
	} else if changePct < -MaxAdjustmentPercent {
		changePct = -MaxAdjustmentPercent
	}
	newVal := effectiveOld * (1 + changePct/100)
	newVal = math.Max(parameter.min, math.Min(parameter.max, newVal))
	if parameter.integer {
		newVal = math.Round(newVal)
	}
	if math.Abs(newVal-effectiveOld) < 1e-9 {
		return nil, fmt.Errorf("điều chỉnh không làm thay đổi setting")
	}
	changePct = (newVal - effectiveOld) / effectiveOld * 100
	overrides[paramKey] = newVal
	encoded, err := json.Marshal(overrides)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE growth_profiles SET tuning_json=?, updated_at=? WHERE account_id=?`, string(encoded), nowStr(), accountID); err != nil {
		return nil, err
	}
	oldString, newString := formatTuningValue(effectiveOld), formatTuningValue(newVal)
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT INTO settings_tuning_history
		(account_id,param_key,old_value,new_value,change_pct,reason,applied,evidence_json,applied_at,rolled_back,rolled_back_at)
		VALUES(?,?,?,?,?,?,1,?,?,0,'')`, accountID, paramKey, oldString, newString, changePct, strings.TrimSpace(reason), string(evidenceJSON), now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &TuningHistory{ID: id, AccountID: accountID, ParamKey: paramKey, OldValue: oldString, NewValue: newString,
		ChangePct: changePct, Reason: strings.TrimSpace(reason), Applied: true, EvidenceJSON: string(evidenceJSON), AppliedAt: now}, nil
}

// RollbackSetting restores the account override and marks its audit row in one
// transaction. Refuse rollback if a newer/manual change superseded it.
func (s *Store) RollbackSetting(accountID int64, historyID int64) (*TuningHistory, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var h TuningHistory
	var applied, rolledBack int
	if err := tx.QueryRow(`SELECT id,account_id,param_key,old_value,new_value,change_pct,reason,evidence_json,applied_at,applied,rolled_back,rolled_back_at
		FROM settings_tuning_history WHERE id=? AND account_id=?`, historyID, accountID).Scan(
		&h.ID, &h.AccountID, &h.ParamKey, &h.OldValue, &h.NewValue, &h.ChangePct, &h.Reason, &h.EvidenceJSON, &h.AppliedAt, &applied, &rolledBack, &h.RolledBackAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("không tìm thấy bản ghi cấu hình #%d", historyID)
		}
		return nil, err
	}
	if applied != 1 {
		return nil, fmt.Errorf("bản ghi #%d là lịch sử legacy/chưa áp dụng, không thể hoàn tác", historyID)
	}
	if rolledBack == 1 {
		return nil, fmt.Errorf("bản ghi #%d đã được hoàn tác", historyID)
	}
	parameter, ok := tunableGrowthParameters[h.ParamKey]
	if !ok {
		return nil, fmt.Errorf("param_key %q không còn được phép tune", h.ParamKey)
	}
	var raw string
	if err := tx.QueryRow(`SELECT tuning_json FROM growth_profiles WHERE account_id=?`, accountID).Scan(&raw); err != nil {
		return nil, err
	}
	overrides := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return nil, err
	}
	current, err := accountTuningValue(accountID, h.ParamKey, overrides)
	if err != nil {
		return nil, err
	}
	newVal, err := strconv.ParseFloat(h.NewValue, 64)
	if err != nil || math.Abs(current-newVal) > 1e-9 {
		return nil, fmt.Errorf("setting %q đã thay đổi sau bản ghi #%d; không thể rollback an toàn", h.ParamKey, historyID)
	}
	oldVal, err := strconv.ParseFloat(h.OldValue, 64)
	if err != nil || oldVal < parameter.min || oldVal > parameter.max {
		return nil, fmt.Errorf("old_value của bản ghi #%d không hợp lệ", historyID)
	}
	if math.Abs(oldVal-parameter.get(DefaultConfig())) < 1e-9 {
		delete(overrides, h.ParamKey)
	} else {
		overrides[h.ParamKey] = oldVal
	}
	encoded, err := json.Marshal(overrides)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE growth_profiles SET tuning_json=?, updated_at=? WHERE account_id=?`, string(encoded), nowStr(), accountID); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.Exec(`UPDATE settings_tuning_history SET rolled_back=1,rolled_back_at=? WHERE id=? AND account_id=? AND applied=1 AND rolled_back=0`, now, historyID, accountID)
	if err != nil {
		return nil, err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return nil, err
	} else if changed != 1 {
		return nil, fmt.Errorf("bản ghi #%d đã được hoàn tác đồng thời", historyID)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	h.Applied = true
	h.RolledBack = true
	h.RolledBackAt = now
	return &h, nil
}

// ListTuningHistory returns settings tuning history for an account.
func (s *Store) ListTuningHistory(accountID int64, limit int) ([]TuningHistory, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT id, account_id, param_key, old_value, new_value, change_pct, reason,
		evidence_json, applied_at, applied, rolled_back, rolled_back_at
		FROM settings_tuning_history WHERE account_id=? ORDER BY id DESC LIMIT ?`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TuningHistory
	for rows.Next() {
		var h TuningHistory
		var appliedInt, rolledBackInt int
		if err := rows.Scan(&h.ID, &h.AccountID, &h.ParamKey, &h.OldValue, &h.NewValue, &h.ChangePct, &h.Reason,
			&h.EvidenceJSON, &h.AppliedAt, &appliedInt, &rolledBackInt, &h.RolledBackAt); err != nil {
			return nil, err
		}
		h.Applied = appliedInt == 1
		h.RolledBack = rolledBackInt == 1
		out = append(out, h)
	}
	return out, rows.Err()
}
