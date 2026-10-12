package growth

import (
	"encoding/json"
	"strings"
)

func (s *Store) InsertAlert(a Alert) error {
	var acct any
	if a.AccountID != 0 {
		acct = a.AccountID
	}
	_, err := s.db.Exec(
		`INSERT INTO growth_alerts (account_id, severity, kind, message, action_taken)
		 VALUES (?, ?, ?, ?, ?)`,
		acct, a.Severity, a.Kind, a.Message, a.ActionTaken)
	return err
}

// ListAlerts returns recent alerts, newest first (accountID 0 = all).
func (s *Store) ListAlerts(accountID int64, limit int) ([]Alert, error) {
	q := `SELECT id, COALESCE(account_id,0), severity, kind, message, COALESCE(action_taken,''), created_at
	      FROM growth_alerts`
	var args []any
	if accountID != 0 {
		q += ` WHERE account_id = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Severity, &a.Kind, &a.Message, &a.ActionTaken, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Decide writes one row into the shared decisions audit trail (same table
// the ledger's Decide writes), so growth decisions are auditable next to
// money decisions.
func (s *Store) Decide(agent, action, target, reason string, inputs map[string]any) error {
	var raw any
	if inputs != nil {
		b, err := json.Marshal(inputs)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := s.db.Exec(
		`INSERT INTO decisions (agent, action, target, reason, inputs_json) VALUES (?, ?, ?, ?, ?)`,
		agent, action, target, reason, raw)
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

var _ = strings.TrimSpace // keep strings import if helpers shrink
