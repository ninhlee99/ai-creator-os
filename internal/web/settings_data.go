package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/backup"
	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) atSettingStr(key, def string) string {
	if s.Ledger == nil {
		return def
	}
	if v, ok, err := s.Ledger.GetSetting(key); err == nil && ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

// settingsData builds the full settings context once; each sub-page picks
// the keys it needs (R2-W3: một trang = một mối quan tâm).
func (s *Server) settingsData(r *http.Request) map[string]any {
	envs := make([]envRow, 0, len(envNames))
	for _, e := range envNames {
		v, ok := s.effectiveEnv(e)
		lg := envLabels[e]
		row := envRow{Name: e, Ok: ok, Help: envHelp[e], Label: lg[0], Group: lg[1]}
		if row.Label == "" {
			row.Label = e
		}
		row.NewGroup = len(envs) == 0 || envs[len(envs)-1].Group != row.Group
		if ok {
			row.Masked = maskSecret(v)
		}
		if s.Ledger != nil {
			if _, inDB, err := s.Ledger.GetSetting(envSettingKey(e)); err == nil && inDB {
				row.FromDB = true
			}
		}
		envs = append(envs, row)
	}
	// Chi phí API (khối trong "Trạng thái hệ thống") — fail-soft: lỗi truy
	// vấn chỉ ghi log, không làm sập trang Cài đặt.
	usage, uerr := s.queryUsage()
	if uerr != nil {
		log.Printf("web: settings usage: %v", uerr)
	}
	var spend float64
	if s.Ledger != nil {
		if v, err := s.Ledger.DailySpendUSD(); err == nil {
			spend = v
		}
	}
	webAIViews, webAIConnected, webAIStudioReady := s.webAIPlatformViews()
	return map[string]any{
		"EnvStatus":              envs,
		"TikTokRadarConfigured": studio.TikTokRadarSourceConfigured(),
		"WebAIPlatforms":   webAIViews,
		"WebAIConnected":   webAIConnected,
		"WebAIStudioReady": webAIStudioReady,
		"EnvSaved":         r.URL.Query().Get("envsaved"),
		"EnvErr":           r.URL.Query().Get("enverr"), // Đợt AA: lỗi validate biến
		"Ok":               r.URL.Query().Get("ok"),     // Đợt AA: toast chung trang system
		"Err":              r.URL.Query().Get("err"),
		"DbPath":           s.Cfg.DatabasePath,
		"Usage":            usage,
		"Spend":            spend,
		"TTSChain":         s.loadChain("tts"),
		"LLMChain":         s.loadChain("llm"),
		"TTSKeys":          s.keyRingStatuses("tts", "gemini"),
		"LLMKeys":          s.keyRingStatuses("llm", "gemini"),
		// R2-W4: persisted master switch + daily API budget, editable in
		// Settings · Hệ thống.
		"MasterOn":   automation.MasterOn(s.settings()),
		"APIBudget":  automation.APIBudgetUSD(s.settings(), s.Cfg.DailyAPIBudgetUSD),
		"BudgetFrom": s.apiBudgetFrom(),
		// Accesstrade (Đợt B): key + trạng thái kết nối cho tab Cài đặt · Accesstrade.
		"ATKeySet":    s.atKeySet(),
		"ATMasked":    s.atMasked(),
		"ATCampaigns": s.atCachedCount(),
		"ATStoreOK":   s.AT != nil,
		// Accesstrade automation (Đợt G): interval tick cho tab Cài đặt ·
		// Accesstrade (công tắc on/off nằm ở trang Affiliate, POST /at/settings).
		"ATHunterHours": s.atSettingInt(automation.KeyATHunterIntervalHrs, 24),
		// Đợt AK: bật/tắt tick AT gộp về đây (trước nằm rời ở trang Affiliate).
		"ATAuto": map[string]any{
			"HunterOn":      s.atSettingOn(automation.KeyATHunterEnabled, true),
			"OrderSyncOn":   s.atSettingOn(automation.KeyATOrderSyncEnabled, true),
			"CampaignChkOn": s.atSettingOn(automation.KeyATCampaignCheckOn, true),
			"HunterVideos":  s.atSettingInt(automation.KeyATHunterVideos, 3),
		},
		"ATOrderSyncMins": s.atSettingInt(automation.KeyATOrderSyncIntervalM, 30),
		// R2-W7: legacy /api/* switch (default OFF), app version, runtime
		// probes, pending-restore flag — all shown in Settings · Hệ thống.
		"APIEnabled":     automation.APIEnabled(s.settings()),
		"Version":        s.Cfg.Version,
		"RuntimeTools":   probeRuntimeTools(),
		"AICapabilities": s.aiCapabilityViews(),
		"RestorePending": backup.PendingRestore(s.DataDir),
		"DataDir":        s.DataDir,
		// Đợt R+S: heartbeat các tick + trạng thái ngắt mạch quota.
		"TickBeats":    s.tickBeatViews(),
		"QuotaCircuit": s.quotaCircuitView(),
		// Đợt L: % đĩa đã dùng cho thẻ Hệ thống (disk guard tự dọn khi đầy).
		"DiskUsage": s.diskUsageView(),
		// Đợt M2: lần sao lưu tự động gần nhất.
		"BackupLastOK": automation.BackupLastOK(s.settings()),
		// Đợt V: thư mục offsite cho bản sao lưu.
		"BackupOffsiteDir": s.atSettingStr(automation.KeyBackupOffsiteDir, ""),
		// Đợt O2: cấu hình Telegram (token không bao giờ hiện lại ra UI).
		"NotifyOn":      s.atSettingOn(automation.KeyNotifyTelegramOn, false),
		"NotifyChat":    s.atSettingStr(automation.KeyNotifyTelegramChat, ""),
		"NotifyWebhook": s.atSettingStr(automation.KeyNotifyWebhookURL, ""),
		"NotifyHasBot": func() bool {
			v, ok, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramBot)
			return ok && strings.TrimSpace(v) != ""
		}(),
		"GeminiModels":            s.geminiModelsView(),
		"GeminiFreeTierConfirmed": s.GeminiFreeTierConfirmed(),
	}
}

// diskUsageView trả về % đĩa đã dùng tại DataDir cho thẻ Hệ thống.
func (s *Server) diskUsageView() map[string]any {
	pct, err := automation.DiskUsagePct(s.DataDir)
	if err != nil || s.DataDir == "" {
		return map[string]any{"Pct": -1, "Badge": "badge-draft"}
	}
	return map[string]any{"Pct": pct, "Badge": diskBadge(pct)}
}

// diskBadge: đỏ từ 85% (ngưỡng tự dọn mặc định của self-heal), cam từ 75%.
// Đợt AK: trước đây ≥90% dùng badge-no (xám, nghĩa "tắt") — đĩa sắp đầy mà
// trông như bình thường.
func diskBadge(pct int) string {
	switch {
	case pct >= 85:
		return "badge-err"
	case pct >= 75:
		return "badge-warn"
	}
	return "badge-ok"
}

// apiBudgetFrom reports where the API budget value comes from (honest
// label for the UI): "ui" when a persisted override exists, "env" when
// it falls back to DAILY_API_BUDGET_USD.
func (s *Server) apiBudgetFrom() string {
	if _, ok := s.settings().Get(automation.KeyAPIBudgetUSD); ok {
		return "ui"
	}
	return "env"
}

// GeminiKeys tập hợp tất cả các API key của Google Gemini từ cấu hình, DB và biến môi trường.
func (s *Server) GeminiKeys() []string {
	var keys []string
	seen := make(map[string]bool)
	add := func(list []string) {
		for _, k := range list {
			k = strings.TrimSpace(k)
			if k != "" && !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	add(s.storedKeys("llm", "gemini"))
	add(s.storedKeys("tts", "gemini"))
	add(s.Cfg.GeminiAPIKeys)
	if env, ok := s.effectiveEnv("GEMINI_API_KEYS"); ok {
		add(strings.Split(env, ","))
	}
	if env, ok := s.effectiveEnv("GEMINI_API_KEY"); ok {
		add([]string{env})
	}
	if env, ok := s.effectiveEnv("TTS_API_KEY"); ok {
		add([]string{env})
	}
	return keys
}

// geminiModelsView chuẩn bị dữ liệu hiển thị khối cập nhật model Gemini ở Cài đặt · Hệ thống.
func (s *Server) geminiModelsView() map[string]any {
	report := gmodels.LastReport()
	lastCheckStr := "Chưa quét. Bấm nút bên phải để kiểm tra & cập nhật model mới nhất."
	if report != nil {
		if report.Success {
			lastCheckStr = fmt.Sprintf("Lúc %s (Thành công, %d models khả dụng)",
				report.CheckedAt.Format("15:04 02/01/2006"), report.TotalModels)
		} else {
			lastCheckStr = fmt.Sprintf("Lúc %s (Lỗi: %s)",
				report.CheckedAt.Format("15:04 02/01/2006"), report.Error)
		}
	}

	modelItem := func(k gmodels.Kind, label, desc string) map[string]any {
		cur := gmodels.Resolve(k)
		def := gmodels.Default(k)
		envVal, hasEnv := s.effectiveEnv(gmodels.EnvName(k))
		source := "Mặc định hệ thống"
		badge := "badge-info"
		if hasEnv && envVal != "" {
			source = "Ghi đè thủ công (" + gmodels.EnvName(k) + ")"
			badge = "badge-warn"
		} else if cur != def {
			source = "Đã cập nhật từ Google API"
			badge = "badge-ok"
		}
		return map[string]any{
			"Kind":    label,
			"Desc":    desc,
			"Model":   cur,
			"Default": def,
			"Source":  source,
			"Badge":   badge,
			"EnvVar":  gmodels.EnvName(k),
		}
	}

	return map[string]any{
		"LastCheck": lastCheckStr,
		"Status":    "🛠️ Cập nhật thủ công theo yêu cầu (Bấm nút để quét & nâng cấp toàn bộ model)",
		"Report":    report,
		"Items": []map[string]any{
			modelItem(gmodels.Text, "Kịch bản video (LLM)", "Viết hook, nội dung và caption cho video TikTok."),
			modelItem(gmodels.TTS, "Lời đọc video (TTS)", "Tạo giọng đọc cho video affiliate và viral."),
		},
	}
}

// handleModelsUpdate kích hoạt kiểm tra và nâng cấp model Gemini tức thì từ giao diện web.
func (s *Server) handleModelsUpdate(w http.ResponseWriter, r *http.Request) {
	keys := s.GeminiKeys()
	if len(keys) == 0 {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Chưa có API key Gemini để kiểm tra cập nhật model."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rep, err := gmodels.CheckAndUpdate(ctx, nil, "", keys, s.DataDir)
	if err != nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Kiểm tra model thất bại: "+err.Error()))
		return
	}
	msg := fmt.Sprintf("✓ Đã kiểm tra Gemini API (%d models khả dụng). Kịch bản: %s | Giọng đọc: %s",
		rep.TotalModels, gmodels.Resolve(gmodels.Text), gmodels.Resolve(gmodels.TTS))
	if rep.Updated && len(rep.Changes) > 0 {
		var activeChanges []string
		for _, label := range []string{"viết chữ", "giọng đọc"} {
			if change, ok := rep.Changes[label]; ok {
				activeChanges = append(activeChanges, label+": "+change)
			}
		}
		if len(activeChanges) > 0 {
			msg = "🚀 Đã nâng cấp model Gemini đang bật: " + strings.Join(activeChanges, " · ") + "."
		} else {
			msg = fmt.Sprintf("Đã đồng bộ danh mục Gemini (%d models).", rep.TotalModels)
		}
	}
	_ = s.Ledger.Decide("human", "model_update_check", nil, msg, map[string]any{"updated": rep.Updated, "total": rep.TotalModels})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(msg))
}
