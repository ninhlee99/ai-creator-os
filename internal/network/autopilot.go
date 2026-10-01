package network

import (
	"fmt"
)

// Affiliate autopilot policy per account (yêu cầu Ninh 2026-10-01):
// Ninh chọn theme cho từng account; hệ thống tự tìm sản phẩm hoa hồng cao
// đúng theme và tự làm video. Ảnh mẫu do Ninh upload (model library nằm ở
// internal/studio).

// fillAutopilot loads theme/autopilot/min_commission for one account.
func (m *AccountManager) fillAutopilot(a *Account) error {
	var theme string
	var auto int64
	var minComm float64
	err := m.db.QueryRow(
		`SELECT theme, autopilot, min_commission FROM accounts WHERE id = ?`,
		a.ID).Scan(&theme, &auto, &minComm)
	if err != nil {
		return err
	}
	a.Theme = theme
	a.Autopilot = auto != 0
	a.MinCommission = minComm
	return nil
}

// GetWithAutopilot is Get plus the autopilot policy fields.
func (m *AccountManager) GetWithAutopilot(accountID int64) (*Account, error) {
	a, err := m.Get(accountID)
	if err != nil {
		return nil, err
	}
	if err := m.fillAutopilot(a); err != nil {
		return nil, err
	}
	return a, nil
}

// ListWithAutopilot is List plus the autopilot policy fields.
func (m *AccountManager) ListWithAutopilot(statuses ...string) ([]*Account, error) {
	accts, err := m.List(statuses...)
	if err != nil {
		return nil, err
	}
	for _, a := range accts {
		if err := m.fillAutopilot(a); err != nil {
			return nil, err
		}
	}
	return accts, nil
}

// SetTheme assigns the affiliate theme (chủ đề) for an account.
func (m *AccountManager) SetTheme(accountID int64, theme string) (*Account, error) {
	if _, err := m.db.Exec(
		`UPDATE accounts SET theme = ?, updated_at = datetime('now') WHERE id = ?`,
		theme, accountID); err != nil {
		return nil, err
	}
	return m.GetWithAutopilot(accountID)
}

// SetAutopilot toggles hands-off mode and the commission floor (0..1).
func (m *AccountManager) SetAutopilot(accountID int64, enabled bool, minCommission float64) (*Account, error) {
	if minCommission < 0 || minCommission > 1 {
		return nil, fmt.Errorf("min_commission must be 0..1, got %v", minCommission)
	}
	auto := 0
	if enabled {
		auto = 1
	}
	if _, err := m.db.Exec(
		`UPDATE accounts SET autopilot = ?, min_commission = ?, updated_at = datetime('now') WHERE id = ?`,
		auto, minCommission, accountID); err != nil {
		return nil, err
	}
	return m.GetWithAutopilot(accountID)
}

// AutopilotReady reports whether an account can run hands-off: autopilot on,
// theme chosen. Model photos are checked by the studio layer (it owns the
// model library) — see studio.ModelStore.
func AutopilotReady(a *Account) bool {
	return a != nil && a.Autopilot && a.Theme != ""
}
