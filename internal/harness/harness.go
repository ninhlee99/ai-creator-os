package harness

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// Harness là Bộ điều phối trung tâm (Master Agent Harness), chịu trách nhiệm
// vận hành toàn bộ hệ sinh thái tự chủ của AI Creator OS.
type Harness struct {
	mu            sync.RWMutex
	db            *sql.DB
	memory        *MemoryStore
	ooda          *OODAEngine
	studio        *studio.Studio
	mgr           *network.AccountManager
	growth        *growth.Store
	llm           studio.LLM
	autonomyLevel AutonomyLevel
	interval      time.Duration
	latestCycle   *OODACycleResult
	recentCycles  []*OODACycleResult
	cancel        context.CancelFunc
}

// Config cấu hình cho Harness Agent.
type Config struct {
	AutonomyLevel AutonomyLevel
	Interval      time.Duration
}

// New khởi tạo Master Agent Harness.
func New(
	db *sql.DB,
	studioInst *studio.Studio,
	mgr *network.AccountManager,
	growthStore *growth.Store,
	llm studio.LLM,
	cfg Config,
) (*Harness, error) {
	memStore, err := NewMemoryStore(db)
	if err != nil {
		log.Printf("harness: cảnh báo khởi tạo memory store: %v", err)
	}

	if cfg.AutonomyLevel == 0 {
		cfg.AutonomyLevel = AutonomyFullyAutonomous
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 6 * time.Hour
	}

	ooda := NewOODAEngine(memStore, llm)

	h := &Harness{
		db:            db,
		memory:        memStore,
		ooda:          ooda,
		studio:        studioInst,
		mgr:           mgr,
		growth:        growthStore,
		llm:           llm,
		autonomyLevel: cfg.AutonomyLevel,
		interval:      cfg.Interval,
	}

	_ = h.ensureHarnessTable()
	h.loadRecentCycles()
	return h, nil
}

func (h *Harness) ensureHarnessTable() error {
	if h.db == nil {
		return nil
	}
	_, err := h.db.Exec(`
		CREATE TABLE IF NOT EXISTS harness_cycles (
			id TEXT PRIMARY KEY,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			diagnosis TEXT NOT NULL DEFAULT '',
			observations_json TEXT NOT NULL DEFAULT '{}',
			decisions_json TEXT NOT NULL DEFAULT '[]',
			action_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'completed'
		);
		CREATE INDEX IF NOT EXISTS idx_harness_cycles_started ON harness_cycles(started_at DESC);
	`)
	return err
}

// Memory trả về kho ký ức thực chiến.
func (h *Harness) Memory() *MemoryStore {
	return h.memory
}

// AutonomyLevel trả về cấp độ tự chủ hiện tại.
func (h *Harness) AutonomyLevel() AutonomyLevel {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.autonomyLevel
}

// SetAutonomyLevel điều chỉnh cấp độ tự chủ.
func (h *Harness) SetAutonomyLevel(level AutonomyLevel) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.autonomyLevel = level
}

// LatestCycle trả về kết quả chu kỳ OODA gần nhất.
func (h *Harness) LatestCycle() *OODACycleResult {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.latestCycle
}

// RunCycle kích hoạt một chu kỳ nhận thức & hành động OODA ngay lập tức.
func (h *Harness) RunCycle(ctx context.Context) (*OODACycleResult, error) {
	h.mu.Lock()
	level := h.autonomyLevel
	mgr := h.mgr
	studioInst := h.studio
	ooda := h.ooda
	h.mu.Unlock()

	var accounts []*network.Account
	if mgr != nil {
		accts, _ := mgr.List()
		accounts = accts
	}

	res, err := ooda.ExecuteCycle(ctx, accounts, studioInst, level)
	if err != nil {
		return nil, fmt.Errorf("harness run cycle: %w", err)
	}

	h.mu.Lock()
	h.latestCycle = res
	h.recentCycles = append([]*OODACycleResult{res}, h.recentCycles...)
	if len(h.recentCycles) > 20 {
		h.recentCycles = h.recentCycles[:20]
	}
	h.mu.Unlock()

	// Lưu vết vào SQLite
	if h.db != nil {
		obsJSON, _ := json.Marshal(res.Observations)
		decJSON, _ := json.Marshal(res.Decisions)
		_, _ = h.db.Exec(`
			INSERT OR REPLACE INTO harness_cycles (
				id, started_at, finished_at, diagnosis, observations_json, decisions_json, action_count, status
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, res.CycleID, res.StartedAt, res.FinishedAt, res.Diagnosis, string(obsJSON), string(decJSON), res.ActionCount, res.Status)
	}

	return res, nil
}

// ListRecentCycles lấy danh sách các chu kỳ OODA đã thực hiện.
func (h *Harness) ListRecentCycles(limit int) []*OODACycleResult {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if limit <= 0 || limit > len(h.recentCycles) {
		limit = len(h.recentCycles)
	}
	out := make([]*OODACycleResult, limit)
	copy(out, h.recentCycles[:limit])
	return out
}

// GetCycle lấy chi tiết một chu kỳ OODA theo ID từ bộ nhớ cache hoặc SQLite.
func (h *Harness) GetCycle(id string) (*OODACycleResult, error) {
	h.mu.RLock()
	for _, c := range h.recentCycles {
		if c.CycleID == id {
			h.mu.RUnlock()
			return c, nil
		}
	}
	h.mu.RUnlock()

	if h.db == nil {
		return nil, sql.ErrNoRows
	}

	var startedAt, finishedAt, diagnosis, obsJSON, decJSON, status string
	var actionCount int
	err := h.db.QueryRow(`
		SELECT started_at, finished_at, diagnosis, observations_json, decisions_json, action_count, status
		FROM harness_cycles
		WHERE id = ?
	`, id).Scan(&startedAt, &finishedAt, &diagnosis, &obsJSON, &decJSON, &actionCount, &status)
	if err != nil {
		return nil, err
	}

	var obs OODAPerception
	_ = json.Unmarshal([]byte(obsJSON), &obs)
	var decs []OODADecisionTask
	_ = json.Unmarshal([]byte(decJSON), &decs)

	return &OODACycleResult{
		CycleID:      id,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		Observations: obs,
		Diagnosis:    diagnosis,
		Decisions:    decs,
		ActionCount:  actionCount,
		Status:       status,
	}, nil
}

func (h *Harness) loadRecentCycles() {
	if h.db == nil {
		return
	}
	rows, err := h.db.Query(`
		SELECT id, started_at, finished_at, diagnosis, observations_json, decisions_json, action_count, status
		FROM harness_cycles
		ORDER BY started_at DESC
		LIMIT 20
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	var cycles []*OODACycleResult
	for rows.Next() {
		var id, start, end, diag, obsJSON, decJSON, st string
		var count int
		if err := rows.Scan(&id, &start, &end, &diag, &obsJSON, &decJSON, &count, &st); err != nil {
			continue
		}
		var obs OODAPerception
		_ = json.Unmarshal([]byte(obsJSON), &obs)
		var decs []OODADecisionTask
		_ = json.Unmarshal([]byte(decJSON), &decs)

		cycles = append(cycles, &OODACycleResult{
			CycleID:      id,
			StartedAt:    start,
			FinishedAt:   end,
			Observations: obs,
			Diagnosis:    diag,
			Decisions:    decs,
			ActionCount:  count,
			Status:       st,
		})
	}
	h.mu.Lock()
	h.recentCycles = cycles
	if len(cycles) > 0 && h.latestCycle == nil {
		h.latestCycle = cycles[0]
	}
	h.mu.Unlock()
}

// StartBackgroundLoop bắt đầu vòng lặp tự hành nền của Harness Agent.
func (h *Harness) StartBackgroundLoop(ctx context.Context) {
	cycleCtx, cancel := context.WithCancel(ctx)
	h.mu.Lock()
	h.cancel = cancel
	interval := h.interval
	h.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("harness: loop PANIC recovered: %v", r)
			}
		}()

		// Chạy 1 chu kỳ ban đầu sau 30 giây khởi động để thẩm định hệ sinh thái
		select {
		case <-time.After(30 * time.Second):
			_, _ = h.RunCycle(cycleCtx)
		case <-cycleCtx.Done():
			return
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, err := h.RunCycle(cycleCtx)
				if err != nil {
					log.Printf("harness: chu kỳ tự hành gặp lỗi: %v", err)
				}
			case <-cycleCtx.Done():
				return
			}
		}
	}()
}

// Stop dừng vòng lặp tự hành.
func (h *Harness) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		h.cancel()
	}
}
