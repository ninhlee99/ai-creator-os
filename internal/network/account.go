package network

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
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
	"growing":          {"live_ready": true, "paused": true, "retired": true},
	"live_ready":       {"live": true, "paused": true, "retired": true},
	"live":             {"live_ready": true, "paused": true, "penalized": true, "retired": true},
	"paused":           {"live_ready": true, "growing": true, "retired": true},
	"penalized":        {"paused": true, "retired": true}, // human decides; never auto-resume
	"retired":          {},
}

// LiveEligibleStatuses mirrors Python's LIVE_ELIGIBLE_STATUSES.
var LiveEligibleStatuses = map[string]bool{"live_ready": true, "live": true}

// FollowersToLive is the TikTok follower requirement for going live.
const FollowersToLive = 1000

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
	RtmpKeyRef          string
	RestWeekday         int64
	Topics              []string // episode topics from the topic engine
	YoutubeChannel      string
	YoutubeContentTypes []string
	// Affiliate autopilot (Ninh's 2026-10-01 requirements).
	Theme         string  // chủ đề: theme slug from products.THEMES, "" = chưa chọn
	Autopilot     bool    // hệ thống tự tìm sản phẩm + làm video
	MinCommission float64 // ngưỡng hoa hồng tối thiểu, 0..1
}

// LiveEligible reports whether the account may go live right now.
func (a *Account) LiveEligible() bool {
	return LiveEligibleStatuses[a.Status] && a.Followers >= FollowersToLive
}

// RtmpKey resolves the actual stream key from the environment at runtime.
// rtmp_key_ref names the env var; when unset it falls back to the
// TIKTOK_RTMP_KEY[_<USERNAME>] convention. Empty when nothing is set.
func (a *Account) RtmpKey() string {
	if a.RtmpKeyRef != "" {
		if v := os.Getenv(a.RtmpKeyRef); v != "" {
			return v
		}
	}
	if v := os.Getenv("TIKTOK_RTMP_KEY_" + strings.ToUpper(a.Username)); v != "" {
		return v
	}
	return os.Getenv("TIKTOK_RTMP_KEY")
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
		RtmpKeyRef:          row.RtmpKeyRef.String,
		RestWeekday:         row.RestWeekday,
		Topics:              jsonStringList(row.TopicsJSON),
		YoutubeChannel:      row.YoutubeChannel.String,
		YoutubeContentTypes: jsonStringList(row.YoutubeContentTypes),
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
	return &AccountManager{ledger: l, db: db}, nil
}

// Close releases the manager's database handle.
func (m *AccountManager) Close() error { return m.db.Close() }

// Add registers a new account (duplicate username returns the existing id,
// no error — see ledger.AddAccount) and returns the wrapped account.
func (m *AccountManager) Add(username, nicheHint, rtmpKeyRef string) (*Account, error) {
	id, err := m.ledger.AddAccount(username, nicheHint, rtmpKeyRef)
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

// ActivePersonas returns the personas of all non-retired accounts that have
// one assigned — the diversity input for AssignPersona.
func (m *AccountManager) ActivePersonas() ([]string, error) {
	accts, err := m.List()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, a := range accts {
		if a.Persona != "" && a.Status != "retired" {
			out = append(out, a.Persona)
		}
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

// SetYoutube configures this account's YouTube channel and which content
// kinds it accepts (subset of short_video, short_film, ai_music, ai_remix;
// empty disables YouTube for the account).
func (m *AccountManager) SetYoutube(accountID int64, channel string, contentTypes []string) (*Account, error) {
	valid := map[string]bool{
		"short_video": true, "short_film": true, "ai_music": true, "ai_remix": true,
	}
	var cleaned []string
	for _, c := range contentTypes {
		if valid[c] {
			cleaned = append(cleaned, c)
		}
	}
	cj, err := json.Marshal(cleaned)
	if err != nil {
		return nil, err
	}
	_, err = m.db.Exec(
		"UPDATE accounts SET youtube_channel = ?, youtube_content_types = ?, "+
			"updated_at = datetime('now') WHERE id = ?",
		channel, string(cj), accountID)
	if err != nil {
		return nil, err
	}
	return m.Get(accountID)
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

// NetworkPauseSimilar is the guardrail for a penalized account: pause every
// other account running the same persona that is live or live-ready, so the
// network can never evade a penalty via sibling accounts.
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
		if a.Persona == persona && (a.Status == "live" || a.Status == "live_ready") {
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
