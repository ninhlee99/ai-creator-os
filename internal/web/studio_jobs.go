package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"strings"
	"time"
)

// (R2-W3), JSON for polling when Accept: application/json.
func (s *Server) handleStudioJobs(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		var jobs []studioJobView
		if ids := r.URL.Query()["id"]; len(ids) > 0 {
			jobs = make([]studioJobView, 0, len(ids))
			seen := make(map[string]struct{}, len(ids))
			for _, id := range ids {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				if s.Studio == nil {
					continue
				}
				j, ok := s.Studio.GetJob(id)
				if ok {
					jobs = append(jobs, s.studioJobViews([]studio.Job{j})[0])
				}
			}
		} else if s.Studio != nil {
			jobs = s.studioJobViews(activeStudioJobs(s.Studio.ListJobs(50)))
		}
		s.writeJSON(w, map[string]any{"jobs": jobs})
		return
	}

	var allJobs []studioJobView
	jobID := strings.TrimSpace(r.URL.Query().Get("id"))
	if jobID == "" {
		jobID = strings.TrimSpace(r.URL.Query().Get("job_id")) // legacy create redirect
	}
	if s.Studio != nil {
		jobs := s.Studio.ListJobs(50)
		if jobID != "" {
			found := false
			for _, job := range jobs {
				if job.ID == jobID {
					found = true
					break
				}
			}
			if !found {
				if job, ok := s.Studio.GetJob(jobID); ok {
					jobs = append(jobs, job)
				}
			}
		}
		allJobs = s.studioJobViews(jobs)
	}

	var runningJobs, doneJobs, failedJobs []studioJobView
	for _, j := range allJobs {
		switch j.Status {
		case "running", "queued":
			runningJobs = append(runningJobs, j)
		case "done":
			doneJobs = append(doneJobs, j)
		case "failed":
			failedJobs = append(failedJobs, j)
		}
	}

	tab := r.URL.Query().Get("tab")
	if jobID != "" && s.Studio != nil {
		if job, ok := s.Studio.GetJob(jobID); ok {
			switch job.Status {
			case studio.StatusQueued, studio.StatusRunning:
				tab = "running"
			case studio.StatusDone:
				tab = "done"
			case studio.StatusFailed:
				tab = "failed"
			}
		}
	}
	if tab == "" {
		if len(runningJobs) > 0 {
			tab = "running"
		} else if len(failedJobs) > 0 {
			tab = "failed"
		} else {
			tab = "done"
		}
	}
	msg := r.URL.Query().Get("msg")
	if msg == "created" {
		msg = "Job video đã được đưa vào hàng đợi Studio."
	}

	var displayedJobs []studioJobView
	switch tab {
	case "running":
		displayedJobs = runningJobs
	case "done":
		displayedJobs = doneJobs
	case "failed":
		displayedJobs = failedJobs
	case "schedule":
		displayedJobs = runningJobs
	default:
		displayedJobs = allJobs
	}

	schedEnabled, schedHours, schedLastRun, schedAutoPub := s.scheduleView()

	nextRunStr := "Đang chờ lịch kích hoạt"
	if schedEnabled {
		if schedLastRun != "" {
			if t, err := time.Parse(time.RFC3339, schedLastRun); err == nil {
				next := t.Add(time.Duration(schedHours) * time.Hour)
				if next.After(time.Now()) {
					nextRunStr = next.Format("15:04 ngày 02/01/2006")
				} else {
					nextRunStr = "Sắp chạy ngay trong vài phút tới"
				}
			}
		} else {
			nextRunStr = "Sắp kích hoạt trong chu kỳ tiếp theo"
		}
	} else {
		nextRunStr = "Tạm dừng (Lịch tự động đang tắt)"
	}

	s.render(w, "studio_jobs", s.ctx(
		"Title", "Quản lý Jobs & Lịch Studio",
		"Path", "/studio/jobs",
		"Jobs", displayedJobs,
		"AllJobs", allJobs,
		"RunningJobs", runningJobs,
		"DoneJobs", doneJobs,
		"FailedJobs", failedJobs,
		"RunningCount", len(runningJobs),
		"DoneCount", len(doneJobs),
		"FailedCount", len(failedJobs),
		"TotalCount", len(allJobs),
		"Tab", tab,
		"CurrentTab", tab,
		"SchedEnabled", schedEnabled,
		"SchedHours", schedHours,
		"SchedLastRun", schedLastRun,
		"SchedNextRun", nextRunStr,
		"SchedAutoPub", schedAutoPub,
		"Msg", msg,
		"Err", r.URL.Query().Get("err"),
	))
}

// activeStudioJobs keeps status-only polling focused on work still in progress.
func activeStudioJobs(jobs []studio.Job) []studio.Job {
	active := make([]studio.Job, 0, len(jobs))
	for _, job := range jobs {
		if job.Status == studio.StatusQueued || job.Status == studio.StatusRunning {
			active = append(active, job)
		}
	}
	return active
}
