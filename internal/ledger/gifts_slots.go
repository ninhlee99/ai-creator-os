package ledger

import (
	"database/sql"
)

func (l *Ledger) RecordGift(accountID int64, diamonds int64, usd float64, sessionID *int64) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tx, err := l.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var sess any
	if sessionID != nil {
		sess = *sessionID
	}
	res, err := tx.Exec(
		"INSERT INTO gifts (account_id, session_id, diamonds, usd) VALUES (?,?,?,?)",
		accountID, sess, diamonds, usd)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		"UPDATE accounts SET gift_usd = gift_usd + ? WHERE id = ?", usd, accountID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AccountGiftUSD returns lifetime gift USD summed from the gifts table.
func (l *Ledger) AccountGiftUSD(accountID int64) (float64, error) {
	var s float64
	err := l.db.QueryRow(
		"SELECT COALESCE(SUM(usd),0) AS s FROM gifts WHERE account_id = ?",
		accountID).Scan(&s)
	return s, err
}

// GetGiftSummary returns per-account gift revenue, richest first.
func (l *Ledger) GetGiftSummary() ([]GiftSummary, error) {
	rows, err := l.db.Query(
		"SELECT a.username AS username, COALESCE(SUM(g.usd), 0) AS usd " +
			"FROM accounts a LEFT JOIN gifts g ON g.account_id = a.id " +
			"GROUP BY a.id ORDER BY usd DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GiftSummary
	for rows.Next() {
		var g GiftSummary
		if err := rows.Scan(&g.Username, &g.USD); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ---- live slots ----

// SaveSlots persists scheduler-produced slots.
func (l *Ledger) SaveSlots(slots []Slot) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	tx, err := l.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range slots {
		if _, err := tx.Exec(
			"INSERT INTO live_slots (account_id, slot_date, start_min, duration_min, status) "+
				"VALUES (?,?,?,?,?)",
			s.AccountID, s.SlotDate, s.StartMin, s.DurationMin, s.Status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSlots returns all slots for a date, ordered by start_min.
func (l *Ledger) GetSlots(slotDate string) ([]Slot, error) {
	rows, err := l.db.Query(
		"SELECT id, account_id, slot_date, start_min, duration_min, status, created_at "+
			"FROM live_slots WHERE slot_date = ? ORDER BY start_min", slotDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSlots(rows)
}

// DueSlots returns planned slots for a date whose start time has arrived.
func (l *Ledger) DueSlots(slotDate string, nowMin int) ([]Slot, error) {
	rows, err := l.db.Query(
		"SELECT id, account_id, slot_date, start_min, duration_min, status, created_at "+
			"FROM live_slots WHERE slot_date = ? AND status = 'planned' "+
			"AND start_min <= ? ORDER BY start_min", slotDate, nowMin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSlots(rows)
}

// SetSlotStatus updates a slot's status.
func (l *Ledger) SetSlotStatus(slotID int64, status string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE live_slots SET status = ? WHERE id = ?", status, slotID)
	return err
}

func scanSlots(rows *sql.Rows) ([]Slot, error) {
	var out []Slot
	for rows.Next() {
		var s Slot
		if err := rows.Scan(&s.ID, &s.AccountID, &s.SlotDate, &s.StartMin,
			&s.DurationMin, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- dashboard read helpers ----

// TotalRevenue returns total settled commission revenue.
