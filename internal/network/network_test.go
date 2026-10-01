package network

import (
	"path/filepath"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// newTestManager builds a ledger + account manager on a temp SQLite file.
func newTestManager(t *testing.T) (*ledger.Ledger, *AccountManager) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	l, err := ledger.New(path)
	if err != nil {
		t.Fatalf("ledger.New: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	mgr, err := NewAccountManager(l, path)
	if err != nil {
		t.Fatalf("NewAccountManager: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })
	return l, mgr
}

// walkChain moves an account through statuses, failing the test on error.
func walkChain(t *testing.T, mgr *AccountManager, a *Account, statuses ...string) *Account {
	t.Helper()
	var err error
	for _, s := range statuses {
		a, err = mgr.Transition(a.ID, s, nil)
		if err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	return a
}
