package accesstrade

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"

	_ "modernc.org/sqlite"
)

// openRaw mở DB thô để giả lập trạng thái schema cũ.
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// --- datafeeds ---

func TestListDatafeeds(t *testing.T) {
	srv := mockServer(t, "k123", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/datafeeds" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if lim := r.URL.Query().Get("limit"); lim != "200" {
			t.Errorf("limit = %q, muốn 200 (max)", lim)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"sku":"A1","name":"Tai nghe X","price":299000,"old_price":399000,"currency":"VND",
			 "aff_link":"https://go.at/a1","category":"cong-nghe","commission_rate":8.5,
			 "image":"https://img/x.jpg"},
			{"product_id":"B2","product_name":"Son Y","sale_price":"159000",
			 "affiliate_link":"https://go.at/b2","commissionRate":0.12,
			 "images":["https://img/y1.jpg","https://img/y2.jpg"]},
			{"sku":"C3","name":"Không link","price":1000}
		]}`))
	})
	c := NewClient("k123")
	c.BaseURL = srv.URL
	ps, err := c.ListDatafeeds(context.Background(), DatafeedFilter{})
	if err != nil {
		t.Fatalf("ListDatafeeds: %v", err)
	}
	if len(ps) != 3 {
		t.Fatalf("muốn 3 sản phẩm, được %d", len(ps))
	}
	// Sort client-side: hoa hồng cao nhất trước (12% > 8.5% > 0).
	if ps[0].SKU != "B2" || ps[0].CommissionRate != 0.12 {
		t.Errorf("sort sai: đầu phải là B2/0.12, được %+v", ps[0])
	}
	if ps[1].CommissionRate != 0.085 {
		t.Errorf("8.5%% phải chuẩn hoá về 0.085, được %v", ps[1].CommissionRate)
	}
	if ps[0].Image() != "https://img/y1.jpg" || len(ps[0].Images) != 2 {
		t.Errorf("images parse sai: %+v", ps[0].Images)
	}
	if ps[1].DiscountPct() < 24 || ps[1].DiscountPct() > 26 {
		t.Errorf("discount sai: %v", ps[1].DiscountPct())
	}
	if got := ps[1].PriceLabel(); got != "299.000₫" {
		t.Errorf("PriceLabel = %q, muốn 299.000₫", got)
	}
}

func TestListDatafeedsLimitCap(t *testing.T) {
	var gotLimit string
	srv := mockServer(t, "k", func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	c := NewClient("k")
	c.BaseURL = srv.URL
	if _, err := c.ListDatafeeds(context.Background(), DatafeedFilter{Limit: 9999}); err != nil {
		t.Fatal(err)
	}
	if gotLimit != "200" {
		t.Errorf("limit phải cắt về 200, được %q", gotLimit)
	}
}

// --- orders ---

func TestListOrders(t *testing.T) {
	srv := mockServer(t, "k123", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/order-list" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("since") == "" || r.URL.Query().Get("until") == "" {
			t.Errorf("thiếu since/until: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"order_id":"O1","campaign_name":"Shopee","status":1,"pub_commission":45000,
			 "utm_source":"tiktok","ordered_at":"2026-10-01T10:00:00Z"},
			{"orderId":"O2","status":"0","commission":0},
			{"id":"O3","status":"rejected","pub_commission":"12000"}
		]}`))
	})
	c := NewClient("k123")
	c.BaseURL = srv.URL
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	orders, err := c.ListOrders(context.Background(), since, time.Now())
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if len(orders) != 3 {
		t.Fatalf("muốn 3 đơn, được %d", len(orders))
	}
	if orders[0].Status != OrderApproved || orders[0].PubCommission != 45000 {
		t.Errorf("đơn 0 sai: %+v", orders[0])
	}
	if orders[1].Status != OrderPending || orders[1].PubCommission != 0 {
		t.Errorf("đơn pending phải status=0 commission=0: %+v", orders[1])
	}
	if orders[2].Status != OrderRejected {
		t.Errorf("đơn 2 phải rejected: %+v", orders[2])
	}
	if orders[1].StatusLabel() != "Chờ duyệt" || orders[0].StatusLabel() != "Đã duyệt" {
		t.Errorf("label sai: %q / %q", orders[1].StatusLabel(), orders[0].StatusLabel())
	}
	if orders[1].CommissionLabel() != "—" {
		t.Errorf("pending chưa có số phải là —, được %q", orders[1].CommissionLabel())
	}
}

// TestOrderRateLimit dùng clock/sleep giả: 3 lần gọi với limit 2/phút
// → lần 3 phải "chờ" ~60s (ghi nhận, không chờ thật).
func TestOrderRateLimit(t *testing.T) {
	now := time.Now()
	var slept []time.Duration
	l := &rateLimiter{
		perMinute: 2,
		now:       func() time.Time { return now },
		sleep: func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			now = now.Add(d) // tua clock giả
			return nil
		},
	}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := l.wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(slept) != 1 {
		t.Fatalf("muốn đúng 1 lần chờ, được %d", len(slept))
	}
	if slept[0] < 59*time.Second || slept[0] > 61*time.Second {
		t.Errorf("thời gian chờ phải ~60s, được %v", slept[0])
	}
	// Cửa sổ mới sau 60s đã có sẵn 1 hit → 1 lần đầu không chờ,
	// lần tiếp theo lại chờ.
	slept = nil
	if err := l.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 0 {
		t.Fatalf("lần đầu cửa sổ mới không được chờ, chờ %d lần", len(slept))
	}
	if err := l.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 {
		t.Errorf("lần 2 cửa sổ mới phải chờ 1 lần, được %d", len(slept))
	}
}

func TestOrderRateLimitCtxCancel(t *testing.T) {
	l := &rateLimiter{perMinute: 1, now: time.Now,
		sleep: func(ctx context.Context, d time.Duration) error { return ctx.Err() }}
	ctx, cancel := context.WithCancel(context.Background())
	if err := l.wait(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := l.wait(ctx); err == nil {
		t.Errorf("ctx hủy phải trả lỗi")
	}
}

// --- store v2: orders + sync state ---

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "at.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestUpsertOrdersUpdateInPlace(t *testing.T) {
	s := testStore(t)
	// Lần 1: đơn pending, commission = 0 (lúc hold).
	added, updated, err := s.UpsertOrders([]Order{
		{OrderID: "O1", CampaignName: "Shopee", Status: OrderPending, PubCommission: 0},
		{OrderID: "O2", CampaignName: "Lazada", Status: OrderApproved, PubCommission: 30000},
	})
	if err != nil {
		t.Fatalf("UpsertOrders: %v", err)
	}
	if added != 2 || updated != 0 {
		t.Fatalf("added=%d updated=%d, muốn 2/0", added, updated)
	}
	// Lần 2: O1 được duyệt, có commission — phải UPDATE tại chỗ.
	added, updated, err = s.UpsertOrders([]Order{
		{OrderID: "O1", CampaignName: "Shopee", Status: OrderApproved, PubCommission: 45000},
	})
	if err != nil {
		t.Fatalf("UpsertOrders: %v", err)
	}
	if added != 0 || updated != 1 {
		t.Fatalf("added=%d updated=%d, muốn 0/1", added, updated)
	}
	orders, err := s.ListOrders(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 {
		t.Fatalf("muốn 2 đơn, được %d", len(orders))
	}
	var o1 *SavedOrder
	for i := range orders {
		if orders[i].OrderID == "O1" {
			o1 = &orders[i]
		}
	}
	if o1 == nil || o1.Status != OrderApproved || o1.PubCommission != 45000 {
		t.Errorf("O1 phải được update tại chỗ (approved/45000): %+v", o1)
	}
}

func TestOrderStatsHonest(t *testing.T) {
	s := testStore(t)
	_, _, err := s.UpsertOrders([]Order{
		{OrderID: "A1", Status: OrderApproved, PubCommission: 50000},
		{OrderID: "A2", Status: OrderApproved, PubCommission: 25000},
		{OrderID: "P1", Status: OrderPending, PubCommission: 0},
		{OrderID: "P2", Status: OrderPending, PubCommission: 10000},
		{OrderID: "R1", Status: OrderRejected, PubCommission: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.GetOrderStats()
	if err != nil {
		t.Fatal(err)
	}
	if st.ApprovedCount != 2 || st.ApprovedTotal != 75000 {
		t.Errorf("approved sai: %+v", st)
	}
	// Trung thực: pending hiện riêng, không cộng vào approved.
	if st.PendingCount != 2 || st.PendingTotal != 10000 {
		t.Errorf("pending sai: %+v", st)
	}
	if st.RejectedCount != 1 {
		t.Errorf("rejected sai: %+v", st)
	}
}

func TestSyncState(t *testing.T) {
	s := testStore(t)
	if _, ok := s.GetSyncState("orders.since"); ok {
		t.Errorf("chưa set phải ok=false")
	}
	if err := s.SetSyncState("orders.since", "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	v, ok := s.GetSyncState("orders.since")
	if !ok || v != "2026-10-01T00:00:00Z" {
		t.Errorf("sync state sai: %q %v", v, ok)
	}
}

func TestMaskedID(t *testing.T) {
	o := SavedOrder{OrderID: "AT2026XYZ12345"}
	if got := o.MaskedID(); got != "AT20••••2345" {
		t.Errorf("MaskedID = %q", got)
	}
}

// TestMigrateV1toV2 tạo DB schema v1 thủ công, mở bằng NewStore → phải có
// bảng mới và user_version=2, dữ liệu cũ còn nguyên.
func TestMigrateV1toV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "at.db")
	s1, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.UpsertCampaigns([]Campaign{{ID: "c1", Name: "Shopee"}}); err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()
	// Giả lập DB v1: hạ user_version về 1 (code hiện tại stamp 2).
	db := openRaw(t, path)
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("migrate v1→v2: %v", err)
	}
	defer s2.Close()
	// Bảng mới tồn tại, dữ liệu cũ còn.
	if _, _, err := s2.UpsertOrders([]Order{{OrderID: "OX"}}); err != nil {
		t.Errorf("at_orders chưa sẵn sàng sau migrate: %v", err)
	}
	cs, err := s2.CachedCampaigns()
	if err != nil || len(cs) != 1 || cs[0].ID != "c1" {
		t.Errorf("dữ liệu v1 mất sau migrate: %+v %v", cs, err)
	}
}

// --- hunter ---

func TestHunt(t *testing.T) {
	var calls int64
	srv := mockServer(t, "k", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"sku":"H1","name":"Tai nghe Bluetooth Pro","price":459000,"currency":"VND",
			 "aff_link":"https://go.at/h1","category":"công nghệ","commission_rate":10,
			 "image":"https://img/h1.jpg","shop_name":"ShopA"},
			{"sku":"H2","name":"Váy hoa nhí","price":189000,
			 "aff_link":"https://go.at/h2","category":"thời trang nữ","commission_rate":15,
			 "image":"https://img/h2.jpg"},
			{"sku":"H3","name":"Thiếu link","price":99000,"commission_rate":20},
			{"sku":"H4","name":"Giá 0","price":0,"aff_link":"https://go.at/h4"}
		]}`))
	})
	c := NewClient("k")
	c.BaseURL = srv.URL
	ps, err := products.NewStore(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	res, err := c.Hunt(context.Background(), ps, HunterFilter{})
	if err != nil {
		t.Fatalf("Hunt: %v", err)
	}
	if res.New != 2 || res.Skipped != 2 {
		t.Errorf("New=%d Skipped=%d, muốn 2/2 (thiếu link + giá 0 bị loại)", res.New, res.Skipped)
	}
	if len(res.Products) != 2 {
		t.Fatalf("Products mới = %d, muốn 2", len(res.Products))
	}
	// Sort hoa hồng cao trước: H2 (15%) trước H1 (10%).
	if res.Products[0].SourceID != "H2" {
		t.Errorf("sản phẩm đầu phải là H2 (hoa hồng cao nhất): %+v", res.Products[0])
	}
	p := res.Products[0]
	if p.Source != HunterSource || p.ProductURL != "https://go.at/h2" {
		t.Errorf("map sai: %+v", p)
	}
	if p.Theme != "thoi-trang-nu" {
		t.Errorf("theme map sai: %q", p.Theme)
	}
	if p.CommissionRate != 0.15 || p.Currency != "VND" {
		t.Errorf("commission/currency sai: %+v", p)
	}
	// Quét lại → dedupe, không thêm mới.
	res2, err := c.Hunt(context.Background(), ps, HunterFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if res2.New != 0 || res2.Updated != 2 {
		t.Errorf("quét lại phải dedupe: New=%d Updated=%d", res2.New, res2.Updated)
	}
	if n, _ := ps.Count(); n != 2 {
		t.Errorf("kho phải có đúng 2 sản phẩm, có %d", n)
	}
}

func TestHuntNoKey(t *testing.T) {
	c := NewClient("")
	ps, _ := products.NewStore(filepath.Join(t.TempDir(), "p.db"))
	defer ps.Close()
	if _, err := c.Hunt(context.Background(), ps, HunterFilter{}); err == nil {
		t.Errorf("chưa có key phải fail-closed")
	}
}

func TestThemeFor(t *testing.T) {
	cases := map[string]string{
		"Tai nghe bluetooth không dây": "cong-nghe",
		"Váy đầm dự tiệc":              "thoi-trang-nu",
		"Son kem lì":                   "my-pham",
		"Nồi chiên không dầu":          "gia-dung",
		"xyz không rõ":                 "",
	}
	for name, want := range cases {
		if got := themeFor("", name); got != want {
			t.Errorf("themeFor(%q) = %q, muốn %q", name, got, want)
		}
	}
}

func TestNormalizeRate(t *testing.T) {
	if normalizeRate(8.5) != 0.085 || normalizeRate(0.12) != 0.12 {
		t.Errorf("normalize sai")
	}
	if normalizeRate(0) != 0 || normalizeRate(150) != 0 || normalizeRate(-1) != 0 {
		t.Errorf("giá trị vô lý phải về 0")
	}
}

func TestThousands(t *testing.T) {
	if Thousands(299000) != "299.000" || Thousands(999) != "999" || Thousands(1000000) != "1.000.000" {
		t.Errorf("thousands sai: %q %q %q", Thousands(299000), Thousands(999), Thousands(1000000))
	}
}

func TestSavedOrderLabels(t *testing.T) {
	o := SavedOrder{OrderID: "O1", Status: OrderPending, PubCommission: 0}
	if o.StatusLabel() != "Chờ duyệt" || o.CommissionLabel() != "—" {
		t.Errorf("label sai: %q / %q", o.StatusLabel(), o.CommissionLabel())
	}
	if !strings.Contains(SavedOrder{OrderID: "O1"}.MaskedID(), "O1") {
		t.Errorf("id ngắn giữ nguyên")
	}
}
