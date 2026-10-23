package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"strconv"
	"strings"
	"time"
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
	Hunted            []products.Product
	Orders            []accesstrade.SavedOrder
	OrderStats        accesstrade.OrderStats
	OrderStatsSynced  bool
	OrderStatsError   bool
	OrderSyncedAt     string
	HunterOn          bool
	HunterFlowCredits bool
	OrderSyncOn       bool
	CampaignChkOn     bool
	HunterVideos      int
	HunterLastRun     string
	OrderLastRun      string
}

type atOrderSummary struct {
	Stats     accesstrade.OrderStats
	Synced    bool
	SyncedAt  string
	KeySet    bool
	StoreOK   bool
	LoadError string
}

// atOrderSummary reads current Accesstrade order stats. Sync marker lets the
// UI distinguish a real zero from a store that has never synced.
func (s *Server) atOrderSummary() (atOrderSummary, error) {
	summary := atOrderSummary{KeySet: s.atKeySet(), StoreOK: s.AT != nil}
	if s.AT == nil {
		return summary, nil
	}
	stats, err := s.AT.GetOrderStats()
	if err != nil {
		summary.LoadError = err.Error()
		return summary, err
	}
	syncedAt, synced := s.AT.GetSyncState("orders.until")
	summary.Stats, summary.Synced, summary.SyncedAt = stats, synced, syncedAt
	return summary, nil
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
	v.HunterFlowCredits = s.atSettingOn(automation.KeyATHunterFlowCredits, false)
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
	if summary, err := s.atOrderSummary(); err != nil {
		log.Printf("web: at order stats: %v", err)
		v.OrderStatsError = true
	} else {
		v.OrderStats = summary.Stats
		v.OrderStatsSynced = summary.Synced
		v.OrderSyncedAt = summary.SyncedAt
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
