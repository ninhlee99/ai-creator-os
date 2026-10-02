package web

import (
	"embed"
	"net/http"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

//go:embed static/agents/*.jpg
var agentAvatars embed.FS

// Team page: visual tree of the agent team for one selected task.
// One click on a task -> the whole team tree lights up with live states:
// done (green check), working (pulsing), exchanging (animated flow),
// waiting (grey), failed (red). Deliberately icon-first, minimal text
// (owner request 2026-10-02: too much text hurts the eyes).

type teamAgent struct {
	Key    string // avatar file stem + status key
	Name   string
	Status string // done|working|exchange|waiting|failed|guard
}

type teamTask struct {
	ID       string
	Title    string
	Kind     string
	Status   string
	Progress int
	Icon     string // status glyph for the list
	Agents   []teamAgent
}

var teamPipeline = []struct{ Key, Name string }{
	{"hunter", "Hunter"},
	{"director", "Director"},
	{"producer", "Producer"},
	{"qc", "QC"},
	{"publisher", "Publisher"},
	{"analyst", "Analyst"},
}

// stageFor maps job progress to the pipeline stage currently working.
func stageFor(status string, progress int) int {
	switch status {
	case "done":
		return len(teamPipeline) // all done
	case "queued", "pending":
		return -1 // orchestrator only
	case "failed":
		// stage that failed, approximated from progress
		switch {
		case progress < 30:
			return 1
		case progress < 70:
			return 2
		case progress < 90:
			return 3
		default:
			return 4
		}
	default: // running
		switch {
		case progress < 15:
			return 0
		case progress < 35:
			return 1
		case progress < 70:
			return 2
		case progress < 90:
			return 3
		default:
			return 4
		}
	}
}

func agentsFor(status string, progress int) []teamAgent {
	stage := stageFor(status, progress)
	out := make([]teamAgent, 0, len(teamPipeline))
	for i, p := range teamPipeline {
		st := "waiting"
		switch {
		case status == "done" || i < stage:
			st = "done"
		case status == "failed" && i == stage:
			st = "failed"
		case status == "failed" && i > stage:
			st = "waiting"
		case i == stage:
			st = "working"
		case i == stage+1 && stage >= 0:
			st = "exchange" // receiving the handoff, about to work
		}
		out = append(out, teamAgent{Key: p.Key, Name: p.Name, Status: st})
	}
	return out
}

func taskIcon(status string) string {
	switch status {
	case "done":
		return "✓"
	case "failed":
		return "!"
	case "running":
		return "▶"
	default:
		return "…"
	}
}

func (s *Server) teamTasks(r *http.Request) []teamTask {
	var tasks []teamTask
	if s.Studio != nil {
		for _, j := range s.Studio.ListJobs(12) {
			tasks = append(tasks, teamTask{
				ID: j.ID, Title: j.Title, Kind: j.Kind, Status: j.Status,
				Progress: j.Progress, Icon: taskIcon(j.Status),
				Agents: agentsFor(j.Status, j.Progress),
			})
		}
	}
	if len(tasks) == 0 {
		// Demo tasks so the page is never empty before the first job.
		demo := []studio.Job{
			{ID: "demo-1", Kind: "affiliate", Title: "Video affiliate — túi kem quilted", Status: "running", Progress: 55},
			{ID: "demo-2", Kind: "affiliate", Title: "Video affiliate — váy hoa mùa hè", Status: "done", Progress: 100},
			{ID: "demo-3", Kind: "film", Title: "Phim ngắn — Người kể chuyện đêm", Status: "queued", Progress: 0},
		}
		for _, j := range demo {
			tasks = append(tasks, teamTask{
				ID: j.ID, Title: j.Title, Kind: j.Kind, Status: j.Status,
				Progress: j.Progress, Icon: taskIcon(j.Status),
				Agents: agentsFor(j.Status, j.Progress),
			})
		}
	}
	return tasks
}

func (s *Server) handleTeam(w http.ResponseWriter, r *http.Request) {
	tasks := s.teamTasks(r)
	sel := 0
	if id := r.URL.Query().Get("job"); id != "" {
		for i, t := range tasks {
			if t.ID == id {
				sel = i
				break
			}
		}
	}
	s.render(w, "team", s.ctx(
		"Tasks", tasks,
		"Selected", tasks[sel],
		"Agents", tasks[sel].Agents,
		"Guard", []teamAgent{
			{Key: "governance", Name: "Governance", Status: "guard"},
			{Key: "scheduler", Name: "Scheduler", Status: "guard"},
		},
		"RootStatus", map[string]string{"done": "done", "failed": "failed", "running": "working"}[tasks[sel].Status],
	))
}

func (s *Server) handleAgentAvatar(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if strings.Contains(name, "/") || strings.Contains(name, "..") || !strings.HasSuffix(name, ".jpg") {
		http.NotFound(w, r)
		return
	}
	b, err := agentAvatars.ReadFile("static/agents/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}
