package network

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// TRANSITIONS is the allowed account status transition table.
// Anything else is rejected loudly. Ported verbatim from account.py.
var TRANSITIONS = map[string]map[string]bool{
	"onboarding":       {"researching": true, "retired": true},
	"researching":      {"persona_assigned": true, "paused": true, "retired": true},
	"persona_assigned": {"growing": true, "paused": true, "retired": true},
	"growing":          {"paused": true, "retired": true},
	"paused":           {"growing": true, "retired": true},
	"penalized":        {"paused": true, "retired": true}, // human decides; never auto-resume
	"retired":          {},
}

const (
	FollowersSourceLegacy       = "legacy"
	FollowersSourceTikTokManual = "tiktok_manual"
	FollowersSourceTikTokAPI    = "tiktok_api"
)

// Account is the network-facing view of a ledger account: plain Go types
// instead of sql.NullString, with the lifecycle helpers attached.
type Account struct {
	ID                  int64
	Username            string
	Status              string
	Persona             string
	Niche               string
	NicheHint           string
	Followers           int64
	FollowersSource     string
	FollowersVerifiedAt string
	Topics              []string // episode topics from the topic engine
	// Affiliate autopilot (Ninh's 2026-10-01 requirements).
	Theme         string  // chủ đề: theme slug from products.THEMES, "" = chưa chọn
	Autopilot     bool    // hệ thống tự tìm sản phẩm + làm video
	MinCommission float64 // ngưỡng hoa hồng tối thiểu, 0..1
}

func jsonStringList(raw string) []string {
	var v []string
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
}

func wrapAccount(row *ledger.Account) *Account {
	return &Account{
		ID:                  row.ID,
		Username:            row.Username,
		Status:              row.Status,
		Persona:             row.Persona.String,
		Niche:               row.Niche.String,
		NicheHint:           row.NicheHint.String,
		Followers:           row.Followers,
		FollowersSource:     row.FollowersSource,
		FollowersVerifiedAt: row.FollowersVerifiedAt,
		Topics:              jsonStringList(row.TopicsJSON),
	}
}

// AccountManager is the account registry + lifecycle, backed by the ledger.
// It keeps its own read/write handle to the same SQLite file for the queries
// the ledger API does not expose (topic-plan writes, violation counter,
// decision/session evidence reads) — the Go equivalent of Python's
// ledger.db.execute.
type AccountManager struct {
	ledger *ledger.Ledger
	db     *sql.DB
}

// NewAccountManager opens the manager against the SQLite file at dbPath
// (the same file the ledger was opened with) and ensures the additive
// violation_count migration.
func NewAccountManager(l *ledger.Ledger, dbPath string) (*AccountManager, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("network: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("network: pragma %q: %w", p, err)
		}
	}
	// Additive-only migration: penalize() counts violations per account.
	if _, err := db.Exec(
		"ALTER TABLE accounts ADD COLUMN violation_count INTEGER NOT NULL DEFAULT 0"); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		db.Close()
		return nil, fmt.Errorf("network: migrate violation_count: %w", err)
	}
	// Additive-only migration: affiliate autopilot per account (2026-10-01).
	for _, stmt := range []string{
		"ALTER TABLE accounts ADD COLUMN theme TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE accounts ADD COLUMN autopilot INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE accounts ADD COLUMN min_commission REAL NOT NULL DEFAULT 0.10",
	} {
		if _, err := db.Exec(stmt); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			db.Close()
			return nil, fmt.Errorf("network: migrate autopilot: %w", err)
		}
	}
	// Map retired account states to the current TikTok growth stage.
	if _, err := db.Exec("UPDATE accounts SET status='growing' WHERE status IN ('live_ready','live')"); err != nil {
		db.Close()
		return nil, fmt.Errorf("network: migrate retired account states: %w", err)
	}
	return &AccountManager{ledger: l, db: db}, nil
}

// Close releases the manager's database handle.
func (m *AccountManager) Close() error { return m.db.Close() }

// Add registers a new account (duplicate username returns the existing id,
// no error — see ledger.AddAccount) and returns the wrapped account.
func (m *AccountManager) Add(username, nicheHint string, _ ...string) (*Account, error) {
	id, err := m.ledger.AddAccount(username, nicheHint)
	if err != nil {
		return nil, err
	}
	return m.Get(id)
}

// Get returns one account by id (sql.ErrNoRows when missing).
func (m *AccountManager) Get(accountID int64) (*Account, error) {
	row, err := m.ledger.GetAccount(accountID)
	if err != nil {
		return nil, err
	}
	return wrapAccount(row), nil
}

// List returns accounts, optionally filtered by status.
func (m *AccountManager) List(statuses ...string) ([]*Account, error) {
	rows, err := m.ledger.ListAccounts(statuses)
	if err != nil {
		return nil, err
	}
	out := make([]*Account, 0, len(rows))
	for i := range rows {
		out = append(out, wrapAccount(&rows[i]))
	}
	return out, nil
}

// SetTopicPlan persists the topic engine's output for an account.
func (m *AccountManager) SetTopicPlan(accountID int64, niche string, topics []string) (*Account, error) {
	tj, err := json.Marshal(topics)
	if err != nil {
		return nil, err
	}
	_, err = m.db.Exec(
		"UPDATE accounts SET niche = ?, topics_json = ?, updated_at = datetime('now') WHERE id = ?",
		niche, string(tj), accountID)
	if err != nil {
		return nil, err
	}
	return m.Get(accountID)
}

// UpdateAccount changes profile fields. Lifecycle status changes must use
// Transition so the allowed-stage rules and decision log stay in force.
func (m *AccountManager) UpdateAccount(accountID int64, username, niche, nicheHint, persona, theme string, autopilot bool, minComm float64) (*Account, error) {
	autoVal := 0
	if autopilot {
		autoVal = 1
	}
	_, err := m.db.Exec(`
		UPDATE accounts 
		SET username = ?, niche = ?, niche_hint = ?, persona = ?,
		    theme = ?, autopilot = ?, min_commission = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, username, niche, nicheHint, persona, theme, autoVal, minComm, accountID)
	if err != nil {
		return nil, fmt.Errorf("network: update account: %w", err)
	}
	return m.GetWithAutopilot(accountID)
}

// DeleteAccount removes an account and associated cascade records.
func (m *AccountManager) DeleteAccount(accountID int64) error {
	_, err := m.db.Exec(`DELETE FROM accounts WHERE id = ?`, accountID)
	if err != nil {
		return fmt.Errorf("network: delete account: %w", err)
	}
	return nil
}

// Transition moves an account to a new status, validating against
// TRANSITIONS first, and records the governance decision. Extra fields
// (e.g. persona) are written to the account row alongside the status.
func (m *AccountManager) Transition(accountID int64, to string, fields map[string]any) (*Account, error) {
	acct, err := m.Get(accountID)
	if err != nil {
		return nil, err
	}
	if !TRANSITIONS[acct.Status][to] {
		return nil, fmt.Errorf("illegal transition %s -> %s (account %s)",
			acct.Status, to, acct.Username)
	}
	upd := make(map[string]any, len(fields))
	for k, v := range fields {
		upd[k] = v
	}
	if err := m.ledger.SetAccountStatus(accountID, to, upd); err != nil {
		return nil, err
	}
	inputs := map[string]any{"to": to}
	for k, v := range fields {
		inputs[k] = v
	}
	target := acct.Username
	if err := m.ledger.Decide("orchestrator", "account_transition", &target,
		acct.Status+" -> "+to, inputs); err != nil {
		return nil, err
	}
	return m.Get(accountID)
}

// Penalize penalizes an account: the violation counter is bumped, the
// account moves to "penalized" (which NEVER auto-resumes — a human decides),
// and sibling accounts on the same persona are network-paused.
func (m *AccountManager) Penalize(accountID int64, reason string) (*Account, error) {
	count, err := m.bumpViolationCount(accountID)
	if err != nil {
		return nil, err
	}
	acct, err := m.Transition(accountID, "penalized",
		map[string]any{"violation_count": count})
	if err != nil {
		return nil, err
	}
	target := acct.Username
	if err := m.ledger.Decide("governance", "account_penalized", &target,
		reason, map[string]any{"violation_count": count}); err != nil {
		return nil, err
	}
	if _, err := m.NetworkPauseSimilar(accountID); err != nil {
		return nil, err
	}
	return acct, nil
}

func (m *AccountManager) bumpViolationCount(accountID int64) (int64, error) {
	if _, err := m.db.Exec(
		"UPDATE accounts SET violation_count = violation_count + 1, "+
			"updated_at = datetime('now') WHERE id = ?", accountID); err != nil {
		return 0, err
	}
	var n int64
	if err := m.db.QueryRow(
		"SELECT violation_count FROM accounts WHERE id = ?", accountID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// NetworkPauseSimilar pauses active sibling accounts after a policy violation.
func (m *AccountManager) NetworkPauseSimilar(accountID int64) ([]*Account, error) {
	acct, err := m.Get(accountID)
	if err != nil {
		return nil, err
	}
	persona := acct.Persona
	accts, err := m.List()
	if err != nil {
		return nil, err
	}
	var paused []*Account
	for _, a := range accts {
		if a.Persona == persona && (a.Status == "growing" || a.Status == "persona_assigned") {
			p, err := m.Transition(a.ID, "paused", nil)
			if err != nil {
				return paused, err
			}
			paused = append(paused, p)
		}
	}
	usernames := make([]string, 0, len(paused))
	for _, p := range paused {
		usernames = append(usernames, p.Username)
	}
	target := "persona=" + persona
	reason := fmt.Sprintf("guardrail: account %s penalized; pausing same-persona siblings",
		acct.Username)
	if err := m.ledger.Decide("governance", "network_pause", &target,
		reason, map[string]any{"paused": usernames}); err != nil {
		return paused, err
	}
	return paused, nil
}
