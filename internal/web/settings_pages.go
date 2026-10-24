package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// settingsPage renders one settings sub-page with its nav key and the
// requested data keys.
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request, page, key string, keys ...string) {
	data := s.settingsData(r)
	if page == "accesstrade" {
		var accounts any
		if s.Mgr != nil {
			accounts, _ = s.Mgr.List()
		}
		var accountID int64
		if s.Ledger != nil {
			if raw, ok, _ := s.Ledger.GetSetting(automation.KeyATHunterAccountID); ok {
				accountID, _ = strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			}
		}
		data["ATHunterAccounts"] = accounts
		data["ATHunterAccountID"] = accountID
	}
	ctx := s.ctx("SettingsPage", page)
	for _, k := range keys {
		ctx[k] = data[k]
	}
	s.render(w, key, ctx)
}

// handleSettingsIndex keeps the old entry point working: it redirects to
// the first sub-page.
func (s *Server) handleSettingsIndex(w http.ResponseWriter, r *http.Request) {
	seeOther(w, r, "/settings/system")
}

// handleAuthPasswordSet đặt/đổi mật khẩu dashboard (Đợt T).
func (s *Server) handleAuthPasswordSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse password")
		return
	}
	// Đợt AF: đã có mật khẩu thì phải nhập đúng mật khẩu hiện tại mới được
	// đổi — chống người khác đổi mật khẩu khóa chủ máy (máy dùng chung).
	if s.authEnabled() {
		cur := r.PostFormValue("current_password")
		stored, ok, _ := s.Ledger.GetSetting(KeyAuthPasswordHash)
		if !ok || !checkAuthPassword(cur, stored) {
			seeOther(w, r, "/settings/system?err="+url.QueryEscape("Mật khẩu hiện tại không đúng."))
			return
		}
	}
	pw := r.PostFormValue("password")
	if err := s.setAuthPassword(pw); err != nil {
		seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Lỗi: "+err.Error()))
		return
	}
	_ = s.Ledger.Decide("human", "auth_password", nil, "Đặt/đổi mật khẩu truy cập dashboard.", nil)
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã đặt mật khẩu. Từ giờ truy cập dashboard cần đăng nhập."))
}

// handleAuthPasswordDisable tắt mật khẩu dashboard (Đợt T).
func (s *Server) handleAuthPasswordDisable(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse password")
		return
	}
	// Đợt AF: tắt cũng cần nhập đúng mật khẩu hiện tại.
	cur := r.PostFormValue("current_password")
	stored, ok, _ := s.Ledger.GetSetting(KeyAuthPasswordHash)
	if !ok || !checkAuthPassword(cur, stored) {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Mật khẩu hiện tại không đúng."))
		return
	}
	if err := s.clearAuthPassword(); err != nil {
		seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Lỗi: "+err.Error()))
		return
	}
	_ = s.Ledger.Decide("human", "auth_password", nil, "Tắt mật khẩu truy cập dashboard.", map[string]any{"off": true})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã tắt mật khẩu."))
}

// handleSettingsHeThong: "App đang chạy chế độ gì, còn thiếu biến nào?"
func (s *Server) handleSettingsHeThong(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "system", "settings_system",
		"EnvStatus", "EnvSaved", "EnvErr", "Ok", "Err", "DbPath", "Usage", "Spend",
		"MasterOn", "APIBudget", "BudgetFrom",
		"APIEnabled", "Version", "RestorePending", "DataDir", "DiskUsage",
		"BackupLastOK", "BackupOffsiteDir", "NotifyOn", "NotifyWebhook", "NotifyChat", "NotifyHasBot",
		"TickBeats", "QuotaCircuit", "WebAIPlatforms", "WebAIConnected", "WebAIStudioReady", "GeminiModels")
}

// handleSettingsNhaCungCap: "AI dùng nhà cung cấp nào trước, key nào còn sống?"
func (s *Server) handleSettingsNhaCungCap(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "providers", "settings_providers",
		"TTSChain", "TTSKeys", "LLMChain", "LLMKeys", "GeminiFreeTierConfirmed")
}

// handleSettingsAPIToggle opens/closes the legacy /api/* JSON endpoints
// (R2-W7, R2-09). Default OFF; the operator turns it on explicitly.
func (s *Server) handleSettingsAPIToggle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse api toggle")
		return
	}
	on := r.PostFormValue("value") == "1"
	if err := automation.SetAPIEnabled(s.settings(), on); err != nil {
		s.fail(w, err, "save api switch")
		return
	}
	msg := "Đã TẮT API /api/*."
	if on {
		msg = "Đã BẬT API /api/* — chỉ dùng nội bộ; cân nhắc rủi ro khi mở app ra mạng LAN."
	}
	_ = s.Ledger.Decide("human", "api_switch", nil, msg, map[string]any{"on": on})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(msg))
}

// aiCapabilityView là một dòng trong bảng "Khả năng AI".
type aiCapabilityView struct {
	Label     string
	Status    string // ok | fail | unknown
	Badge     string // badge-ok | badge-err | badge-warn
	Text      string // nhãn tiếng Việt
	CheckedAt string
	Detail    string
}

// aiCapabilityViews reads image-generation readiness from Studio.
func (s *Server) aiCapabilityViews() []aiCapabilityView {
	if s.Studio == nil {
		return nil
	}
	var out []aiCapabilityView
	for _, c := range s.Studio.GetCapabilities() {
		v := aiCapabilityView{Label: c.Label, Status: c.Status,
			CheckedAt: c.CheckedAt, Detail: c.Detail}
		switch c.Status {
		case "ok":
			v.Badge, v.Text = "badge-ok", "Hoạt động"
		case "fail":
			v.Badge, v.Text = "badge-err", "Không hoạt động"
		default:
			v.Badge, v.Text = "badge-warn", "Chưa kiểm tra"
		}
		out = append(out, v)
	}
	return out
}

// handleSettingsAnToan: "Dừng khẩn cấp bằng cách nào?"
func (s *Server) handleSettingsAnToan(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "safety", "settings_safety")
}
