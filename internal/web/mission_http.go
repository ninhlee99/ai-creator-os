package web

import (
	"encoding/json"
	"net/http"
	"net/url"
)

// handleMissionCancel cancels an active mission.
func (s *Server) handleMissionCancel(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id != "" && s.db != nil {
		_, _ = s.db.Exec(`UPDATE ai_missions SET status = 'failed', result_summary = 'Đã hủy bởi người điều hành' WHERE id = ?`, id)
	}
	seeOther(w, r, "/?ok="+url.QueryEscape("Đã dừng nhiệm vụ."))
}

// handleMissionDetailJSON returns the mission status as JSON for polling.
func (s *Server) handleMissionDetailJSON(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || s.db == nil {
		http.Error(w, "không tìm thấy", http.StatusNotFound)
		return
	}
	var m AIMission
	var stepsJSON string
	err := s.db.QueryRow(`
		SELECT id, goal, intent, target_account, account_id, status, progress, current_step,
		       steps_json, result_summary, artifact_url, created_at, finished_at
		FROM ai_missions WHERE id = ?`, id).Scan(
		&m.ID, &m.Goal, &m.Intent, &m.TargetAccount, &m.AccountID, &m.Status,
		&m.Progress, &m.CurrentStep, &stepsJSON, &m.ResultSummary, &m.ArtifactURL,
		&m.CreatedAt, &m.FinishedAt)
	if err != nil {
		http.Error(w, "không tìm thấy", http.StatusNotFound)
		return
	}
	_ = json.Unmarshal([]byte(stepsJSON), &m.Steps)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(m)
}
