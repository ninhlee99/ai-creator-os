package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAgentWorkforceList(t *testing.T) {
	srv := newTestServer(t)
	agents := srv.listAgents()
	if len(agents) < 4 {
		t.Fatalf("expected at least 4 elite agents, got %d", len(agents))
	}

	foundMinhThu := false
	foundDucHuy := false
	for _, ag := range agents {
		if strings.Contains(ag.Name, "Minh Thư") {
			foundMinhThu = true
			if len(ag.BestPractices) == 0 {
				t.Errorf("expected Minh Thư to have best practices, got none")
			}
			if ag.Level < 1 {
				t.Errorf("expected positive level for Minh Thu, got %d", ag.Level)
			}
		}
		if strings.Contains(ag.Name, "Đức Huy") {
			foundDucHuy = true
		}
	}

	if !foundMinhThu || !foundDucHuy {
		t.Errorf("missing key elite agents in default workforce")
	}
}

func TestAgentWorkforceCreateAndToggle(t *testing.T) {
	srv := newTestServer(t)

	// 1. Create a custom agent
	form := url.Values{
		"name":   {"Hoàng Yến Skincare"},
		"avatar": {"👩‍💼"},
		"role":   {"Reviewer Mỹ Phẩm & Chăm Sóc Da"},
		"niche":  {"mỹ phẩm"},
		"tone":   {"Thân thiện, chân thật, hay dùng từ mấy bà ơi"},
		"voice":  {"Nữ miền Nam ngọt ngào"},
	}

	req := httptest.NewRequest("POST", "/team/agents/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.handleAgentCreate(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", w.Code)
	}

	agents := srv.listAgents()
	var createdID string
	for _, ag := range agents {
		if ag.Name == "Hoàng Yến Skincare" {
			createdID = ag.ID
			if !ag.IsCustom {
				t.Errorf("expected is_custom = true")
			}
			break
		}
	}
	if createdID == "" {
		t.Fatalf("newly created agent not found in list")
	}

	// 2. Toggle autopilot
	tForm := url.Values{"id": {createdID}}
	req = httptest.NewRequest("POST", "/team/agents/toggle", strings.NewReader(tForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.handleAgentToggleAutopilot(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", w.Code)
	}

	// 3. Delete custom agent
	dForm := url.Values{"id": {createdID}}
	req = httptest.NewRequest("POST", "/team/agents/delete", strings.NewReader(dForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.handleAgentDelete(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", w.Code)
	}

	agents = srv.listAgents()
	for _, ag := range agents {
		if ag.ID == createdID {
			t.Errorf("agent %s should have been deleted", createdID)
		}
	}
}

func TestTeamDispatchGoal(t *testing.T) {
	srv := newTestServer(t)

	// Dispatch a tech goal
	form := url.Values{"goal": {"Tạo video review tai nghe Sony chống ồn mới nhất"}}
	req := httptest.NewRequest("POST", "/team/dispatch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.handleTeamDispatchGoal(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/studio") || !strings.Contains(loc, "c%C3%B4ng+ngh%E1%BB%87") {
		t.Errorf("expected redirect to studio with tech niche, got: %s", loc)
	}
}

func TestDepartmentsListAndTaskDifferentiation(t *testing.T) {
	srv := newTestServer(t)
	agents := srv.listAgents()
	depts := srv.listDepartments(agents)

	if len(depts) < 4 {
		t.Fatalf("expected at least 4 departments, got %d", len(depts))
	}

	// Verify that Minh Thư has different tasks in Studio vs Reup vs Affiliate
	var minhThuStudioTask, minhThuReupTask, minhThuAffiliateTask string
	for _, d := range depts {
		for _, m := range d.Members {
			if strings.Contains(m.Agent.Name, "Minh Thư") {
				if d.ID == "dept-studio" {
					minhThuStudioTask = m.CurrentTask
				} else if d.ID == "dept-reup" {
					minhThuReupTask = m.CurrentTask
				} else if d.ID == "dept-affiliate" {
					minhThuAffiliateTask = m.CurrentTask
				}
			}
		}
	}

	if minhThuStudioTask == "" {
		t.Errorf("expected Minh Thư to have a task in dept-studio")
	}
	if minhThuReupTask == "" {
		t.Errorf("expected Minh Thư to have a task in dept-reup")
	}
	if minhThuAffiliateTask == "" {
		t.Errorf("expected Minh Thư to have a task in dept-affiliate")
	}

	if minhThuStudioTask == minhThuReupTask {
		t.Errorf("expected different tasks for Minh Thư in Studio vs Reup, got same: %s", minhThuStudioTask)
	}
	if minhThuStudioTask == minhThuAffiliateTask {
		t.Errorf("expected different tasks for Minh Thư in Studio vs Affiliate, got same: %s", minhThuStudioTask)
	}
}

func TestDepartmentAssign(t *testing.T) {
	srv := newTestServer(t)
	agents := srv.listAgents()
	if len(agents) == 0 {
		t.Fatalf("no agents available")
	}
	targetAgent := agents[0]

	form := url.Values{
		"agent_id":     {targetAgent.ID},
		"dept_id":      {"dept-story"},
		"role_in_dept": {"Biên Kịch Đặc Nhiệm"},
		"current_task": {"Viết câu chuyện về người thợ gốm Bát Tràng"},
		"deliverable":  {"Kịch bản 3 hồi hoàn chỉnh"},
	}

	req := httptest.NewRequest("POST", "/team/departments/assign", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.handleDeptAssign(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", w.Code)
	}

	// Verify assignment reflected
	depts := srv.listDepartments(srv.listAgents())
	found := false
	for _, d := range depts {
		if d.ID == "dept-story" {
			for _, m := range d.Members {
				if m.Agent.ID == targetAgent.ID && m.RoleInDept == "Biên Kịch Đặc Nhiệm" {
					found = true
					if m.CurrentTask != "Viết câu chuyện về người thợ gốm Bát Tràng" {
						t.Errorf("unexpected task: %s", m.CurrentTask)
					}
					break
				}
			}
		}
	}
	if !found {
		t.Errorf("assigned agent not found in dept-story")
	}
}

func TestEnterprise9DivisionsAndLiveOperations(t *testing.T) {
	srv := newTestServer(t)
	agents := srv.listAgents()

	// 1. Verify 9 elite enterprise specialists with codes & metrics
	if len(agents) < 9 {
		t.Fatalf("expected at least 9 enterprise agents, got %d", len(agents))
	}
	for _, ag := range agents {
		if ag.Code == "" {
			t.Errorf("expected agent %s to have corporate code, got empty", ag.Name)
		}
		if ag.SLA == "" || ag.KPI == "" {
			t.Errorf("expected agent %s to have SLA & KPI, got SLA=%q, KPI=%q", ag.Name, ag.SLA, ag.KPI)
		}
	}

	// 2. Verify all 9 corporate divisions (DEP-01 to DEP-09)
	depts := srv.listDepartments(agents)
	if len(depts) != 9 {
		t.Fatalf("expected exactly 9 corporate divisions, got %d", len(depts))
	}

	expectedCodes := map[string]string{
		"dept-exec":         "DEP-01",
		"dept-intelligence": "DEP-02",
		"dept-affiliate":    "DEP-03",
		"dept-story":        "dept-story", // DEP-04
		"dept-creative":     "DEP-05",
		"dept-studio":       "dept-studio", // DEP-06
		"dept-audio":        "dept-audio",  // DEP-07
		"dept-reup":         "dept-reup",   // DEP-08
		"dept-analytics":    "DEP-09",
	}

	for id := range expectedCodes {
		found := false
		for _, d := range depts {
			if d.ID == id {
				found = true
				if d.SLA == "" || d.KPI == "" || d.Throughput == "" {
					t.Errorf("dept %s missing SLA/KPI/Throughput: SLA=%q, KPI=%q, Throughput=%q",
						id, d.SLA, d.KPI, d.Throughput)
				}
				if !d.Active247 {
					t.Errorf("dept %s expected to be Active247=true", id)
				}
				if len(d.Members) == 0 {
					t.Errorf("dept %s has no assigned members", id)
				}
				break
			}
		}
		if !found {
			t.Errorf("missing corporate division %s", id)
		}
	}

	// 3. Verify LiveOperations 24/7 stream
	liveOps := srv.listLiveOperations()
	if len(liveOps) == 0 {
		t.Fatalf("expected live operation events feed, got 0")
	}
	for _, op := range liveOps {
		if op.DeptCode == "" || op.AgentName == "" || op.Action == "" {
			t.Errorf("invalid live operation event: %+v", op)
		}
	}
}
