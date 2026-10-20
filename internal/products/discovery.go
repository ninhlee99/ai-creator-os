package products

import (
	"context"
	"errors"
)

// MarketplaceDiscoveryProvider is retained for compatibility with callers,
// but no live marketplace API is configured by the application.
type MarketplaceDiscoveryProvider struct{}

func NewMarketplaceDiscoveryProvider() *MarketplaceDiscoveryProvider {
	return &MarketplaceDiscoveryProvider{}
}

func (m *MarketplaceDiscoveryProvider) Name() string { return "marketplace-unconfigured" }

func (m *MarketplaceDiscoveryProvider) Search(context.Context, Query) ([]Product, error) {
	return nil, errors.New("không có API marketplace trực tiếp được cấu hình")
}

// RealMarketplaceCatalog is intentionally empty. Previous hardcoded entries
// looked like verified offers but did not come from any marketplace.
func RealMarketplaceCatalog() []Product { return nil }

// SeedDefaultStore no longer seeds fabricated marketplace offers.
func SeedDefaultStore(*Store) (int, error) { return 0, nil }
