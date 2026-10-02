package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"
)

// Trạng thái đơn hàng theo spec /v1/order-list.
const (
	OrderPending  = 0
	OrderApproved = 1
	OrderRejected = 2
)

// orderRatePerMinute là giới hạn nội bộ cho /v1/order-list (spec: 10 req/phút).
const orderRatePerMinute = 10

// Order là một đơn hàng affiliate từ /v1/order-list.
type Order struct {
	// OrderID mã đơn (PK để upsert).
	OrderID string
	// CampaignID/CampaignName chiến dịch ghi nhận đơn.
	CampaignID   string
	CampaignName string
	// Status 0=pending, 1=approved, 2=rejected.
	Status int
	// PubCommission hoa hồng publisher (đơn pending thường = 0, sync sau
	// mới điền số — sync phải UPDATE tại chỗ, không đóng băng ở 0).
	PubCommission float64
	// UTMSource/UTMMedium/Sub1..Sub4 attribution của đơn.
	UTMSource string
	UTMMedium string
	Sub1      string
	Sub2      string
	Sub3      string
	Sub4      string
	// OrderedAt thời điểm đặt đơn (ISO từ API, có thể trống).
	OrderedAt string
	// Raw JSON gốc để debug khi shape đổi.
	Raw json.RawMessage
}

// StatusLabel nhãn tiếng Việt cho trạng thái đơn.
func (o Order) StatusLabel() string {
	switch o.Status {
	case OrderApproved:
		return "Đã duyệt"
	case OrderRejected:
		return "Từ chối"
	default:
		return "Chờ duyệt"
	}
}

// CommissionLabel hiển thị hoa hồng trung thực: đơn chờ duyệt chưa có số
// thì hiện "—" (không cộng vào "đã nhận").
func (o Order) CommissionLabel() string {
	if o.PubCommission > 0 {
		return fmt.Sprintf("%s₫", Thousands(int64(o.PubCommission)))
	}
	return "—"
}

// rateLimiter là token bucket đơn giản: tối đa perMinute lần gọi trong
// cửa sổ 60 giây trượt. now/sleep inject được để test không phải chờ thật.
type rateLimiter struct {
	perMinute int
	mu        sync.Mutex
	hits      []time.Time
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
}

// newOrderLimiter dựng limiter cho /v1/order-list (10 req/phút).
func newOrderLimiter() *rateLimiter {
	return &rateLimiter{
		perMinute: orderRatePerMinute,
		now:       time.Now,
		sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

// wait chặn cho tới khi được phép gọi tiếp (hoặc ctx hủy).
func (l *rateLimiter) wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := l.now()
		cutoff := now.Add(-time.Minute)
		kept := l.hits[:0]
		for _, h := range l.hits {
			if h.After(cutoff) {
				kept = append(kept, h)
			}
		}
		l.hits = kept
		if len(l.hits) < l.perMinute {
			l.hits = append(l.hits, now)
			l.mu.Unlock()
			return nil
		}
		waitFor := l.hits[0].Add(time.Minute).Sub(now)
		if waitFor < 0 {
			waitFor = 0
		}
		l.mu.Unlock()
		if err := l.sleep(ctx, waitFor); err != nil {
			return err
		}
	}
}

// ListOrders gọi GET /v1/order-list trong khoảng since..until (ISO).
// Tự theo phân trang (nếu API trả) tối đa 10 trang; MỖI request đều qua
// rate limiter nội bộ 10 req/phút. Không gọi khi chưa có key.
func (c *Client) ListOrders(ctx context.Context, since, until time.Time) ([]Order, error) {
	if c.orderLimit == nil {
		c.orderLimit = newOrderLimiter()
	}
	var out []Order
	pageToken := ""
	for page := 0; page < 10; page++ {
		q := url.Values{}
		if !since.IsZero() {
			q.Set("since", since.UTC().Format(time.RFC3339))
		}
		if !until.IsZero() {
			q.Set("until", until.UTC().Format(time.RFC3339))
		}
		q.Set("limit", "200")
		if pageToken != "" {
			q.Set("page_token", pageToken)
		}
		if err := c.orderLimit.wait(ctx); err != nil {
			return out, fmt.Errorf("accesstrade: rate limit bị hủy: %w", err)
		}
		data, err := c.do(ctx, "GET", "/v1/order-list?"+q.Encode(), nil)
		if err != nil {
			return out, err
		}
		orders, next, err := parseOrderPage(data)
		if err != nil {
			return out, err
		}
		out = append(out, orders...)
		if next == "" {
			break
		}
		pageToken = next
	}
	return out, nil
}

// parseOrderPage tách danh sách đơn + token trang tiếp theo khỏi response.
func parseOrderPage(data []byte) ([]Order, string, error) {
	raws, err := decodeList(data, "data", "orders", "order_list", "results", "items")
	if err != nil {
		return nil, "", err
	}
	out := make([]Order, 0, len(raws))
	for _, raw := range raws {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		o := Order{
			OrderID:      firstString(m, "order_id", "orderId", "id", "transaction_id"),
			CampaignID:   firstString(m, "campaign_id", "campaignId"),
			CampaignName: firstString(m, "campaign_name", "campaignName", "advertiser"),
			Status:       orderStatus(firstString(m, "status")),
			PubCommission: firstFloat(m, "pub_commission", "commission", "publisher_commission",
				"amount", "payout"),
			UTMSource: firstString(m, "utm_source"),
			UTMMedium: firstString(m, "utm_medium"),
			Sub1:      firstString(m, "sub1"),
			Sub2:      firstString(m, "sub2"),
			Sub3:      firstString(m, "sub3"),
			Sub4:      firstString(m, "sub4"),
			OrderedAt: firstString(m, "ordered_at", "created_at", "date", "time"),
			Raw:       raw,
		}
		if o.OrderID == "" {
			continue
		}
		out = append(out, o)
	}
	var env map[string]json.RawMessage
	next := ""
	if err := json.Unmarshal(data, &env); err == nil {
		next = firstString(env, "next_page_token", "page_token", "nextPageToken")
		if next == "" {
			if hm := firstString(env, "has_more", "hasMore"); hm == "true" || hm == "1" {
				// API báo còn trang nhưng không đưa token → dừng để
				// tránh lặp vô hạn (trung thực hơn đoán số trang).
				next = ""
			}
		}
	}
	return out, next, nil
}

// orderStatus chuẩn hoá trạng thái về 0/1/2 (mặc định pending khi lạ).
func orderStatus(s string) int {
	switch s {
	case "1", "approved", "success", "successful", "confirmed":
		return OrderApproved
	case "2", "rejected", "cancelled", "canceled", "failed":
		return OrderRejected
	default:
		return OrderPending
	}
}
