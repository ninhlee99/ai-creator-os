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
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// Key lưu access_key: dùng chung accesstrade.KeySetting với automation tick
// (một nguồn duy nhất — đọc mỗi lần gọi API, đổi key hiệu lực ngay).

// atTimeout là timeout cho mỗi thao tác AT từ UI.
const atTimeout = 45 * time.Second

// maskSecret đã có trong settings.go (••••abcd).

// atAccessKey đọc access_key hiện tại.
func (s *Server) atAccessKey() (string, bool) {
	if s.Ledger == nil {
		return "", false
	}
	v, ok, err := s.Ledger.GetSetting(accesstrade.KeySetting)
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
	// Đợt C: hunter + đối soát.
	Hunted        []products.Product
	Orders        []accesstrade.SavedOrder
	OrderStats    accesstrade.OrderStats
	HunterOn      bool
	OrderSyncOn   bool
	CampaignChkOn bool
	HunterVideos  int
	HunterLastRun string
	OrderLastRun  string
}

// atPageData đọc cache campaign + link đã tạo; lỗi đọc chỉ log, UI hiện
// trạng thái trung thực thay vì sập trang.
func (s *Server) atPageData() atView {
	v := atView{StoreOK: s.AT != nil}
	if _, ok := s.atAccessKey(); ok {
		v.KeySet = true
	}
	// Công tắc tick chỉ cần Ledger — đọc luôn để UI hiển thị đúng
	// cả khi kho AT chưa mở.
	v.HunterOn = s.atSettingOn(automation.KeyATHunterEnabled, true)
	v.OrderSyncOn = s.atSettingOn(automation.KeyATOrderSyncEnabled, true)
	v.CampaignChkOn = s.atSettingOn(automation.KeyATCampaignCheckOn, true)
	v.HunterVideos = s.atSettingInt(automation.KeyATHunterVideos, 3)
	if s.Ledger != nil {
		if lr, ok, _ := s.Ledger.GetSetting("at.hunter_last_run"); ok {
			v.HunterLastRun = lr
		}
		if lr, ok, _ := s.Ledger.GetSetting("at.ordersync_last_run"); ok {
			v.OrderLastRun = lr
		}
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
	if os, err := s.AT.ListOrders(20); err != nil {
		log.Printf("web: at orders: %v", err)
	} else {
		v.Orders = os
	}
	if st, err := s.AT.GetOrderStats(); err != nil {
		log.Printf("web: at order stats: %v", err)
	} else {
		v.OrderStats = st
	}
	if s.Products != nil {
		if hs, err := s.Products.TopUnusedBySource(
			accesstrade.HunterSource, 0, 0, 10); err != nil {
			log.Printf("web: at hunted: %v", err)
		} else {
			v.Hunted = hs
		}
	}
	return v
}

// atSettingOn / atSettingInt đọc công tắc UI cho tick AT (unset = def).
func (s *Server) atSettingOn(key string, def bool) bool {
	if s.Ledger == nil {
		return def
	}
	v, ok, err := s.Ledger.GetSetting(key)
	if err != nil || !ok {
		return def
	}
	return v == "1" || v == "true"
}

func (s *Server) atSettingInt(key string, def int) int {
	if s.Ledger == nil {
		return def
	}
	if v, ok, err := s.Ledger.GetSetting(key); err == nil && ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return def
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

// ------------------------------------------------------------------ Đợt C

// handleATHunt chạy hunter tay (AJAX): quét datafeed → kho sản phẩm →
// top N mới tạo video affiliate. Trả JSON cho toast.
func (s *Server) handleATHunt(w http.ResponseWriter, r *http.Request) {
	cl, err := s.atClient()
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	if s.Products == nil {
		writeJSONErr(w, "kho sản phẩm chưa sẵn sàng", http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, err := cl.Hunt(ctx, s.Products, accesstrade.HunterFilter{})
	if err != nil {
		writeJSONErr(w, "Săn sản phẩm thất bại: "+err.Error(), http.StatusBadGateway)
		return
	}
	// Tạo video cho top N sản phẩm mới (nối hunter → studio, như tick).
	videos := 0
	want := s.atSettingInt(automation.KeyATHunterVideos, 3)
	if s.Studio != nil && want > 0 {
		for _, p := range res.Products {
			if videos >= want {
				break
			}
			jobID, jerr := s.makeHunterVideo(ctx, p)
			if jerr != nil {
				log.Printf("web: at hunt video %q: %v", p.Title, jerr)
				continue
			}
			_ = s.Products.RecordUse(p.ID, 0, jobID)
			videos++
		}
	}
	if s.Ledger != nil {
		_ = s.Ledger.SetSetting("at.hunter_last_run", time.Now().Format(time.RFC3339))
	}
	writeJSON(w, map[string]any{
		"ok": true, "new": res.New, "updated": res.Updated,
		"skipped": res.Skipped, "videos": videos,
	})
}

// makeHunterVideo tạo 1 studio job affiliate từ sản phẩm hunter (bản web
// của automation tick — tải ảnh listing + gọi CreateAffiliateJob).
func (s *Server) makeHunterVideo(ctx context.Context, p products.Product) (string, error) {
	auto := s.automation()
	auto.StudioRunner = s.Studio
	return auto.MakeHunterVideo(ctx, p)
}

// handleATOrderSync đồng bộ đơn tay (AJAX): order-list → upsert → JSON
// số liệu đối soát cho toast + cập nhật card.
func (s *Server) handleATOrderSync(w http.ResponseWriter, r *http.Request) {
	cl, err := s.atClient()
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	until := time.Now()
	since := until.Add(-7 * 24 * time.Hour)
	orders, err := cl.ListOrders(ctx, since, until)
	if err != nil {
		writeJSONErr(w, "Đồng bộ đơn thất bại: "+err.Error(), http.StatusBadGateway)
		return
	}
	added, updated, uerr := s.AT.UpsertOrders(orders)
	if uerr != nil {
		writeJSONErr(w, "Lưu đơn thất bại: "+uerr.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.AT.SetSyncState("orders.until", until.UTC().Format(time.RFC3339))
	if s.Ledger != nil {
		_ = s.Ledger.SetSetting("at.ordersync_last_run", time.Now().Format(time.RFC3339))
	}
	st, _ := s.AT.GetOrderStats()
	writeJSON(w, map[string]any{
		"ok": true, "total": len(orders), "added": added, "updated": updated,
		"approved_count": st.ApprovedCount, "approved_total": st.ApprovedTotal,
		"pending_count": st.PendingCount, "pending_total": st.PendingTotal,
		"rejected_count": st.RejectedCount,
	})
}

// atSettingsJSON là body JSON của POST /at/settings.
type atSettingsJSON struct {
	HunterOn     *bool `json:"hunter_on"`
	HunterVideos *int  `json:"hunter_videos"`
	OrderSyncOn  *bool `json:"order_sync_on"`
	CampaignOn   *bool `json:"campaign_on"`
}

// handleATSettings lưu công tắc tick AT (AJAX, JSON).
func (s *Server) handleATSettings(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		writeJSONErr(w, "kho dữ liệu chưa sẵn sàng", http.StatusPreconditionFailed)
		return
	}
	var in atSettingsJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeJSONErr(w, "JSON không hợp lệ.", http.StatusBadRequest)
		return
	}
	set := func(key string, v *bool) {
		if v == nil {
			return
		}
		_ = s.Ledger.SetSetting(key, boolStr(*v))
	}
	set(automation.KeyATHunterEnabled, in.HunterOn)
	set(automation.KeyATOrderSyncEnabled, in.OrderSyncOn)
	set(automation.KeyATCampaignCheckOn, in.CampaignOn)
	if in.HunterVideos != nil {
		n := *in.HunterVideos
		if n < 0 {
			n = 0
		}
		if n > 10 {
			n = 10
		}
		_ = s.Ledger.SetSetting(automation.KeyATHunterVideos, itoa(n))
	}
	writeJSON(w, map[string]any{"ok": true})
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
