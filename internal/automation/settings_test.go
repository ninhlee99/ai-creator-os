package automation

import (
	"path/filepath"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

func openTestLedger(t *testing.T) (*ledger.Ledger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	l, err := ledger.New(path)
	if err != nil {
		t.Fatalf("ledger.New: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l, path
}

// TestMasterSwitchDefaultsOff — công tắc chính mặc định TẮT khi chưa ai
// đặt (R2-08: không hàm ý đang chạy).
func TestMasterSwitchDefaultsOff(t *testing.T) {
	l, _ := openTestLedger(t)
	if MasterOn(LedgerSettings{L: l}) {
		t.Error("MasterOn = true on a fresh ledger, want OFF by default")
	}
	// A nil ledger can never read "on" either.
	if MasterOn(LedgerSettings{}) {
		t.Error("MasterOn = true with nil ledger, want OFF (fail-closed)")
	}
}

// TestMasterSwitchPersistsAcrossRestart — đổi MASTER_SWITCH → restart →
// giữ nguyên (nghiệm thu R2-W4 bắt buộc).
func TestMasterSwitchPersistsAcrossRestart(t *testing.T) {
	l, path := openTestLedger(t)
	st := LedgerSettings{L: l}
	if err := SetMasterOn(st, true); err != nil {
		t.Fatalf("SetMasterOn: %v", err)
	}
	l.Close() // simulate shutdown

	l2, err := ledger.New(path) // restart: reopen the same database
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { l2.Close() })
	if !MasterOn(LedgerSettings{L: l2}) {
		t.Error("MasterOn = false after restart, want the persisted ON")
	}
	if err := SetMasterOn(LedgerSettings{L: l2}, false); err != nil {
		t.Fatalf("SetMasterOn(false): %v", err)
	}
	l2.Close()

	l3, err := ledger.New(path)
	if err != nil {
		t.Fatalf("reopen 2: %v", err)
	}
	t.Cleanup(func() { l3.Close() })
	if MasterOn(LedgerSettings{L: l3}) {
		t.Error("MasterOn = true after restart, want the persisted OFF")
	}
}

// TestSeedMasterSwitchOnlyOnce — env chỉ gieo một lần; lựa chọn đã lưu
// của operator luôn thắng.
func TestSeedMasterSwitchOnlyOnce(t *testing.T) {
	l, _ := openTestLedger(t)
	st := LedgerSettings{L: l}
	if err := SeedMasterSwitch(st, true); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !MasterOn(st) {
		t.Error("seed from env=true did not land")
	}
	if err := SetMasterOn(st, false); err != nil {
		t.Fatalf("off: %v", err)
	}
	if err := SeedMasterSwitch(st, true); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if MasterOn(st) {
		t.Error("reseed overwrote the stored OFF — a stored choice must always win")
	}
}

// TestMasterSwitchGate — cổng daemon đọc live từ facade; nil = TẮT.
func TestMasterSwitchGate(t *testing.T) {
	l, _ := openTestLedger(t)
	st := LedgerSettings{L: l}
	gate := MasterSwitchGate{Settings: st}
	if gate.MasterEnabled() {
		t.Error("gate = true before any set, want OFF")
	}
	if err := SetMasterOn(st, true); err != nil {
		t.Fatal(err)
	}
	if !gate.MasterEnabled() {
		t.Error("gate = false after SetMasterOn(true), want live read")
	}
	if (MasterSwitchGate{}).MasterEnabled() {
		t.Error("nil-settings gate = true, want fail-closed OFF")
	}
}

// TestMigrateProductsSettings — di trú một lần từ products.db, giá trị
// ledger đã có luôn thắng, marker chặn chạy lại.
func TestMigrateProductsSettings(t *testing.T) {
	l, _ := openTestLedger(t)
	st := LedgerSettings{L: l}
	pdir := t.TempDir()
	ps, err := products.NewStore(filepath.Join(pdir, "products.db"))
	if err != nil {
		t.Fatalf("products.NewStore: %v", err)
	}
	t.Cleanup(func() { ps.Close() })
	if err := ps.SetSetting("autopilot_enabled", "0"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetSetting("autopilot_interval_hours", "12"); err != nil {
		t.Fatal(err)
	}

	if err := MigrateProductsSettings(ps, st); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if v, ok := st.Get("autopilot_enabled"); !ok || v != "0" {
		t.Errorf("autopilot_enabled = %q, want migrated 0", v)
	}
	if v, ok := st.Get("autopilot_interval_hours"); !ok || v != "12" {
		t.Errorf("autopilot_interval_hours = %q, want migrated 12", v)
	}
	// Second run must not overwrite a newer ledger value.
	if err := st.Set("autopilot_interval_hours", "3"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetSetting("autopilot_interval_hours", "24"); err != nil {
		t.Fatal(err)
	}
	if err := MigrateProductsSettings(ps, st); err != nil {
		t.Fatalf("migrate 2: %v", err)
	}
	if v, _ := st.Get("autopilot_interval_hours"); v != "3" {
		t.Errorf("autopilot_interval_hours = %q after re-migrate, want ledger's 3", v)
	}
	if _, ok := st.Get(KeyProductsSettingsMigrated); !ok {
		t.Error("migration marker missing")
	}
}

// TestAutopilotSwitchDefaultsOn — unset = ON, stored 0 = OFF (Đợt 3).
func TestAutopilotSwitchDefaultsOn(t *testing.T) {
	l, _ := openTestLedger(t)
	st := LedgerSettings{L: l}
	if !AutopilotEnabled(st) || !AutopilotAutoPublish(st) {
		t.Error("unset switches: want both ON by default")
	}
	if err := st.Set("autopilot_enabled", "0"); err != nil {
		t.Fatal(err)
	}
	if AutopilotEnabled(st) {
		t.Error("stored 0: AutopilotEnabled = true, want false")
	}
	if !AutopilotAutoPublish(st) {
		t.Error("autoPublish must stay ON independently")
	}
	if n := AutopilotIntervalHours(st); n != 6 {
		t.Errorf("default interval = %d, want 6", n)
	}
}

// TestLoadThresholds — mặc định khi chưa lưu, JSON hỏng không sập,
// overlay từng trường.
func TestLoadThresholds(t *testing.T) {
	l, _ := openTestLedger(t)
	st := LedgerSettings{L: l}
	def := growth.DefaultConfig()
	if got := LoadThresholds(st); got != def {
		t.Errorf("unset: got %+v, want defaults", got)
	}
	if err := st.Set(KeyGrowthThresholds, "{khong-phai-json"); err != nil {
		t.Fatal(err)
	}
	if got := LoadThresholds(st); got != def {
		t.Errorf("corrupt JSON: got %+v, want defaults (fail-safe)", got)
	}
	partial := `{"kill_min_videos":5,"kill_min_days":10}`
	if err := st.Set(KeyGrowthThresholds, partial); err != nil {
		t.Fatal(err)
	}
	got := LoadThresholds(st)
	if got.KillMinVideos != 5 || got.KillMinDays != 10 {
		t.Errorf("partial overlay: kill=%d/%d, want 5/10", got.KillMinVideos, got.KillMinDays)
	}
	if got.DoubleDownFactor != def.DoubleDownFactor {
		t.Errorf("partial overlay: double_down=%v, want default %v",
			got.DoubleDownFactor, def.DoubleDownFactor)
	}
	// Zero values in a full save must not clobber defaults.
	zero := `{"kill_min_videos":0,"double_down_factor":0}`
	if err := st.Set(KeyGrowthThresholds, zero); err != nil {
		t.Fatal(err)
	}
	got = LoadThresholds(st)
	if got.KillMinVideos != def.KillMinVideos || got.DoubleDownFactor != def.DoubleDownFactor {
		t.Errorf("zero overlay: got %+v, want defaults preserved", got)
	}
	if err := ResetThresholds(st); err != nil {
		t.Fatal(err)
	}
	if got := LoadThresholds(st); got != def {
		t.Errorf("after reset: got %+v, want defaults", got)
	}
}

// TestAPIBudgetUSD — override UI thắng env; giá trị hỏng → env.
func TestAPIBudgetUSD(t *testing.T) {
	l, _ := openTestLedger(t)
	st := LedgerSettings{L: l}
	if got := APIBudgetUSD(st, 5.0); got != 5.0 {
		t.Errorf("unset: got %v, want env default 5.0", got)
	}
	if err := SetAPIBudgetUSD(st, 7.5); err != nil {
		t.Fatal(err)
	}
	if got := APIBudgetUSD(st, 5.0); got != 7.5 {
		t.Errorf("set: got %v, want 7.5", got)
	}
	if err := st.Set(KeyAPIBudgetUSD, "khong-phai-so"); err != nil {
		t.Fatal(err)
	}
	if got := APIBudgetUSD(st, 5.0); got != 5.0 {
		t.Errorf("corrupt: got %v, want env fallback 5.0", got)
	}
	if got := APIBudgetUSD(nil, 5.0); got != 5.0 {
		t.Errorf("nil settings: got %v, want env fallback 5.0", got)
	}
}
