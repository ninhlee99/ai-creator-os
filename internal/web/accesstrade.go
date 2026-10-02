package web

// Accesstrade: Cài đặt · tab Accesstrade (nhập key, test kết nối) + card
// "Chiến dịch Accesstrade" trên trang Affiliate (/products): đồng bộ
// campaign, tạo tracking link qua modal, danh sách link đã tạo.
//
// Fail-closed mọi nơi: chưa có key hoặc store chưa mở → báo rõ + trỏ về
// Cài đặt · Accesstrade, không gọi API.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
)

// atKeySetting là key lưu access_key trong bảng settings của ledger.
// Đọc mỗi lần gọi API → đổi key có hiệu lực ngay, không restart.
const atKeySetting = "at.access_key"

// atTimeout là timeout cho mỗi thao tác AT từ UI.
const atTimeout = 45 * time.Second

// maskSecret đã có trong settings.go (••••abcd).

// atAccessKey đọc access_key hiện tại.
func (s *Server) atAccessKey() (string, bool) {
	if s.Ledger == nil {
		return "", false
	}
	v, ok, err := s.Ledger.GetSetting(atKeySetting)
	if err != nil || !ok || strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

// atClient dựng client từ key đang lưu; fail-closed khi thiếu key/store.
func (s *Server) atClient() (*accesstrade.Client, error) {
	if s.AT == nil {
		return nil, errATStore
	}
	key, ok := s.atAccessKey()
	if !ok {
		return nil, errATNoKey
	}
	return accesstrade.NewClient(key), nil
}

var (
	errATStore = atFail("kho Accesstrade chưa sẵn sàng — kiểm tra log khởi động app")
	errATNoKey = atFail("chưa có Accesstrade access_key — nhập key ở Cài đặt · Accesstrade")
)

type atFail string

func (e atFail) Error() string { return string(e) }

// atView gom dữ liệu AT cho template trang Affiliate.
type atView struct {
	KeySet    bool
	StoreOK   bool
	Campaigns []accesstrade.Campaign
	Links     []accesstrade.SavedLink
}

// atPageData đọc cache campaign + link đã tạo; lỗi đọc chỉ log, UI hiện
// trạng thái trung thực thay vì sập trang.
func (s *Server) atPageData() atView {
	v := atView{StoreOK: s.AT != nil}
	if _, ok := s.atAccessKey(); ok {
		v.KeySet = true
	}
	if s.AT == nil {
		return v
	}
	if cs, err := s.AT.CachedCampaigns(); err != nil {
		log.Printf("web: at campaigns cache: %v", err)
	} else {
		v.Campaigns = cs
	}
	if ls, err := s.AT.ListLinks(50); err != nil {
		log.Printf("web: at links: %v", err)
	} else {
		v.Links = ls
	}
	return v
}

// atKeySet / atMasked / atCachedCount phục vụ settingsData (tab Accesstrade).
func (s *Server) atKeySet() bool {
	_, ok := s.atAccessKey()
	return ok
}

func (s *Server) atMasked() string {
	key, ok := s.atAccessKey()
	if !ok {
		return ""
	}
	return maskSecret(key)
}

func (s *Server) atCachedCount() int {
	if s.AT == nil {
		return 0
	}
	cs, err := s.AT.CachedCampaigns()
	if err != nil {
		return 0
	}
	return len(cs)
}

// handleSettingsAccesstrade: "Accesstrade đã nối chưa, key còn sống không?"
func (s *Server) handleSettingsAccesstrade(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "accesstrade", "settings_accesstrade",
		"ATKeySet", "ATMasked", "ATCampaigns", "ATStoreOK")
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
	if err := s.Ledger.SetSetting(atKeySetting, key); err != nil {
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
	cl, err := s.atClient()
	if err != nil {
		seeOther(w, r, "/products?err="+url.QueryEscape(err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), atTimeout)
	defer cancel()
	cs, err := cl.ListCampaigns(ctx, accesstrade.CampaignFilter{})
	if err != nil {
		seeOther(w, r, "/products?err="+url.QueryEscape("Tải chiến dịch thất bại: "+err.Error()))
		return
	}
	if err := s.AT.UpsertCampaigns(cs); err != nil {
		seeOther(w, r, "/products?err="+url.QueryEscape("Lưu cache thất bại: "+err.Error()))
		return
	}
	approved := len(accesstrade.ApprovedOnly(cs))
	seeOther(w, r, "/products?ok="+url.QueryEscape(
		"Đã tải "+itoa(len(cs))+" chiến dịch ("+itoa(approved)+" đã duyệt)."))
}

// linkCreateJSON là body JSON của POST /at/links/create.
type linkCreateJSON struct {
	ProductURL string `json:"product_url"`
	CampaignID string `json:"campaign_id"`
	AccountRef string `json:"account_ref"`
	UTMSource  string `json:"utm_source"`
	UTMMedium  string `json:"utm_medium"`
	Sub1       string `json:"sub1"`
	Sub2       string `json:"sub2"`
	Sub3       string `json:"sub3"`
	Sub4       string `json:"sub4"`
}

// handleATCreateLink tạo tracking link qua API, lưu store, trả JSON.
// Chỉ campaign đã duyệt (approval=successful) mới được tạo link.
func (s *Server) handleATCreateLink(w http.ResponseWriter, r *http.Request) {
	cl, err := s.atClient()
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	var in linkCreateJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSONErr(w, "JSON không hợp lệ.", http.StatusBadRequest)
		return
	}
	in.ProductURL = strings.TrimSpace(in.ProductURL)
	in.CampaignID = strings.TrimSpace(in.CampaignID)
	if in.ProductURL == "" {
		writeJSONErr(w, "Thiếu URL sản phẩm.", http.StatusBadRequest)
		return
	}
	if in.CampaignID == "" {
		writeJSONErr(w, "Thiếu chiến dịch.", http.StatusBadRequest)
		return
	}
	// Chặn tạo link cho campaign chưa duyệt (dựa trên cache đã sync).
	if err := s.atRequireApproved(in.CampaignID); err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), atTimeout)
	defer cancel()
	link, err := cl.CreateProductLink(ctx, accesstrade.LinkRequest{
		URL:        in.ProductURL,
		CampaignID: in.CampaignID,
		UTM:        map[string]string{"source": in.UTMSource, "medium": in.UTMMedium},
		Sub1:       in.Sub1, Sub2: in.Sub2, Sub3: in.Sub3, Sub4: in.Sub4,
	})
	if err != nil {
		writeJSONErr(w, "Tạo link thất bại: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := s.AT.SaveLink(in.ProductURL, in.CampaignID,
		strings.TrimSpace(in.AccountRef), link.TrackingLink, link.ShortLink); err != nil {
		writeJSONErr(w, "Tạo link OK nhưng lưu thất bại: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "tracking_link": link.TrackingLink, "short_link": link.ShortLink,
	})
}

// atRequireApproved chặn tạo link khi campaign chưa ở trạng thái
// "successful" trong cache. Cache trống → yêu cầu sync trước (không đoán).
func (s *Server) atRequireApproved(campaignID string) error {
	cs, err := s.AT.CachedCampaigns()
	if err != nil {
		return err
	}
	if len(cs) == 0 {
		return atFail("chưa có dữ liệu chiến dịch — bấm “Tải chiến dịch” trước")
	}
	for _, c := range cs {
		if c.ID == campaignID {
			if c.Approval == "successful" {
				return nil
			}
			return atFail("chiến dịch “" + c.Name + "” chưa duyệt (" + c.ApprovalLabel() + ") — không tạo được link")
		}
	}
	return atFail("không tìm thấy chiến dịch trong cache — bấm “Tải chiến dịch” để làm mới")
}

func itoa(n int) string { return strconv.Itoa(n) }
