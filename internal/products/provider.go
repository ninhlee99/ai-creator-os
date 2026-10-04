package products

import (
	"context"
	"fmt"
)

// Provider finds affiliate products for a theme.
type Provider interface {
	// Name is the stable source tag stored on products.
	Name() string
	// Search returns candidates for q. It must honor q.Limit when > 0.
	Search(ctx context.Context, q Query) ([]Product, error)
}

// NOTE (Đợt H1, 2026-10-04): TikTok Shop provider đã loại bỏ hoàn toàn.
// Lý do: endpoint chưa từng được verify, production cần app duyệt + creator
// OAuth (quy trình hành chính mà Ninh đã từ chối 2026-10-01), và PIVOT_REDESIGN
// §2.2 chốt Accesstrade datafeed là nguồn sản phẩm duy nhất. Hunter
// Accesstrade ghi thẳng vào products.Store; autopilot đọc store trước,
// providers chỉ là fallback (hiện để trống).

// ---------------------------------------------------------------------------
// aggregation
// ---------------------------------------------------------------------------

// Aggregate queries every provider, merges, filters, ranks. A provider that
// errors is skipped but reported — one dead source never kills discovery.
func Aggregate(ctx context.Context, providers []Provider, q Query) ([]Product, []error) {
	var all []Product
	var errs []error
	seen := map[string]bool{}
	for _, pv := range providers {
		ps, err := pv.Search(ctx, q)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pv.Name(), err))
			continue
		}
		for _, p := range ps {
			if p.Source == "" {
				p.Source = pv.Name()
			}
			key := p.Source + "\x00" + p.SourceID
			if p.SourceID == "" || seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, p)
		}
	}
	return Rank(Filter(all, q)), errs
}
