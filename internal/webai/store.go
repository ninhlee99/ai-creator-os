package webai

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// Store persists Web AI platform credentials and session states with multi-account rotation.
type Store struct {
	mu sync.RWMutex
	db *sql.DB
}

// NewStore opens or creates the Web AI database.
func NewStore(dbPath string) (*Store, error) {
	dsn := dbPath
	if !strings.Contains(dbPath, "?") {
		dsn = dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-20000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("webai: open db: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=NORMAL"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("webai: pragma %q: %w", p, err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
