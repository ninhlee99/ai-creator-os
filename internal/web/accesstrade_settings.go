package web

import (
	"context"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) handleSettingsAccesstrade(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "accesstrade", "settings_accesstrade",
		"ATKeySet", "ATMasked", "ATCampaigns", "ATStoreOK",
		"ATHunterHours", "ATOrderSyncMins", "ATAuto", "ATHunterAccounts", "ATHunterAccountID")
}

// handleATAutomationSave lưu cadence và công tắc Accesstrade từ form
// Cài đặt · Accesstrade. Caller cũ không gửi toggles chỉ đổi cadence.
func (s *Server) handleATAutomationSave(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Kho dữ liệu chưa sẵn sàng."))
		return
	}
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Không đọc được form."))
		return
	}
	hunterH, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("hunter_hours")))
	if hunterH < 1 {
		hunterH = 1
	}
	if hunterH > 168 {
		hunterH = 168
	}
	syncM, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("ordersync_mins")))
	if syncM < 5 {
		syncM = 5
	}
	if syncM > 1440 {
		syncM = 1440
	}
	type settingWrite struct {
		key    string
		value  string
		delete bool
	}
	writes := []settingWrite{
		{key: automation.KeyATHunterIntervalHrs, value: strconv.Itoa(hunterH)},
		{key: automation.KeyATOrderSyncIntervalM, value: strconv.Itoa(syncM)},
	}
	// Đợt AK: form gộp gửi kèm công tắc (checkbox bỏ trống = tắt). Chỉ áp
	// khi có cờ toggles để form/gọi cũ chỉ gửi chu kỳ không vô tình tắt hết.
	if r.PostFormValue("toggles") == "1" {
		on := func(k string) string { return boolStr(r.PostFormValue(k) == "1") }
		n, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("hunter_videos")))
		n = clampInt(n, 0, 10)
		hunterOn := r.PostFormValue("hunter_on") == "1"
		hunterMuseCredits := r.PostFormValue("hunter_flow_credits") == "1"
		rawAccountID := strings.TrimSpace(r.PostFormValue("hunter_account_id"))
		if hunterOn && n > 0 && hunterMuseCredits && rawAccountID == "" {
			seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Đang bật Muse AI credits cho video hunter: chọn kênh đích có ảnh nhân vật, hoặc tắt credits/đặt số video về 0."))
			return
		}
		if rawAccountID != "" {
			accountID, parseErr := strconv.ParseInt(rawAccountID, 10, 64)
			if parseErr != nil || accountID <= 0 || s.Mgr == nil {
				seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Kênh đích hunter không hợp lệ."))
				return
			}
			account, getErr := s.Mgr.Get(accountID)
			if getErr != nil || account == nil || s.Studio == nil {
				seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Không xác minh được kênh đích hoặc Studio."))
				return
			}
			library, libErr := s.Studio.ModelLibrary()
			hasPhotos := false
			if libErr == nil {
				hasPhotos, libErr = library.HasPhotos(accountID)
			}
			if libErr != nil || !hasPhotos {
				seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Kênh đích chưa có ảnh nhân vật trong thư viện riêng."))
				return
			}
			writes = append(writes, settingWrite{key: automation.KeyATHunterAccountID, value: strconv.FormatInt(accountID, 10)})
		} else {
			writes = append(writes, settingWrite{key: automation.KeyATHunterAccountID, delete: true})
		}
		writes = append(writes,
			settingWrite{key: automation.KeyATHunterEnabled, value: on("hunter_on")},
			settingWrite{key: automation.KeyATOrderSyncEnabled, value: on("ordersync_on")},
			settingWrite{key: automation.KeyATCampaignCheckOn, value: on("campaign_on")},
			settingWrite{key: automation.KeyATHunterVideos, value: itoa(n)},
			settingWrite{key: automation.KeyATHunterFlowCredits, value: on("hunter_flow_credits")},
		)
	}
	for _, write := range writes {
		var err error
		if write.delete {
			err = s.Ledger.DeleteSetting(write.key)
		} else {
			err = s.Ledger.SetSetting(write.key, write.value)
		}
		if err != nil {
			message := "Lưu cài đặt " + write.key + " thất bại: " + err.Error() + ". Các mục ghi trước đó có thể đã lưu."
			seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape(message))
			return
		}
	}
	_ = s.Ledger.Decide("human", "at_automation_save", nil, "Lưu cài đặt tự động Accesstrade.", nil)
	seeOther(w, r, "/settings/accesstrade?ok="+url.QueryEscape("Đã lưu cài đặt tự động Accesstrade."))
}

// handleATKeySave lưu access_key từ form (POST, redirect 303).
func (s *Server) handleATKeySave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Không đọc được form."))
		return
	}
	key := strings.TrimSpace(r.PostFormValue("access_key"))
	if key == "" {
		seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Key trống — chưa lưu gì."))
		return
	}
	if s.Ledger == nil {
		seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Kho dữ liệu chưa sẵn sàng."))
		return
	}
	if err := s.Ledger.SetSetting(accesstrade.KeySetting, key); err != nil {
		seeOther(w, r, "/settings/accesstrade?err="+url.QueryEscape("Lưu key thất bại: "+err.Error()))
		return
	}
	log.Printf("web: accesstrade key saved (masked %s)", maskSecret(key))
	seeOther(w, r, "/settings/accesstrade?ok="+url.QueryEscape("Đã lưu key. Bấm “Kiểm tra kết nối” để thử."))
}

// handleATTest gọi thử ListCampaigns(limit=1), trả JSON cho toast AJAX.
// Key thật chỉ dùng để gọi API, không bao giờ trả về client.
func (s *Server) handleATTest(w http.ResponseWriter, r *http.Request) {
	cl, err := s.atClient()
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), atTimeout)
	defer cancel()
	cs, err := cl.ListCampaigns(ctx, accesstrade.CampaignFilter{Limit: 1})
	if err != nil {
		writeJSONErr(w, "Kết nối thất bại: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "count": len(cs)})
}

// handleATSyncCampaigns đồng bộ campaign từ API vào cache (POST → redirect).
func (s *Server) handleATSyncCampaigns(w http.ResponseWriter, r *http.Request) {
	redirect := func(key, message string) string {
		values := url.Values{}
		if r.URL.Query().Get("tab") == "affiliate" {
			values.Set("tab", "affiliate")
		}
		values.Set(key, message)
		return "/products?" + values.Encode()
	}
	cl, err := s.atClient()
	if err != nil {
		seeOther(w, r, redirect("err", err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), atTimeout)
	defer cancel()
	// Chỉ chiến dịch đã duyệt mới tạo được link; danh sách công khai có
	// ~10.000 chiến dịch (~9MB/trang) nên không tải hết.
	cs, err := cl.ListCampaigns(ctx, accesstrade.CampaignFilter{Approval: "successful"})
	if err != nil {
		seeOther(w, r, redirect("err", "Tải chiến dịch thất bại: "+err.Error()))
		return
	}
	if err := s.AT.UpsertCampaigns(cs); err != nil {
		seeOther(w, r, redirect("err", "Lưu cache thất bại: "+err.Error()))
		return
	}
	msg := "Đã tải " + itoa(len(cs)) + " chiến dịch bạn đã được duyệt."
	if len(cs) == 0 {
		msg = "Chưa có chiến dịch nào được duyệt — đăng ký chiến dịch trên pub.accesstrade.vn rồi tải lại."
	} else if len(cs) < 3 {
		msg += " Nên đăng ký thêm chiến dịch đúng ngành hàng (thời trang, mỹ phẩm, gia dụng…) trên pub.accesstrade.vn."
	}
	seeOther(w, r, redirect("ok", msg))
}

// linkCreateJSON là body JSON của POST /at/links/create.
