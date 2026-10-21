package web

import (
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestActiveStudioJobsOnlyIncludesQueuedAndRunning(t *testing.T) {
	jobs := []studio.Job{
		{ID: "queued", Status: studio.StatusQueued},
		{ID: "running", Status: studio.StatusRunning},
		{ID: "done", Status: studio.StatusDone},
		{ID: "failed", Status: studio.StatusFailed},
	}
	got := activeStudioJobs(jobs)
	if len(got) != 2 || got[0].ID != "queued" || got[1].ID != "running" {
		t.Fatalf("activeStudioJobs = %+v, want queued and running only", got)
	}
}

func TestStudioPollScriptTargetsLiveCardsOnly(t *testing.T) {
	js := string(staticJS)
	if !strings.Contains(js, ".studio-live-job[data-job]") {
		t.Fatal("poller must target only explicitly marked processing cards")
	}
	if strings.Contains(js, ".studio-job-card[data-job]") {
		t.Fatal("poller must not select generic cards that may appear on completed tabs")
	}
}
