package growth

func (s *Store) ListTargets(accountID int64) ([]Target, error) {
	rows, err := s.db.Query(
		`SELECT id, account_id, metric, threshold, label, COALESCE(deadline,''), COALESCE(reached_at,'')
		 FROM growth_targets WHERE account_id = ? ORDER BY metric, threshold`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.AccountID, &t.Metric, &t.Threshold, &t.Label, &t.Deadline, &t.ReachedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkTargetsReached stamps reached_at on every milestone the given metric
// readings satisfy. Returns the labels newly reached (for milestone alerts).
func (s *Store) MarkTargetsReached(accountID int64, readings map[string]float64) ([]string, error) {
	targets, err := s.ListTargets(accountID)
	if err != nil {
		return nil, err
	}
	var reached []string
	for _, t := range targets {
		if t.ReachedAt != "" {
			continue
		}
		v, ok := readings[t.Metric]
		if !ok || v < t.Threshold {
			continue
		}
		if _, err := s.db.Exec(
			`UPDATE growth_targets SET reached_at = ? WHERE id = ?`, nowStr(), t.ID); err != nil {
			return reached, err
		}
		reached = append(reached, t.Label)
	}
	return reached, nil
}

// ----------------------------------------------------------------- plans

// InsertPlan supersedes the account's previous active plan and stores the
// new rolling plan + items (history is preserved, never rewritten).
