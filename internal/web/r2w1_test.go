package web

// R2-W1 — "Một kho sản phẩm + writer tiền thật" (đóng R2-02, R2-03):
// tab Kệ hàng ghi vào kho chung nên autopilot thấy được; reconcile tiền
// chạy trong vòng sync với payload thật (fail-closed khi không có nguồn).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// errFakeOrders simulates an affiliate API outage.
var errFakeOrders = errors.New("simulated affiliate API outage")

// ------------------------------------------------------------------ kho 1

// TestShelfAddVisibleToAutopilot — thêm sản phẩm qua đường Kệ hàng
// (handler POST) → cùng kho store: TopByTheme của autopilot thấy nó,
// tab Kệ hàng hiển thị nó, /api/products đọc kho chung.
func TestShelfAddVisibleToAutopilot(t *testing.T) {
	s := newTestServer(t)
	store, err := products.NewStore("products-r2w1.db")
	if err != nil {
		t.Fatalf("products store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	s.Products = store

	rec := postForm(t, s, "/products/shelf/add", url.Values{
		"platform_pid":    {"TT-999"},
		"title":           {"Tai nghe X10"},
		"price":           {"50"},
		"commission_rate": {"0.2"},
		"category":        {"điện tử"},
		"theme":           {"cong-nghe"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("shelf add = %d, want 303", rec.Code)
	}

	// The same method studio.Autopilot.Run uses to pick products.
	top, err := store.TopByTheme("cong-nghe", 0.10, 42, 5)
	if err != nil {
		t.Fatalf("TopByTheme: %v", err)
	}
	if len(top) != 1 || top[0].SourceID != "TT-999" {
		t.Fatalf("TopByTheme = %v, want the shelf-added TT-999", top)
	}
	if top[0].ShelfStatus != "shelf" {
		t.Errorf("ShelfStatus = %q, want shelf", top[0].ShelfStatus)
	}

	// Shelf tab renders it from the shared store.
	rec = get(t, s, "/products?tab=ke")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /products?tab=ke = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Tai nghe X10") {
		t.Error("shelf tab body does not show the added product")
	}

	// /api/products reads the shared store too (R2-W1). The legacy API is
	// gated (R2-W7, default OFF): enable it first like an operator would.
	if err := automation.SetAPIEnabled(s.settings(), true); err != nil {
		t.Fatalf("enable api: %v", err)
	}
	rec = get(t, s, "/api/products")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/products = %d, want 200", rec.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("api products json: %v", err)
	}
	found := false
	for _, it := range items {
		if it["platform_pid"] == "TT-999" && it["status"] == "shelf" {
			found = true
		}
	}
	if !found {
		t.Errorf("api products does not list TT-999 as shelf: %v", items)
	}
}

// TestShelfMigrationLegacyLedger — hàng cũ trong bảng ledger products
// được nhập một lần, idempotent, sang kho chung.
func TestShelfMigrationLegacyLedger(t *testing.T) {
	s := newTestServer(t)
	store, err := products.NewStore("products-r2w1-mig.db")
	if err != nil {
		t.Fatalf("products store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	s.Products = store

	// Legacy shelf row + a non-shelf row, written the old way.
	if _, err := s.Ledger.AddProduct("TT-OLD1", "Váy hoa", 30, 0.15, "thời trang"); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if _, err := s.Ledger.UpsertProduct("TT-CAND", map[string]any{
		"title": "Bình giữ nhiệt", "price": 20.0, "commission_rate": 0.1,
		"commission_value": 2.0, "status": "candidate",
	}); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}

	n, err := s.MigrateLedgerShelf()
	if err != nil {
		t.Fatalf("MigrateLedgerShelf: %v", err)
	}
	if n != 2 {
		t.Errorf("migrated = %d, want 2", n)
	}

	shelf, err := store.Shelf(50)
	if err != nil {
		t.Fatalf("Shelf: %v", err)
	}
	if len(shelf) != 1 || shelf[0].SourceID != "TT-OLD1" || shelf[0].ShelfStatus != "shelf" {
		t.Errorf("shelf after migration = %+v, want only TT-OLD1", shelf)
	}
	if n2, err := s.MigrateLedgerShelf(); err != nil || n2 != 0 {
		t.Errorf("second migration = (%d, %v), want (0, nil) idempotent", n2, err)
	}
	if n3, _ := store.Count(); n3 != 2 {
		t.Errorf("store count = %d, want 2 (no duplicates)", n3)
	}
}

// -------------------------------------------------------------- reconcile

type fakeOrdersSource struct {
	data  map[string]any
	err   error
	calls int
}

func (f *fakeOrdersSource) AffiliateOrders(startTS, endTS int64, page, pageSize int) (map[string]any, error) {
	f.calls++
	return f.data, f.err
}

func ordersPayload(pairs ...any) map[string]any {
	out := make([]any, 0, len(pairs)/3)
	for i := 0; i+2 < len(pairs); i += 3 {
		out = append(out, map[string]any{
			"order_id":   pairs[i],
			"amount":     pairs[i+1],
			"commission": pairs[i+2],
			"ordered_at": "2026-10-02T10:00:00",
		})
	}
	return map[string]any{"orders": out}
}

// TestReconcileWritesRealMoney — payload giả lập ở biên client (test
// double) → orders có dòng, commissions có dòng, Trang chủ đổi số; chạy
// lại không ghi trùng.
func TestReconcileWritesRealMoney(t *testing.T) {
	s := newTestServer(t)
	s.CommissionSource = &fakeOrdersSource{data: ordersPayload(
		"O1", 500000.0, 100000.0,
		"O2", 200000.0, 23456.0,
	)}

	notes := s.reconcileCommissions(context.Background())
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want 1 money note", notes)
	}

	rev, err := s.Ledger.TotalRevenue()
	if err != nil {
		t.Fatalf("TotalRevenue: %v", err)
	}
	if rev != 123456 {
		t.Errorf("TotalRevenue = %v, want 123456", rev)
	}

	// Dashboard money cards move from 0.
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "123.456 ₫") {
		t.Error("dashboard does not show the reconciled 123.456 ₫")
	}

	// Idempotent: second run writes nothing twice.
	s.reconcileCommissions(context.Background())
	if rev2, _ := s.Ledger.TotalRevenue(); rev2 != 123456 {
		t.Errorf("after rerun TotalRevenue = %v, want still 123456", rev2)
	}
	if got := s.scalarFloat("SELECT COUNT(*) FROM orders"); got != 2 {
		t.Errorf("orders = %v, want 2", got)
	}
}

// TestReconcileFailClosedNoSource — TikTok Shop đã loại bỏ (Đợt H1) →
// không ghi số nào, ghi 1 quyết định lý do trung thực, lặp lại không spam.
func TestReconcileFailClosedNoSource(t *testing.T) {
	s := newTestServer(t)

	if notes := s.reconcileCommissions(context.Background()); len(notes) != 0 {
		t.Errorf("notes = %v, want none (fail-closed)", notes)
	}
	if rev, _ := s.Ledger.TotalRevenue(); rev != 0 {
		t.Errorf("TotalRevenue = %v, want 0 (no fabricated money)", rev)
	}
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions " +
			"WHERE action='commission_reconcile' ORDER BY id DESC")
	if err != nil {
		t.Fatalf("query decisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want exactly 1", len(decisions))
	}
	if !strings.Contains(decisions[0].Reason, "TikTok Shop đã loại bỏ") {
		t.Errorf("decision reason = %q, want the honest no-source reason", decisions[0].Reason)
	}

	// State-change only: a second tick adds no duplicate decision.
	s.reconcileCommissions(context.Background())
	if again, _ := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions " +
			"WHERE action='commission_reconcile' ORDER BY id DESC"); len(again) != 1 {
		t.Errorf("decisions after rerun = %d, want still 1", len(again))
	}
}

// TestReconcileErrorKeepsHonest — nguồn lỗi → không ghi, ghi lý do lỗi.
func TestReconcileErrorKeepsHonest(t *testing.T) {
	s := newTestServer(t)
	s.CommissionSource = &fakeOrdersSource{err: errFakeOrders}

	s.reconcileCommissions(context.Background())
	if rev, _ := s.Ledger.TotalRevenue(); rev != 0 {
		t.Errorf("TotalRevenue = %v, want 0 after source error", rev)
	}
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions " +
			"WHERE action='commission_reconcile' ORDER BY id DESC")
	if err != nil || len(decisions) != 1 {
		t.Fatalf("decisions = (%d, %v), want 1", len(decisions), err)
	}
	if !strings.Contains(decisions[0].Reason, "lỗi") {
		t.Errorf("decision reason = %q, want the error reason", decisions[0].Reason)
	}
}

// TestReconcileEmptyPayload — nguồn có nhưng không đơn nào → giữ nguyên,
// không quyết định ồn ào.
func TestReconcileEmptyPayload(t *testing.T) {
	s := newTestServer(t)
	s.CommissionSource = &fakeOrdersSource{data: map[string]any{}}

	if notes := s.reconcileCommissions(context.Background()); len(notes) != 0 {
		t.Errorf("notes = %v, want none for empty payload", notes)
	}
	if rev, _ := s.Ledger.TotalRevenue(); rev != 0 {
		t.Errorf("TotalRevenue = %v, want 0", rev)
	}
	decisions, err := s.queryDecisions(
		"SELECT id FROM decisions WHERE action='commission_reconcile'")
	if err != nil || len(decisions) != 0 {
		t.Errorf("decisions = (%d, %v), want none for empty payload", len(decisions), err)
	}
}
