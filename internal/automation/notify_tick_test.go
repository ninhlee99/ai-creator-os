package automation

// Đợt O2: NotifyTick gửi alert warn+ qua Telegram.

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
)

func notifyTestService(t *testing.T) (*Service, *growth.Store) {
	t.Helper()
	l, _ := openTestLedger(t)
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "growth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	gs, err := growth.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	svc.Growth = gs
	return svc, gs
}

func TestNotifyTickOffByDefault(t *testing.T) {
	svc, gs := notifyTestService(t)
	_ = gs.InsertAlert(growth.Alert{Severity: "warn", Kind: "disk_guard", Message: "đầy"})
	if notes := svc.NotifyTick(context.Background()); len(notes) != 0 {
		t.Fatalf("mặc định tắt → im lặng, got %q", notes)
	}
}

func TestNotifyTickNeedsCredentials(t *testing.T) {
	svc, gs := notifyTestService(t)
	_ = svc.Settings.Set(KeyNotifyTelegramOn, "1")
	_ = gs.InsertAlert(growth.Alert{Severity: "warn", Kind: "x", Message: "y"})
	if notes := svc.NotifyTick(context.Background()); len(notes) != 0 {
		t.Fatalf("thiếu token/chat → im lặng, got %q", notes)
	}
}

func TestNotifyTickRespectsKillAndDryRun(t *testing.T) {
	svc, gs := notifyTestService(t)
	_ = svc.Settings.Set(KeyNotifyTelegramOn, "1")
	_ = svc.Settings.Set(KeyNotifyTelegramBot, "b")
	_ = svc.Settings.Set(KeyNotifyTelegramChat, "c")
	_ = gs.InsertAlert(growth.Alert{Severity: "warn", Kind: "x", Message: "y"})
	svc.Gate = &fakeGate{kill: true}
	if notes := svc.NotifyTick(context.Background()); len(notes) != 0 {
		t.Fatalf("kill phải chặn, got %q", notes)
	}
	svc.Gate = &fakeGate{dry: true}
	if notes := svc.NotifyTick(context.Background()); len(notes) != 0 {
		t.Fatalf("dry-run phải chặn, got %q", notes)
	}
}
