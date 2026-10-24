package accesstrade

import (
	"fmt"
	"strings"
)

// OrderAttributionSummary is the AccessTrade order reconciliation for one
// exact sub2 value. Commission includes approved orders only.
type OrderAttributionSummary struct {
	TotalCount         int64
	PendingCount       int64
	ApprovedCount      int64
	RejectedCount      int64
	ApprovedCommission float64
}

// OrderAttributionBySub2 aggregates saved orders attributed to one video job.
func (s *Store) OrderAttributionBySub2(sub2 string) (OrderAttributionSummary, error) {
	var summary OrderAttributionSummary
	sub2 = strings.TrimSpace(sub2)
	if sub2 == "" {
		return summary, fmt.Errorf("accesstrade: thiếu sub2 để đối chiếu đơn")
	}
	err := s.db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN status=? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status=? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status=? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status=? THEN pub_commission ELSE 0 END),0)
		FROM at_orders WHERE sub2=?`,
		OrderPending, OrderApproved, OrderRejected, OrderApproved, sub2).Scan(
		&summary.TotalCount, &summary.PendingCount, &summary.ApprovedCount,
		&summary.RejectedCount, &summary.ApprovedCommission)
	if err != nil {
		return OrderAttributionSummary{}, fmt.Errorf("accesstrade: đối chiếu đơn theo sub2: %w", err)
	}
	return summary, nil
}
