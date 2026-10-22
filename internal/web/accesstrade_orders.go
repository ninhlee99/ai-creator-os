package web

import (
	"context"
	"log"
	"net/http"
	"time"
)

func (s *Server) handleATOrderSync(w http.ResponseWriter, r *http.Request) {
	cl, err := s.atClient()
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	until := time.Now()
	since := until.Add(-7 * 24 * time.Hour)
	orders, truncated, err := cl.ListOrders(ctx, since, until)
	if err != nil {
		writeJSONErr(w, "Đồng bộ đơn thất bại: "+err.Error(), http.StatusBadGateway)
		return
	}
	if truncated {
		log.Printf("accesstrade: CẢNH BÁO — đồng bộ tay bị cắt ở 10 trang (2000 đơn), một số đơn có thể chưa về.")
	}
	added, updated, uerr := s.AT.UpsertOrders(orders)
	if uerr != nil {
		writeJSONErr(w, "Lưu đơn thất bại: "+uerr.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.AT.SetSyncState("orders.until", until.UTC().Format(time.RFC3339))
	if s.Ledger != nil {
		_ = s.Ledger.SetSetting("at.ordersync_last_run", time.Now().Format(time.RFC3339))
	}
	st, _ := s.AT.GetOrderStats()
	writeJSON(w, map[string]any{
		"ok": true, "total": len(orders), "added": added, "updated": updated,
		"approved_count": st.ApprovedCount, "approved_total": st.ApprovedTotal,
		"pending_count": st.PendingCount, "pending_total": st.PendingTotal,
		"rejected_count": st.RejectedCount, "truncated": truncated,
	})
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
