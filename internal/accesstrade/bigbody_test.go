package accesstrade

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Danh sách chiến dịch thật ~9MB (vượt trần cũ 8MB) vẫn parse được; API
// trả 200 kèm chuỗi lỗi thì báo lỗi rõ, không coi là danh sách rỗng.
func TestListCampaignsLargeAndStringError(t *testing.T) {
	big := `{"data":[` + strings.Repeat(`{"campaign_id":"1","name":"`+strings.Repeat("x", 9000)+`","approval":"successful","cookie_duration":2592000},`, 1100)
	big = strings.TrimSuffix(big, ",") + `]}`
	if len(big) < 9<<20 {
		t.Fatalf("fixture phải >9MB, got %d", len(big))
	}
	mode := "big"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "big" {
			fmt.Fprint(w, big)
			return
		}
		fmt.Fprint(w, `"{\"message\": \"Max limited is 50\"}"`)
	}))
	defer srv.Close()
	c := NewClient("k")
	c.BaseURL = srv.URL
	cs, err := c.ListCampaigns(context.Background(), CampaignFilter{})
	if err != nil || len(cs) != 1100 || cs[0].CookieDuration != "2592000" {
		t.Fatalf("big: n=%d err=%v", len(cs), err)
	}
	mode = "strerr"
	if _, err := c.ListCampaigns(context.Background(), CampaignFilter{Limit: 100}); err == nil || !strings.Contains(err.Error(), "Max limited is 50") {
		t.Fatalf("chuỗi lỗi phải thành lỗi rõ, got %v", err)
	}
}

// API phân trang bằng page/total_page: đọc hết các trang (trước chỉ đọc
// trang 1 nên mất chiến dịch đã duyệt nằm ở trang sau).
func TestListCampaignsPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
		if p == "" {
			p = "1"
		}
		fmt.Fprintf(w, `{"data":[{"id":"c%s","name":"C%s","approval":"successful"}],"page":%s,"total_page":3.0}`, p, p, p)
	}))
	defer srv.Close()
	c := NewClient("k")
	c.BaseURL = srv.URL
	cs, err := c.ListCampaigns(context.Background(), CampaignFilter{Approval: "successful"})
	if err != nil || len(cs) != 3 || cs[2].ID != "c3" {
		t.Fatalf("phải đọc 3 trang: %+v %v", cs, err)
	}
	one, _ := c.ListCampaigns(context.Background(), CampaignFilter{Limit: 1})
	if len(one) != 1 {
		t.Fatalf("Limit=1 chỉ cần 1 trang, got %d", len(one))
	}
}
