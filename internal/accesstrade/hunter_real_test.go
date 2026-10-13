package accesstrade

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// Quy tắc rút ra khi chạy với key thật (12/10/2026):
//   - chỉ săn merchant của chiến dịch ĐÃ DUYỆT (link chưa duyệt không ra tiền);
//   - bỏ sản phẩm datafeed quá cũ (feed 2023 → ảnh 404 hết);
//   - không dùng "url" gốc của shop làm link affiliate.
func TestHuntApprovedFreshAffiliateOnly(t *testing.T) {
	approved := `[{"id":"c1","name":"Referral","merchant":"referral","approval":"successful"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/campaigns":
			fmt.Fprintf(w, `{"data":%s,"page":1,"total_page":1}`, approved)
		case "/v1/datafeeds":
			if r.URL.Query().Get("merchant") != "shopee" {
				fmt.Fprint(w, `{"data":[],"total":0}`)
				return
			}
			fmt.Fprint(w, `{"data":[
			 {"sku":"new","name":"Mới","price":100000,"aff_link":"https://go.isclix.com/a","merchant":"shopee","update_time":"12-10-2026T01:30:14"},
			 {"sku":"old","name":"Cũ","price":100000,"aff_link":"https://go.isclix.com/b","merchant":"shopee","update_time":"28-02-2023T04:00:07"},
			 {"sku":"raw","name":"Không mã","price":100000,"url":"https://shop.vn/p","merchant":"shopee","update_time":"12-10-2026T01:30:14"}
			]}`)
		}
	}))
	defer srv.Close()
	c := NewClient("k")
	c.BaseURL = srv.URL
	ps, err := products.NewStore(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	f := HunterFilter{ApprovedOnly: true, MaxAgeDays: 180}

	res, err := c.Hunt(context.Background(), ps, f)
	if err != nil || res.New != 0 || !strings.Contains(res.Note, "không có datafeed") {
		t.Fatalf("chỉ duyệt Referral (không có feed) → không săn, có hướng dẫn: %+v %v", res, err)
	}

	approved = `[{"id":"c2","name":"Shopee","merchant":"shopee","approval":"successful"}]`
	res, err = c.Hunt(context.Background(), ps, f)
	if err != nil || res.New != 1 || res.Products[0].SourceID == "" || !strings.Contains(res.Products[0].ProductURL, "isclix") {
		t.Fatalf("chỉ sản phẩm mới + có link affiliate: %+v %v", res, err)
	}
	if res.Skipped != 2 {
		t.Fatalf("phải bỏ 1 cũ + 1 không mã affiliate, skipped=%d", res.Skipped)
	}
}
