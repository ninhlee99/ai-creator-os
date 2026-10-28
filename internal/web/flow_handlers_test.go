package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
	_ "modernc.org/sqlite"
)

func setupTestServerWithFlow(t *testing.T) (*Server, func()) {
	tmpDir, err := os.MkdirTemp("", "web-flow-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "studio.db")
	outDir := filepath.Join(tmpDir, "out")
	_ = os.MkdirAll(outDir, 0o755)

	st, err := studio.New(dbPath, nil, nil, nil, outDir)
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}

	srv := &Server{
		Studio: st,
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return srv, cleanup
}

func TestTC13_WebAPI_Definitions(t *testing.T) {
	srv, cleanup := setupTestServerWithFlow(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/flow/definitions", nil)
	w := httptest.NewRecorder()

	srv.handleAPIFlowDefinitions(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var res struct {
		Ok   bool                  `json:"ok"`
		Defs []flow.NodeDefinition `json:"defs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("Decode JSON: %v", err)
	}
	if !res.Ok || len(res.Defs) < 17 {
		t.Errorf("Kỳ vọng ít nhất 17 node definitions, nhận: %d", len(res.Defs))
	}
}

func TestTC14_WebAPI_GraphsCRUD(t *testing.T) {
	srv, cleanup := setupTestServerWithFlow(t)
	defer cleanup()

	// 1. GET /api/flow/graphs
	req := httptest.NewRequest("GET", "/api/flow/graphs", nil)
	w := httptest.NewRecorder()
	srv.handleAPIFlowGraphs(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("GET /api/flow/graphs status = %d", w.Result().StatusCode)
	}

	// 2. POST /api/flow/graphs (Save custom graph)
	customG := flow.FlowGraph{
		ID:          "api_custom_01",
		Name:        "API Custom Workflow",
		Description: "Tạo qua API test",
		Category:    "custom",
		Nodes: []flow.FlowNode{
			{ID: "n1", Type: "character_input"},
		},
	}
	body, _ := json.Marshal(customG)
	reqPost := httptest.NewRequest("POST", "/api/flow/graphs", bytes.NewReader(body))
	wPost := httptest.NewRecorder()
	srv.handleAPIFlowGraphs(wPost, reqPost)
	if wPost.Result().StatusCode != http.StatusOK {
		t.Fatalf("POST /api/flow/graphs status = %d", wPost.Result().StatusCode)
	}

	// 3. GET /api/flow/graphs/api_custom_01
	reqDetail := httptest.NewRequest("GET", "/api/flow/graphs/api_custom_01", nil)
	reqDetail.SetPathValue("id", "api_custom_01")
	wDetail := httptest.NewRecorder()
	srv.handleAPIFlowGraphDetail(wDetail, reqDetail)
	if wDetail.Result().StatusCode != http.StatusOK {
		t.Fatalf("GET detail status = %d", wDetail.Result().StatusCode)
	}

	// 4. DELETE /api/flow/graphs/api_custom_01
	reqDel := httptest.NewRequest("DELETE", "/api/flow/graphs/api_custom_01", nil)
	reqDel.SetPathValue("id", "api_custom_01")
	wDel := httptest.NewRecorder()
	srv.handleAPIFlowGraphDetail(wDel, reqDel)
	if wDel.Result().StatusCode != http.StatusOK {
		t.Fatalf("DELETE detail status = %d", wDel.Result().StatusCode)
	}
}

func TestTC15_WebAPI_ExecuteAndPolling(t *testing.T) {
	srv, cleanup := setupTestServerWithFlow(t)
	defer cleanup()

	// Execute preset_viral_dance
	execPayload := map[string]any{
		"graph_id":                  "preset_viral_dance",
		"muse_credits_acknowledged": true,
	}
	body, _ := json.Marshal(execPayload)
	reqExec := httptest.NewRequest("POST", "/api/flow/execute", bytes.NewReader(body))
	wExec := httptest.NewRecorder()
	srv.handleAPIFlowExecute(wExec, reqExec)

	if wExec.Result().StatusCode != http.StatusOK {
		t.Fatalf("POST /api/flow/execute status = %d", wExec.Result().StatusCode)
	}

	var execRes struct {
		Ok    bool   `json:"ok"`
		RunID string `json:"run_id"`
	}
	_ = json.NewDecoder(wExec.Result().Body).Decode(&execRes)
	if !execRes.Ok || execRes.RunID == "" {
		t.Fatalf("Không nhận được RunID hợp lệ: %+v", execRes)
	}

	// Chờ luồng chạy nền xong
	time.Sleep(300 * time.Millisecond)

	// Poll status
	reqStatus := httptest.NewRequest("GET", "/api/flow/runs/"+execRes.RunID, nil)
	reqStatus.SetPathValue("id", execRes.RunID)
	wStatus := httptest.NewRecorder()
	srv.handleAPIFlowRunDetail(wStatus, reqStatus)

	if wStatus.Result().StatusCode != http.StatusOK {
		t.Fatalf("GET /api/flow/runs/{id} status = %d", wStatus.Result().StatusCode)
	}

	var statusRes struct {
		Ok  bool         `json:"ok"`
		Run flow.FlowRun `json:"run"`
	}
	_ = json.NewDecoder(wStatus.Result().Body).Decode(&statusRes)
	if !statusRes.Ok || statusRes.Run.ID != execRes.RunID {
		t.Errorf("Dữ liệu run không khớp: %+v", statusRes)
	}

	// List runs
	reqRuns := httptest.NewRequest("GET", "/api/flow/runs", nil)
	wRuns := httptest.NewRecorder()
	srv.handleAPIFlowRuns(wRuns, reqRuns)
	if wRuns.Result().StatusCode != http.StatusOK {
		t.Fatalf("GET /api/flow/runs status = %d", wRuns.Result().StatusCode)
	}
}

func TestFlowCancelDoesNotOverwriteTerminalRun(t *testing.T) {
	srv, cleanup := setupTestServerWithFlow(t)
	defer cleanup()

	store, err := srv.Studio.GetFlowStore()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"completed", "failed", "cancelled"} {
		run := &flow.FlowRun{ID: "terminal-" + status, Status: status, NodeStates: map[string]flow.NodeState{}, Outputs: map[string]any{}}
		if err := store.SaveRun(run); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/flow/runs/"+run.ID+"/cancel", nil)
		req.SetPathValue("id", run.ID)
		w := httptest.NewRecorder()
		srv.handleAPIFlowRunCancel(w, req)
		if w.Result().StatusCode != http.StatusOK {
			t.Fatalf("cancel %s returned HTTP %d", status, w.Result().StatusCode)
		}
		var response struct {
			Cancelled bool `json:"cancelled"`
		}
		if err := json.NewDecoder(w.Result().Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.Cancelled != (status == "cancelled") {
			t.Fatalf("cancel %s response cancelled=%v", status, response.Cancelled)
		}
		persisted, err := store.GetRun(run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Status != status {
			t.Fatalf("cancel overwrote terminal status %q with %q", status, persisted.Status)
		}
	}
}

func TestFlowCancelMarksActiveRun(t *testing.T) {
	srv, cleanup := setupTestServerWithFlow(t)
	defer cleanup()

	store, err := srv.Studio.GetFlowStore()
	if err != nil {
		t.Fatal(err)
	}
	run := &flow.FlowRun{ID: "active-run", Status: "running", NodeStates: map[string]flow.NodeState{}, Outputs: map[string]any{}}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/flow/runs/"+run.ID+"/cancel", nil)
	req.SetPathValue("id", run.ID)
	w := httptest.NewRecorder()
	srv.handleAPIFlowRunCancel(w, req)
	persisted, err := store.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != "cancelled" {
		t.Fatalf("active run status = %q, want cancelled", persisted.Status)
	}
}

func TestTC16_WebAPI_Upload(t *testing.T) {
	srv, cleanup := setupTestServerWithFlow(t)
	defer cleanup()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "áo nhân vật (test).JPEG")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	// Ghi byte ảnh JPEG hợp lệ
	_, _ = part.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xFF, 0xD9})
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/flow/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	srv.handleAPIFlowUpload(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("POST /api/flow/upload status = %d, body = %s", w.Result().StatusCode, w.Body.String())
	}

	var res struct {
		Ok       bool   `json:"ok"`
		FilePath string `json:"file_path"`
		Filename string `json:"filename"`
		URL      string `json:"url"`
	}
	if err := json.NewDecoder(w.Result().Body).Decode(&res); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !res.Ok || res.FilePath == "" || res.URL == "" {
		t.Errorf("Kết quả upload không hợp lệ: %+v", res)
	}
	if !mediaNameRe.MatchString(res.Filename) {
		t.Fatalf("upload filename %q is rejected by media route", res.Filename)
	}
	if filepath.Ext(res.Filename) != ".jpeg" {
		t.Fatalf("upload extension = %q, want normalized .jpeg", filepath.Ext(res.Filename))
	}
	if _, err := os.Stat(res.FilePath); err != nil {
		t.Errorf("File upload không tồn tại trên đĩa: %s", res.FilePath)
	}
	mediaReq := httptest.NewRequest("GET", res.URL, nil)
	mediaReq.SetPathValue("name", res.Filename)
	mediaResp := httptest.NewRecorder()
	srv.handleMedia(mediaResp, mediaReq)
	if mediaResp.Result().StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", res.URL, mediaResp.Result().StatusCode)
	}
}

type zeroFlowUploadReader struct{}

func (zeroFlowUploadReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestFlowUploadRejectsFilesOverLimit(t *testing.T) {
	srv, cleanup := setupTestServerWithFlow(t)
	defer cleanup()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "large.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(part, zeroFlowUploadReader{}, maxFlowUploadBytes+1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/flow/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	srv.handleAPIFlowUpload(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
	entries, err := os.ReadDir(srv.Studio.UploadDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversized upload left %d files in upload dir", len(entries))
	}
}
