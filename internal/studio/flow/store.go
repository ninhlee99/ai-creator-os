package flow

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Store quản lý lưu trữ SQLite cho đồ thị workflow và lịch sử thực thi.
type Store struct {
	db *sql.DB
}

// NewStore khởi tạo và tự động migrate bảng SQLite cho flow workflows.
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("db connection is nil")
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	// Không tự động seed presets vào database khi khởi động.
	// Chỉ tạo flow theo nhu cầu, và chỉ lưu thành flow hệ thống (preset) khi người dùng yêu cầu.
	return s, nil
}

func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS studio_workflow_graphs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT 'custom',
			graph_json TEXT NOT NULL DEFAULT '{}',
			is_preset INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS studio_workflow_runs (
			id TEXT PRIMARY KEY,
			graph_id TEXT NOT NULL,
			graph_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			progress INTEGER NOT NULL DEFAULT 0,
			node_states TEXT NOT NULL DEFAULT '{}',
			outputs TEXT NOT NULL DEFAULT '{}',
			final_output TEXT NOT NULL DEFAULT '',
			log TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_runs_time ON studio_workflow_runs(created_at DESC);`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate workflow store: %w", err)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM studio_workflow_graphs WHERE id = ? AND is_preset = 1`, "preset_fashion_3d_studio"); err != nil {
		return fmt.Errorf("remove retired 3D/VFX preset: %w", err)
	}
	return nil
}

// SeedDefaultPresets nạp và đồng bộ các preset mẫu vào cơ sở dữ liệu.
func (s *Store) SeedDefaultPresets() error {
	for _, preset := range GetPresetGraphs() {
		if err := s.SaveGraph(&preset); err != nil {
			return err
		}
	}
	return nil
}

// SaveGraph lưu hoặc cập nhật thông tin đồ thị vào database.
func (s *Store) SaveGraph(g *FlowGraph) error {
	if g == nil || g.ID == "" {
		return errors.New("graph hoặc ID không hợp lệ")
	}
	now := time.Now().Format(time.RFC3339)
	if g.CreatedAt == "" {
		g.CreatedAt = now
	}
	g.UpdatedAt = now

	data, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("mã hóa graph JSON: %w", err)
	}

	isPresetInt := 0
	if g.IsPreset {
		isPresetInt = 1
	}

	query := `INSERT INTO studio_workflow_graphs (id, name, description, category, graph_json, is_preset, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			category = excluded.category,
			graph_json = excluded.graph_json,
			updated_at = excluded.updated_at;`

	_, err = s.db.Exec(query, g.ID, g.Name, g.Description, g.Category, string(data), isPresetInt, g.CreatedAt, g.UpdatedAt)
	return err
}

// GetGraph lấy đồ thị theo ID.
func (s *Store) GetGraph(id string) (*FlowGraph, error) {
	var graphJSON string
	err := s.db.QueryRow(`SELECT graph_json FROM studio_workflow_graphs WHERE id = ?;`, id).Scan(&graphJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Thử tìm trong bộ preset tĩnh
			for _, p := range GetPresetGraphs() {
				if p.ID == id {
					return &p, nil
				}
			}
			return nil, fmt.Errorf("không tìm thấy workflow %s", id)
		}
		return nil, err
	}
	var g FlowGraph
	if err := json.Unmarshal([]byte(graphJSON), &g); err != nil {
		return nil, fmt.Errorf("giải mã workflow JSON: %w", err)
	}
	return &g, nil
}

// ListGraphs trả về danh sách tóm tắt tất cả workflows.
func (s *Store) ListGraphs() ([]FlowSummary, error) {
	rows, err := s.db.Query(`SELECT id, name, description, category, is_preset, graph_json, updated_at FROM studio_workflow_graphs ORDER BY is_preset DESC, updated_at DESC;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]FlowSummary, 0)
	for rows.Next() {
		var (
			id, name, desc, cat, updated string
			isPresetInt                  int
			graphJSON                    string
		)
		if err := rows.Scan(&id, &name, &desc, &cat, &isPresetInt, &graphJSON, &updated); err != nil {
			return nil, err
		}
		var g FlowGraph
		_ = json.Unmarshal([]byte(graphJSON), &g)
		inputs, outputs, contractErr := WorkflowContract(&g)
		_, validationErr := ValidateGraph(&g)
		workflowReusable := contractErr == nil && validationErr == nil && len(inputs) > 0 && len(outputs) > 0
		list = append(list, FlowSummary{
			ID:               id,
			Name:             name,
			Description:      desc,
			Category:         cat,
			IsPreset:         isPresetInt == 1,
			NodeCount:        len(g.Nodes),
			EdgeCount:        len(g.Edges),
			WorkflowReusable: workflowReusable,
			Inputs:           inputs,
			Outputs:          outputs,
			UpdatedAt:        updated,
		})
	}
	return list, nil
}

// DeleteGraph xóa workflow khỏi database (không cho phép xóa preset).
func (s *Store) DeleteGraph(id string) error {
	for _, p := range GetPresetGraphs() {
		if p.ID == id {
			return errors.New("không thể xóa workflow preset mẫu của hệ thống")
		}
	}
	var isPreset int
	err := s.db.QueryRow(`SELECT is_preset FROM studio_workflow_graphs WHERE id = ?;`, id).Scan(&isPreset)
	if err == nil && isPreset == 1 {
		return errors.New("không thể xóa workflow preset mẫu của hệ thống")
	}
	_, err = s.db.Exec(`DELETE FROM studio_workflow_graphs WHERE id = ?;`, id)
	return err
}

// SaveRun lưu hoặc cập nhật lượt chạy flow.
func (s *Store) SaveRun(r *FlowRun) error {
	if r == nil || r.ID == "" {
		return errors.New("run hoặc ID không hợp lệ")
	}
	nodeStatesJSON, _ := json.Marshal(r.NodeStates)
	outputsJSON, _ := json.Marshal(r.Outputs)

	query := `INSERT INTO studio_workflow_runs (id, graph_id, graph_name, status, progress, node_states, outputs, final_output, log, created_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			progress = excluded.progress,
			node_states = excluded.node_states,
			outputs = excluded.outputs,
			final_output = excluded.final_output,
			log = excluded.log,
			finished_at = excluded.finished_at;`

	_, err := s.db.Exec(query, r.ID, r.GraphID, r.GraphName, r.Status, r.Progress, string(nodeStatesJSON), string(outputsJSON), r.FinalOutput, r.Log, r.CreatedAt, r.FinishedAt)
	return err
}

// GetRun lấy trạng thái chi tiết của một lượt chạy flow.
func (s *Store) GetRun(id string) (*FlowRun, error) {
	var (
		r                       FlowRun
		nodeStatesJSON, outJSON string
	)
	query := `SELECT id, graph_id, graph_name, status, progress, node_states, outputs, final_output, log, created_at, finished_at
		FROM studio_workflow_runs WHERE id = ?;`
	err := s.db.QueryRow(query, id).Scan(
		&r.ID, &r.GraphID, &r.GraphName, &r.Status, &r.Progress,
		&nodeStatesJSON, &outJSON, &r.FinalOutput, &r.Log, &r.CreatedAt, &r.FinishedAt,
	)
	if err != nil {
		return nil, err
	}
	r.NodeStates = make(map[string]NodeState)
	r.Outputs = make(map[string]any)
	_ = json.Unmarshal([]byte(nodeStatesJSON), &r.NodeStates)
	_ = json.Unmarshal([]byte(outJSON), &r.Outputs)
	return &r, nil
}

// ListRuns lấy danh sách các lần thực thi gần đây.
func (s *Store) ListRuns(limit int) ([]FlowRun, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, graph_id, graph_name, status, progress, node_states, outputs, final_output, log, created_at, finished_at
		FROM studio_workflow_runs ORDER BY created_at DESC LIMIT ?;`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]FlowRun, 0)
	for rows.Next() {
		var (
			r                       FlowRun
			nodeStatesJSON, outJSON string
		)
		if err := rows.Scan(
			&r.ID, &r.GraphID, &r.GraphName, &r.Status, &r.Progress,
			&nodeStatesJSON, &outJSON, &r.FinalOutput, &r.Log, &r.CreatedAt, &r.FinishedAt,
		); err != nil {
			return nil, err
		}
		r.NodeStates = make(map[string]NodeState)
		r.Outputs = make(map[string]any)
		_ = json.Unmarshal([]byte(nodeStatesJSON), &r.NodeStates)
		_ = json.Unmarshal([]byte(outJSON), &r.Outputs)
		list = append(list, r)
	}
	return list, nil
}
