package web

import (
	"log"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

const (
	productSalesMinSampleGap = 6 * time.Hour
	productSalesMaxSampleGap = 14 * 24 * time.Hour
)

// attachProductSalesChange adds an observed units_sold delta only when two
// recent product snapshots support a time comparison.
func (s *Server) attachProductSalesChange(items []products.Product) []products.Product {
	if s == nil || s.Products == nil {
		return items
	}
	now := time.Now()
	for i := range items {
		if !items[i].SoldCountKnown {
			continue
		}
		history, err := s.Products.History(items[i].ID, 2)
		if err != nil {
			log.Printf("web: product %d sales snapshots: %v", items[i].ID, err)
			continue
		}
		if len(history.Snapshots) < 2 {
			continue
		}
		latest, previous := history.Snapshots[0], history.Snapshots[1]
		if !latest.Product.SoldCountKnown || !previous.Product.SoldCountKnown {
			continue
		}
		latestAt, latestErr := time.Parse(time.RFC3339Nano, latest.CapturedAt)
		previousAt, previousErr := time.Parse(time.RFC3339Nano, previous.CapturedAt)
		if latestErr != nil || previousErr != nil {
			continue
		}
		elapsed := latestAt.Sub(previousAt)
		if elapsed < productSalesMinSampleGap || elapsed > productSalesMaxSampleGap ||
			latestAt.After(now.Add(5*time.Minute)) || now.Sub(latestAt) > productSalesMaxSampleGap {
			continue
		}
		items[i].SalesChangeMeasured = true
		items[i].SalesUnitsChange = latest.Product.SoldCount - previous.Product.SoldCount
		items[i].SalesChangeDays = elapsed.Hours() / 24
	}
	return items
}
