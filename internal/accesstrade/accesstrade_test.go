package accesstrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// mockServer dựng server giả kiểm tra header Authorization đúng format
// "Token <key>" và trả JSON theo kịch bản.
func mockServer(t *testing.T, key string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		if got := r.Header.Get("Authorization"); got != "Token "+key {
			t.Errorf("Authorization = %q, muốn %q", got, "Token "+key)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q, muốn application/json", ct)
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListCampaigns(t *testing.T) {
	srv := mockServer(t, "k123", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/campaigns" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"campaign_id":"c1","name":"Shopee","approval":"successful","commission_rate":8.5,"cookie_duration":"7 ngày"},
			{"campaign_id":"c2","name":"Lazada","approval":"pending"},
			{"id":"c3","campaign_name":"Tiki","approval_status":"successful","commission_policy":"8% giá trị đơn"}
		]}`))
	})
	c := NewClient("k123")
	c.BaseURL = srv.URL
	cs, err := c.ListCampaigns(context.Background(), CampaignFilter{})
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("muốn 3 campaign, được %d", len(cs))
	}
	if cs[0].ID != "c1" || cs[0].Name != "Shopee" || cs[0].CommissionRate != 8.5 {
		t.Errorf("campaign[0] parse sai: %+v", cs[0])
	}
	if cs[1].Approval != "pending" {
		t.Errorf("campaign[1].Approval = %q", cs[1].Approval)
	}
	if cs[2].ID != "c3" || cs[2].CommissionPolicy != "8% giá trị đơn" {
		t.Errorf("campaign[2] parse sai: %+v", cs[2])
	}
	ap := ApprovedOnly(cs)
	if len(ap) != 2 {
		t.Fatalf("ApprovedOnly muốn 2, được %d", len(ap))
	}
	if ap[0].ApprovalLabel() != "Đã duyệt" || cs[1].ApprovalLabel() != "Chờ duyệt" {
		t.Errorf("label sai: %q / %q", ap[0].ApprovalLabel(), cs[1].ApprovalLabel())
	}
	if cs[1].CommissionLabel() != "—" {
		t.Errorf("commission trống phải là —, được %q", cs[1].CommissionLabel())
	}
}

func TestListCampaignsRawArray(t *testing.T) {
	srv := mockServer(t, "k", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"campaign_id":"x","name":"X"}]`))
	})
	c := NewClient("k")
	c.BaseURL = srv.URL
	cs, err := c.ListCampaigns(context.Background(), CampaignFilter{Approval: "successful", Limit: 1})
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if len(cs) != 1 || cs[0].ID != "x" {
		t.Fatalf("parse mảng trần sai: %+v", cs)
	}
}

func TestCreateProductLink(t *testing.T) {
	var gotBody string
	srv := mockServer(t, "k123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/product_link/create" {
			t.Errorf("method/path = %s %s", r.Method, r.URL.Path)
		}
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"url":"https://go.accesstrade.vn/abc","short_url":"https://short.vn/abc"}}`))
	})
	c := NewClient("k123")
	c.BaseURL = srv.URL
	l, err := c.CreateProductLink(context.Background(), LinkRequest{
		URL: "https://shopee.vn/sp1", CampaignID: "c1",
		UTM: map[string]string{"source": "tiktok"}, Sub1: "acc1",
	})
	if err != nil {
		t.Fatalf("CreateProductLink: %v", err)
	}
	if l.TrackingLink != "https://go.accesstrade.vn/abc" || l.ShortLink != "https://short.vn/abc" {
		t.Errorf("link sai: %+v", l)
	}
	for _, want := range []string{`"url":"https://shopee.vn/sp1"`, `"campaign_id":"c1"`, `"utm_source":"tiktok"`, `"sub1":"acc1"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body thiếu %s (body=%s)", want, gotBody)
		}
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("k")
	c.BaseURL = srv.URL
	_, err := c.ListCampaigns(context.Background(), CampaignFilter{})
	if err == nil {
		t.Fatal("muốn lỗi 400")
	}
	if n := atomic.LoadInt64(&hits); n != 1 {
		t.Errorf("4xx không được retry, nhưng đã gọi %d lần", n)
	}
}

func TestRetryOn5xx(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&hits, 1) == 1 {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("k")
	c.BaseURL = srv.URL
	cs, err := c.ListCampaigns(context.Background(), CampaignFilter{})
	if err != nil {
		t.Fatalf("retry 5xx phải thành công: %v", err)
	}
	if len(cs) != 0 {
		t.Errorf("muốn 0 campaign, được %d", len(cs))
	}
	if n := atomic.LoadInt64(&hits); n != 2 {
		t.Errorf("muốn 2 lần gọi (1 retry), được %d", n)
	}
}

func TestNoKeyFailClosed(t *testing.T) {
	c := NewClient("")
	if _, err := c.ListCampaigns(context.Background(), CampaignFilter{}); err == nil {
		t.Error("thiếu key phải fail-closed")
	}
	if _, err := c.CreateProductLink(context.Background(), LinkRequest{URL: "x"}); err == nil {
		t.Error("thiếu key phải fail-closed")
	}
	if _, err := c.CreateProductLink(context.Background(), LinkRequest{}); err == nil {
		t.Error("thiếu URL phải báo lỗi")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "accesstrade.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	cs := []Campaign{
		{ID: "c1", Name: "Shopee", Approval: "successful", CommissionRate: 8.5, CookieDuration: "7 ngày"},
		{ID: "c2", Name: "Lazada", Approval: "pending"},
	}
	if err := s.UpsertCampaigns(cs); err != nil {
		t.Fatalf("UpsertCampaigns: %v", err)
	}
	// Upsert lại với tên đổi → phải update tại chỗ, không nhân đôi.
	cs[0].Name = "Shopee VN"
	if err := s.UpsertCampaigns(cs[:1]); err != nil {
		t.Fatalf("UpsertCampaigns lần 2: %v", err)
	}
	got, err := s.CachedCampaigns()
	if err != nil {
		t.Fatalf("CachedCampaigns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("muốn 2 campaign, được %d", len(got))
	}
	if got[1].Name != "Shopee VN" || got[1].CommissionRate != 8.5 {
		t.Errorf("upsert không update tại chỗ: %+v", got[1])
	}

	if err := s.SaveLink("https://shopee.vn/sp1", "c1", "acc1",
		"https://go.accesstrade.vn/abc", "https://short.vn/abc"); err != nil {
		t.Fatalf("SaveLink: %v", err)
	}
	// Lưu lại cùng key → upsert, không nhân đôi.
	if err := s.SaveLink("https://shopee.vn/sp1", "c1", "acc1",
		"https://go.accesstrade.vn/xyz", ""); err != nil {
		t.Fatalf("SaveLink lần 2: %v", err)
	}
	links, err := s.ListLinks(0)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("muốn 1 link, được %d", len(links))
	}
	l := links[0]
	if l.TrackingURL != "https://go.accesstrade.vn/xyz" {
		t.Errorf("upsert link không update: %+v", l)
	}
	if l.Campaign != "Shopee VN" {
		t.Errorf("join tên campaign sai: %+v", l)
	}
}
