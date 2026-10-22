package web

import (
	"database/sql"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"os"
	"sort"
)

type statusCount struct {
	Status string
	N      int
}

func sortedStatusCounts(accounts []*network.Account) []statusCount {
	counts := map[string]int{}
	for _, a := range accounts {
		counts[a.Status]++
	}
	out := make([]statusCount, 0, len(counts))
	for st, n := range counts {
		out = append(out, statusCount{st, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Status < out[j].Status })
	return out
}

// decisionView is the template/JSON-safe projection of a decisions row.
type decisionView struct {
	ID        int64  `json:"id"`
	Agent     string `json:"agent"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// queryDecisions runs a decisions-table SELECT the ledger API does not
// expose, projecting to decisionView.
func (s *Server) queryDecisions(query string, args ...any) ([]decisionView, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []decisionView
	for rows.Next() {
		var d decisionView
		var target, reason sql.NullString
		if err := rows.Scan(&d.ID, &d.Agent, &d.Action, &target, &reason, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Target, d.Reason = nullStr(target), nullStr(reason)
		out = append(out, d)
	}
	return out, rows.Err()
}

// scalarFloat is a tiny helper for the one-off aggregate queries.
func (s *Server) scalarFloat(query string, args ...any) float64 {
	var v float64
	if err := s.db.QueryRow(query, args...).Scan(&v); err != nil {
		return 0
	}
	return v
}

// scalarInt is scalarFloat's integer sibling (COUNT-style aggregates).
func (s *Server) scalarInt(query string, args ...any) int64 {
	var v int64
	if err := s.db.QueryRow(query, args...).Scan(&v); err != nil {
		return 0
	}
	return v
}

// --------------------------------------------------------------- publisher

type publisherRow struct {
	Username         string
	Status           string
	TiktokToken      bool
	TiktokTokenDead  bool // refresh token chết / hết hạn
	TiktokClient     bool // client key + secret present in env
	YoutubeToken     bool
	YoutubeClient    bool // OAuth client ID + secret present in env (Đợt AA)
	YoutubeTokenDead bool // refresh token chết (invalid_grant) — Đợt AA
	FbPage           bool
	FacebookClient   bool // Meta App ID + Secret present in settings
	Instagram        bool // IG_USER_ID + META_PAGE_TOKEN
	Threads          bool // THREADS_USER_ID + THREADS_TOKEN
	Platforms        []string
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
