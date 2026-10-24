package ledger

func (l *Ledger) TotalRevenue() (float64, error) {
	var r float64
	err := l.db.QueryRow(
		"SELECT COALESCE(SUM(amount), 0) FROM commissions").Scan(&r)
	return r, err
}

// GetUsage returns api_usage rows for a day (YYYY-MM-DD), newest first.
func (l *Ledger) GetUsage(day string, limit int) ([]UsageRow, error) {
	rows, err := l.db.Query(
		"SELECT engine, provider, units, cost_usd FROM api_usage "+
			"WHERE date(created_at) = ? ORDER BY id DESC LIMIT ?", day, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageRow
	for rows.Next() {
		var u UsageRow
		if err := rows.Scan(&u.Engine, &u.Provider, &u.Units, &u.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---- settings (key-value dashboard config) ----

// Settings key conventions (used by the web worker / dashboard):
//
//	tts.chain, llm.chain — JSON-serialized ChainConfig of the respective
//	engine: the ordered provider chain used when the engine runs.
//	Any other key is free-form configuration storage.
const (
	SettingTTSChain                  = "tts.chain"
	SettingLLMChain                  = "llm.chain"
	SettingGeminiFreeTierFingerprint = "gemini.free_tier_fingerprint"
)

// GetSetting returns the value for key. The second return is false (not an
// error) when the key does not exist yet.
