package automation

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// mapSettings là Settings giả cho test tick (không cần ledger thật).
type mapSettings map[string]string

func (m mapSettings) Get(k string) (string, bool) { v, ok := m[k]; return v, ok }
func (m mapSettings) Set(k, v string) error       { m[k] = v; return nil }

// fakeStudioRunner ghi nhận job affiliate được tạo.
type fakeStudioRunner struct {
	jobs []studio.AffiliateParams
}

func (f *fakeStudioRunner) CreateAffiliateJob(p studio.AffiliateParams) (string, error) {
	f.jobs = append(f.jobs, p)
	return fmt.Sprintf("job-%d", len(f.jobs)), nil
}

// tinyPNG là ảnh PNG 1x1 hợp lệ cho test tải ảnh.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
	0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0x3f,
	0x00, 0x05, 0xfe, 0x02, 0xfe, 0xdc, 0xcc, 0x59, 0xe7, 0x00, 0x00, 0x00,
	0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// atTestServer dựng mock Accesstrade: datafeed + order-list + campaigns,
// ảnh sản phẩm trỏ về chính nó (/img).
func atTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := "http://" + r.Host
		switch r.URL.Path {
		case "/img":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(tinyPNG)
		case "/v1/datafeeds":
			_, _ = fmt.Fprintf(w, `{"data":[
				{"sku":"T1","name":"Tai nghe Pro","price":459000,"currency":"VND",
				 "aff_link":"https://go.at/t1","category":"công nghệ","commission_rate":10,
				 "image":"%s/img"},
				{"sku":"T2","name":"Váy hoa","price":189000,"aff_link":"https://go.at/t2",
				 "category":"thời trang","commission_rate":15,"image":"%s/img"}
			]}`, host, host)
		case "/v1/order-list":
			_, _ = w.Write([]byte(`{"data":[
				{"order_id":"DO1","campaign_name":"Shopee","status":1,"pub_commission":45000},
				{"order_id":"DO2","campaign_name":"Lazada","status":0,"pub_commission":0}
			]}`))
		case "/v1/campaigns":
			_, _ = w.Write([]byte(`{"data":[
				{"campaign_id":"c1","name":"Shopee","approval":"successful"},
				{"campaign_id":"c2","name":"Bank X","approval":"pending"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newATService(t *testing.T, st mapSettings, srv *httptest.Server) (*Service, *fakeStudioRunner) {
	t.Helper()
	dir := t.TempDir()
	l, err := ledger.New(filepath.Join(dir, "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	atStore, err := accesstrade.NewStore(filepath.Join(dir, "at.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = atStore.Close() })
	ps, err := products.NewStore(filepath.Join(dir, "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ps.Close() })
	sr := &fakeStudioRunner{}
	svc := NewService()
	svc.Gate = &fakeGate{}
	svc.Settings = st
	svc.Ledger = l
	svc.AT = atStore
	svc.Products = ps
	svc.StudioRunner = sr
	svc.ATBaseURL = srv.URL
	svc.ATWorkDir = filepath.Join(dir, "atwork")
	return svc, sr
}

// --- gates ---

func TestATTickGates(t *testing.T) {
	srv := atTestServer()
	defer srv.Close()
	ctx := context.Background()

	// Kill switch → im lặng.
	svc, _ := newATService(t, mapSettings{accesstrade.KeySetting: "k"}, srv)
	svc.Gate = &fakeGate{kill: true}
	if notes := svc.ATTick(ctx); len(notes) != 0 {
		t.Errorf("kill switch phải chặn hết, được %v", notes)
	}
	// DRY-RUN → im lặng.
	svc, _ = newATService(t, mapSettings{accesstrade.KeySetting: "k"}, srv)
	svc.Gate = &fakeGate{dry: true}
	if notes := svc.ATTick(ctx); len(notes) != 0 {
		t.Errorf("dry-run phải chặn hết, được %v", notes)
	}
	// Chưa có key → bỏ qua im lặng (không gọi API, không alert).
	svc, _ = newATService(t, mapSettings{}, srv)
	if notes := svc.ATTick(ctx); len(notes) != 0 {
		t.Errorf("thiếu key phải bỏ qua im lặng, được %v", notes)
	}
	// Tắt hunter trong settings → hunter không chạy (order sync vẫn chạy).
	svc, _ = newATService(t, mapSettings{
		accesstrade.KeySetting: "k",
		KeyATHunterEnabled:     "0",
		// order sync + campaign check cũng tắt để ATTick im lặng hẳn.
		KeyATOrderSyncEnabled: "0",
		KeyATCampaignCheckOn:  "0",
	}, srv)
	if notes := svc.ATTick(ctx); len(notes) != 0 {
		t.Errorf("tắt hết tick phải im lặng, được %v", notes)
	}
}

// --- hunter ---

func TestATHunterTick(t *testing.T) {
	srv := atTestServer()
	defer srv.Close()
	svc, sr := newATService(t, mapSettings{
		accesstrade.KeySetting: "k",
		KeyATHunterVideos:      "1", // chỉ tạo 1 video
		KeyATOrderSyncEnabled:  "0",
		KeyATCampaignCheckOn:   "0",
	}, srv)

	notes := svc.ATHunterTick(context.Background())
	if len(notes) != 1 {
		t.Fatalf("muốn 1 note, được %v", notes)
	}
	if !strings.Contains(notes[0], "2 mới") || !strings.Contains(notes[0], "1 video") {
		t.Errorf("note sai: %q", notes[0])
	}
	// Chỉ top 1 (hoa hồng cao nhất = T2) được tạo video.
	if len(sr.jobs) != 1 {
		t.Fatalf("muốn 1 job, được %d", len(sr.jobs))
	}
	j := sr.jobs[0]
	if j.ProductName != "Váy hoa" || j.Mode != studio.AffiliateModePhoto {
		t.Errorf("job sai: %+v", j)
	}
	if j.ProductPhoto == "" {
		t.Errorf("job thiếu ảnh sản phẩm đã tải")
	}
	// Dedupe: sản phẩm đã dùng không được tạo video lại.
	top, err := svc.Products.TopUnusedBySource("accesstrade", 0, hunterDirectAccountID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].SourceID != "T1" {
		t.Errorf("sau khi dùng T2, chỉ còn T1 chưa dùng: %+v", top)
	}
	// Cadence: chạy lại ngay → không chạy nữa.
	if notes := svc.ATHunterTick(context.Background()); len(notes) != 0 {
		t.Errorf("chưa đến hạn phải im lặng, được %v", notes)
	}
	// Quá hạn → chạy lại: 0 sản phẩm mới → không tạo thêm video
	// (sản phẩm cũ còn lại do autopilot theo account dùng tiếp).
	old := time.Now().Add(-25 * time.Hour).Format(time.RFC3339)
	_ = svc.Settings.Set(keyATHunterLastRun, old)
	svc2sr := &fakeStudioRunner{}
	svc.StudioRunner = svc2sr
	notes = svc.ATHunterTick(context.Background())
	if len(svc2sr.jobs) != 0 {
		t.Errorf("không có sản phẩm mới thì không tạo video, được %d job", len(svc2sr.jobs))
	}
	if !strings.Contains(notes[0], "0 mới") || !strings.Contains(notes[0], "0 video") {
		t.Errorf("quét lại phải 0 mới / 0 video: %q", notes[0])
	}
}

// --- order sync ---

func TestATOrderSyncTick(t *testing.T) {
	srv := atTestServer()
	defer srv.Close()
	svc, _ := newATService(t, mapSettings{
		accesstrade.KeySetting: "k",
		KeyATHunterEnabled:     "0",
		KeyATCampaignCheckOn:   "0",
	}, srv)

	notes := svc.ATOrderSyncTick(context.Background())
	if len(notes) != 1 {
		t.Fatalf("muốn 1 note, được %v", notes)
	}
	n := notes[0]
	if !strings.Contains(n, "2 đơn") || !strings.Contains(n, "2 mới") {
		t.Errorf("note sai: %q", n)
	}
	if !strings.Contains(n, "chờ duyệt 1") {
		t.Errorf("note phải báo 1 đơn chờ duyệt: %q", n)
	}
	// Watermark đã ghi.
	if v, ok := svc.AT.GetSyncState(keyATOrdersUntil); !ok || v == "" {
		t.Errorf("thiếu watermark orders.until")
	}
	// Đối soát: approved và pending hiện riêng.
	st, err := svc.AT.GetOrderStats()
	if err != nil {
		t.Fatal(err)
	}
	if st.ApprovedCount != 1 || st.ApprovedTotal != 45000 || st.PendingCount != 1 {
		t.Errorf("stats sai: %+v", st)
	}
	// Cadence: chạy lại ngay → im lặng.
	if notes := svc.ATOrderSyncTick(context.Background()); len(notes) != 0 {
		t.Errorf("chưa đến 30 phút phải im lặng, được %v", notes)
	}
}

// --- campaign check ---

func TestATCampaignCheckTick(t *testing.T) {
	srv := atTestServer()
	defer srv.Close()
	svc, _ := newATService(t, mapSettings{
		accesstrade.KeySetting: "k",
		KeyATHunterEnabled:     "0",
		KeyATOrderSyncEnabled:  "0",
	}, srv)

	notes := svc.ATCampaignCheckTick(context.Background())
	if len(notes) != 1 {
		t.Fatalf("muốn 1 note, được %v", notes)
	}
	if !strings.Contains(notes[0], "chưa duyệt") || !strings.Contains(notes[0], "Bank X") {
		t.Errorf("phải cảnh báo campaign pending: %q", notes[0])
	}
	// Cache campaign đã làm mới.
	cs, err := svc.AT.CachedCampaigns()
	if err != nil || len(cs) != 2 {
		t.Errorf("cache campaign sai: %+v %v", cs, err)
	}
	// Chạy lại ngay → im lặng (daily).
	if notes := svc.ATCampaignCheckTick(context.Background()); len(notes) != 0 {
		t.Errorf("chưa đến hạn phải im lặng, được %v", notes)
	}
}

// --- helpers ---

func TestAtOnDefaults(t *testing.T) {
	if !atOn(nil, "x", true) || atOn(nil, "x", false) {
		t.Errorf("nil settings phải trả default")
	}
	m := mapSettings{}
	if !atOn(m, "x", true) {
		t.Errorf("unset phải mặc định BẬT (zero-touch)")
	}
	m["x"] = "0"
	if atOn(m, "x", true) {
		t.Errorf("\"0\" phải tắt")
	}
	if atInt(m, "n", 3) != 3 || atInt(mapSettings{"n": "abc"}, "n", 3) != 3 {
		t.Errorf("atInt default sai")
	}
	if atInt(mapSettings{"n": "5"}, "n", 3) != 5 {
		t.Errorf("atInt parse sai")
	}
}

func TestThousandsFmt(t *testing.T) {
	if accesstrade.Thousands(45000) != "45.000" || accesstrade.Thousands(999) != "999" {
		t.Errorf("thousands sai: %q", accesstrade.Thousands(45000))
	}
}
