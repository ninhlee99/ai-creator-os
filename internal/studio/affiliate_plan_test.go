package studio

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type affiliatePlanMedia struct{ analyzeCalls int }
type affiliatePlanLLM struct {
	reply string
	calls int
}

func (l *affiliatePlanLLM) Complete(context.Context, string, string) (string, error) {
	l.calls++
	return l.reply, nil
}
func (*affiliatePlanLLM) Name() string                 { return "affiliate-plan-test" }
func (*affiliatePlanLLM) Healthy(context.Context) bool { return true }

func (m *affiliatePlanMedia) GenerateImage(context.Context, string, []ImageRef, string) error {
	return nil
}
func (m *affiliatePlanMedia) GenerateVideo(context.Context, string, string, int, string, string) error {
	return nil
}
func (m *affiliatePlanMedia) Name() string                 { return "affiliate-plan-test" }
func (m *affiliatePlanMedia) Healthy(context.Context) bool { return true }
func (m *affiliatePlanMedia) AnalyzeImages(context.Context, string, []ImageRef) (string, error) {
	m.analyzeCalls++
	return `{"product_match":"match","category":"apparel","item":"cardigan","visible_attributes":["front buttons","long sleeves"],"evidence":["front button row is visible"],"confidence":"high","requested_focus_support":"match","selected_focus":"front button row","focus_evidence":"front button row is visible","operator_use_support":"match"}`, nil
}

func TestAffiliatePlanIsSavedAndReusedByJob(t *testing.T) {
	dir := t.TempDir()
	media := &affiliatePlanMedia{}
	llm := &affiliatePlanLLM{reply: `{"scene_setting":"quiet cafe with soft window light","wardrobe_styling":"neutral styling that keeps cardigan clear","shots":[
	{"purpose":"hook","shot_size":"CU","camera_move":"slow push-in","lens_light":"50mm soft window light","action":"Mẫu chạm hàng khuy rồi nhìn lên","emotion_beat":"tò mò","product_use_mode":"wear","product_interaction":"chạm hàng khuy cardigan đang mặc","product_focus":"hàng khuy trước","image_prompt":"same channel character wearing cardigan","video_prompt":"slow push-in as hand touches cardigan buttons"},
	{"purpose":"build","shot_size":"WS","camera_move":"tracking","lens_light":"35mm daylight","action":"Mẫu bước qua quán","emotion_beat":"thư thái","product_use_mode":"wear","product_interaction":"đi bộ khi đang mặc cardigan","product_focus":"phom áo trên toàn thân","image_prompt":"same channel character walking in cardigan","video_prompt":"track one natural walk in cardigan"},
	{"purpose":"build","shot_size":"MS","camera_move":"gentle pan","lens_light":"50mm side light","action":"Mẫu xoay vai để vạt áo chuyển động","emotion_beat":"thích thú","product_use_mode":"wear","product_interaction":"xoay nhẹ khi mặc cardigan","product_focus":"vạt áo chuyển động","image_prompt":"same channel character turns in cardigan","video_prompt":"pan with one smooth shoulder turn"},
	{"purpose":"macro","shot_size":"ECU","camera_move":"locked tripod","lens_light":"macro softbox","action":"Tay mẫu cài một khuy","emotion_beat":"chăm chút","product_use_mode":"wear","product_interaction":"cài khuy nhìn thấy trên cardigan","product_focus":"chi tiết khuy áo","image_prompt":"macro of exact cardigan button","video_prompt":"one hand fastens one visible cardigan button"},
	{"purpose":"payoff","shot_size":"MCU","camera_move":"slow dolly-out","lens_light":"50mm warm light","action":"Mẫu chỉnh tay áo rồi mỉm cười","emotion_beat":"hài lòng","product_use_mode":"wear","product_interaction":"vuốt nhẹ tay áo cardigan đang mặc","product_focus":"tay áo và phom thân trên","image_prompt":"same channel character wearing cardigan","video_prompt":"slow dolly-out as model adjusts sleeve and smiles"}]}`}
	st, err := New(filepath.Join(dir, "studio.db"), llm, media, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.persistTikTokRadarResults(context.Background(), []TikTokTrendVideo{{
		VideoID: "1234567890", Title: "Viral dance", Music: "Radar sound",
		VideoURL: "https://www.tiktok.com/@creator/video/1234567890", PlayCount: 1000000,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetTikTokRadarDanceVerification(context.Background(), "1234567890", true); err != nil {
		t.Fatal(err)
	}
	modelSource := filepath.Join(dir, "model.png")
	productPhoto := filepath.Join(dir, "product.png")
	if err := os.WriteFile(modelSource, []byte("model image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(productPhoto, []byte("listing image"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := st.ModelLibrary()
	if err != nil {
		t.Fatal(err)
	}
	model, err := library.Add(42, modelSource)
	if err != nil {
		t.Fatal(err)
	}
	prep, err := st.PrepareAffiliatePlan(context.Background(), AffiliateParams{
		Mode: AffiliateModeShots, VideoEngine: "manual", AccountID: 42,
		ProductName: "Cardigan M5091", Niche: "thời trang", ProductCategory: "apparel",
		ProductUsage: "nhân vật mặc cardigan và cài khuy", ProductFocus: "hàng khuy phía trước",
		ProductPhoto: productPhoto, ModelPhoto: model.Path, ModelPhotos: []string{model.Path}, Seconds: 30,
		SourceMusicVideoURL: "https://www.tiktok.com/@creator/video/1234567890",
	})
	if err != nil {
		t.Fatalf("PrepareAffiliatePlan: %v", err)
	}
	if len(prep.Shots) != 5 || prep.ProductBrief.Category != "apparel" {
		t.Fatalf("unexpected preparation: %+v", prep)
	}
	if prep.Params.SourceMusicTitle != "Radar sound" || prep.Params.SourceMusicVideoURL != "https://www.tiktok.com/@creator/video/1234567890" {
		t.Fatalf("shot plan did not retain Radar sound source: %+v", prep.Params)
	}
	loaded, err := st.GetAffiliatePreparation(prep.ID)
	if err != nil || len(loaded.Shots) != len(prep.Shots) {
		t.Fatalf("saved plan load = %d shots, err %v", len(loaded.Shots), err)
	}
	badClip := filepath.Join(dir, "not-a-video.mp4")
	if err := os.WriteFile(badClip, []byte("not a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	badClips := make([]string, len(prep.Shots))
	for i := range badClips {
		badClips[i] = badClip
	}
	_, err = st.CreateAffiliateJob(AffiliateParams{
		PreparedPlanID: prep.ID, AccountID: 42, Mode: AffiliateModeShots, Clips: badClips,
		FlowCreditsAcknowledged: true,
	})
	if err == nil || !strings.Contains(err.Error(), "clip 1") {
		t.Fatalf("CreateAffiliateJob with corrupt clip error = %v, want synchronous clip rejection", err)
	}
	if _, err := st.GetAffiliatePreparation(prep.ID); err != nil {
		t.Fatalf("corrupt clip consumed approved plan: %v", err)
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable for valid clip fixture")
	}
	validClip := filepath.Join(dir, "valid-clip.mp4")
	if output, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=32x32:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", validClip).CombinedOutput(); err != nil {
		t.Fatalf("create valid clip fixture: %v: %s", err, output)
	}
	clips := make([]string, len(prep.Shots))
	for i := range clips {
		clips[i] = validClip
	}
	var readinessChecks atomic.Int32
	st.SetWebAIMuseRenderer(func(context.Context, *WebAISession, MuseChatRequest, string, func(string)) (string, error) {
		return "unused-output.mp4", nil
	})
	st.SetWebAIVideoRenderer(nil, func(string) (bool, string) {
		if readinessChecks.Add(1) == 1 {
			return true, "ready"
		}
		return false, "test readiness changed after job submission"
	})
	jobID, err := st.CreateAffiliateJob(AffiliateParams{
		PreparedPlanID: prep.ID, AccountID: 42, Mode: AffiliateModeShots, Clips: clips,
		FlowCreditsAcknowledged: true,
		SourceMusicTitle:        "untrusted form title", SourceMusicVideoURL: "https://www.tiktok.com/@other/video/9999999999",
	})
	if err != nil {
		t.Fatalf("CreateAffiliateJob with saved plan: %v", err)
	}
	job, ok := st.GetJob(jobID)
	if !ok {
		t.Fatal("job missing after create")
	}
	var saved AffiliateParams
	if err := json.Unmarshal([]byte(job.Params), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.SourceMusicTitle != "Radar sound" || saved.SourceMusicVideoURL != "https://www.tiktok.com/@creator/video/1234567890" {
		t.Fatalf("job accepted untrusted sound source: title=%q URL=%q", saved.SourceMusicTitle, saved.SourceMusicVideoURL)
	}
	if len(saved.PreparedShots) != len(prep.Shots) || saved.PreparedProductBrief == nil || saved.PreparedPlanID != "" {
		t.Fatalf("job did not retain consumed plan: shots=%d brief=%v plan_id=%q", len(saved.PreparedShots), saved.PreparedProductBrief != nil, saved.PreparedPlanID)
	}
	if _, err := st.GetAffiliatePreparation(prep.ID); err == nil {
		t.Fatal("consumed plan remains reusable")
	}
	deadline := time.Now().Add(3 * time.Second)
	failed := false
	for time.Now().Before(deadline) {
		job, _ = st.GetJob(jobID)
		if job.Status == StatusFailed {
			failed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !failed {
		t.Fatal("worker did not stop when Muse readiness changed")
	}
	if media.analyzeCalls != 1 || llm.calls != 1 {
		t.Fatalf("job should reuse plan; analyzer calls=%d director calls=%d", media.analyzeCalls, llm.calls)
	}

	st.SetWebAIVideoRenderer(func(context.Context, *WebAISession, WebAIVideoRequest, string, func(string)) (string, error) {
		return "unused-project", nil
	}, func(string) (bool, string) { return true, "ready" })
	flowPrep, err := st.PrepareAffiliatePlan(context.Background(), AffiliateParams{
		Mode: AffiliateModeShots, VideoEngine: AffiliateVideoEngineGoogleFlow, AccountID: 42,
		ProductName: "Cardigan M5091", Niche: "thời trang", ProductCategory: "apparel",
		ProductUsage: "nhân vật mặc cardigan và cài khuy", ProductFocus: "hàng khuy phía trước",
		ProductPhoto: productPhoto, ModelPhoto: model.Path, ModelPhotos: []string{model.Path}, Seconds: 30,
		SourceMusicVideoURL: "https://www.tiktok.com/@creator/video/1234567890",
	})
	if err != nil {
		t.Fatalf("PrepareAffiliatePlan for Flow: %v", err)
	}
	if _, err := st.CreateAffiliateJob(AffiliateParams{PreparedPlanID: flowPrep.ID, AccountID: 42}); err == nil || !strings.Contains(err.Error(), "credits") {
		t.Fatalf("Flow job without request consent error = %v, want credits consent requirement", err)
	}
	if _, err := st.GetAffiliatePreparation(flowPrep.ID); err != nil {
		t.Fatalf("rejected Flow job consumed its prepared plan: %v", err)
	}

	retryPrep, err := st.PrepareAffiliatePlan(context.Background(), flowPrep.Params)
	if err != nil {
		t.Fatalf("PrepareAffiliatePlan for disk guard: %v", err)
	}
	st.DiskGuard = func() (int, error) { return 99, nil }
	retryParams := retryPrep.Params
	retryParams.PreparedPlanID = retryPrep.ID
	retryParams.FlowCreditsAcknowledged = true
	if _, err := st.CreateAffiliateJob(retryParams); err == nil || !strings.Contains(err.Error(), "đĩa đã dùng") {
		t.Fatalf("CreateAffiliateJob at disk limit error = %v, want disk guard", err)
	}
	if _, err := st.GetAffiliatePreparation(retryPrep.ID); err != nil {
		t.Fatalf("disk guard consumed retryable shot plan: %v", err)
	}
	if _, ok := st.GetJob(retryPrep.ID); ok {
		t.Fatal("disk guard inserted a partial affiliate job")
	}
	st.DiskGuard = nil
	dbRetryPrep, err := st.PrepareAffiliatePlan(context.Background(), retryPrep.Params)
	if err != nil {
		t.Fatalf("PrepareAffiliatePlan for database rollback: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO studio_jobs(id,kind,title,status,params,created_at) VALUES(?,?,?,?,?,?)`,
		dbRetryPrep.ID, "sentinel", "existing row", StatusQueued, "{}", time.Now().Format("2006-01-02T15:04:05")); err != nil {
		t.Fatalf("insert duplicate job ID fixture: %v", err)
	}
	dbRetryParams := dbRetryPrep.Params
	dbRetryParams.PreparedPlanID = dbRetryPrep.ID
	dbRetryParams.FlowCreditsAcknowledged = true
	if _, err := st.CreateAffiliateJob(dbRetryParams); err == nil {
		t.Fatal("CreateAffiliateJob succeeded with duplicate job ID")
	}
	if _, err := st.GetAffiliatePreparation(dbRetryPrep.ID); err != nil {
		t.Fatalf("database insert failure consumed retryable shot plan: %v", err)
	}
}
