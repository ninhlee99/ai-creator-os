package web

// Affiliate product catalog: product search and discovery, manual add,
// the merged shelf tab, autopilot scheduling policy and music upload.

import (
	"context"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"strings"
)

type accesstradeSearchProvider struct {
	client *accesstrade.Client
	store  *products.Store
}

func (p accesstradeSearchProvider) Name() string { return accesstrade.HunterSource }

func (p accesstradeSearchProvider) Search(ctx context.Context, q products.Query) ([]products.Product, error) {
	if _, err := p.client.Hunt(ctx, p.store, accesstrade.HunterFilter{
		Limit: 200, MinCommissionRate: q.MinCommission, ThemeFallback: q.Theme,
		ApprovedOnly: true, MaxAgeDays: 30,
	}); err != nil {
		return nil, err
	}
	items, err := p.store.Search("", q.Theme, q.MinCommission, 500)
	if err != nil {
		return nil, err
	}
	var out []products.Product
	for _, item := range items {
		if len(q.Keywords) > 0 {
			hay := strings.ToLower(item.Title + " " + item.Category + " " + item.ShopName)
			matched := false
			for _, term := range q.Keywords {
				if strings.Contains(hay, strings.ToLower(term)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, item)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}
