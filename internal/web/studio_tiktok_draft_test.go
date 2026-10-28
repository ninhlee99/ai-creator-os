package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestStudioOutputQCVerifiedUsesProducerMarkers(t *testing.T) {
	tests := []struct {
		name string
		job  studio.Job
	}{
		{
			name: "affiliate",
			job: studio.Job{
				Kind: studio.KindAffiliate, Status: studio.StatusDone, Output: "affiliate.mp4",
				Log: strings.Join([]string{
					"PRODUCT_ANALYSIS_START:",
					"PRODUCT_ANALYSIS_PASS:",
					"DIRECTOR_START:",
					"DIRECTOR_PASS:",
					"WORKFLOW_START:",
					"WORKFLOW_PASS:",
					"VISUAL_QC_START:",
					"VISUAL_QC_PASS:",
				}, "\n"),
			},
		},
		{
			name: "viral",
			job: studio.Job{
				Kind: studio.KindViral, Status: studio.StatusDone, Output: "viral.mp4",
				Log: strings.Join([]string{
					"WORKFLOW_START:",
					"WORKFLOW_OUTPUT_PASS:",
					"VISUAL_QC_START:",
					"VISUAL_QC_PASS:",
				}, "\n"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !studioOutputQCVerified(tt.job) {
				t.Fatalf("successful producer log rejected for %s job", tt.name)
			}
		})
	}
}

func TestStudioOutputQCVerifiedRejectsMissingOrOutOfOrderStage(t *testing.T) {
	base := studio.Job{
		Kind: studio.KindAffiliate, Status: studio.StatusDone, Output: "affiliate.mp4",
		Log: strings.Join([]string{
			"PRODUCT_ANALYSIS_START:",
			"PRODUCT_ANALYSIS_PASS:",
			"DIRECTOR_START:",
			"DIRECTOR_PASS:",
			"WORKFLOW_START:",
			"WORKFLOW_PASS:",
			"VISUAL_QC_START:",
			"VISUAL_QC_PASS:",
		}, "\n"),
	}
	if !studioOutputQCVerified(base) {
		t.Fatal("valid affiliate producer log rejected")
	}

	validLog := base.Log
	base.Log = strings.Replace(base.Log, "WORKFLOW_PASS:", "PRODUCTION_PASS:", 1)
	if studioOutputQCVerified(base) {
		t.Fatal("legacy marker with no producer workflow pass was accepted")
	}

	base.Log = strings.Replace(validLog, "WORKFLOW_START:\nWORKFLOW_PASS:", "WORKFLOW_PASS:\nWORKFLOW_START:", 1)
	if studioOutputQCVerified(base) {
		t.Fatal("workflow pass before workflow start was accepted")
	}
}

func TestStudioTikTokDraftRequiresPerVideoConsent(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("POST", "/studio/jobs/job-1/tiktok-draft", strings.NewReader(""))
	r.SetPathValue("id", "job-1")
	w := httptest.NewRecorder()

	s.handleStudioTikTokDraft(w, r)

	if w.Code != 303 {
		t.Fatalf("status = %d, want redirect", w.Code)
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("redirect location: %v", err)
	}
	if got := location.Query().Get("err"); !strings.Contains(got, "xem trước và đồng ý") {
		t.Fatalf("error = %q, want explicit preview and consent prompt", got)
	}
}
