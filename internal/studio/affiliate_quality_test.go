package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestAffiliateShotCount(t *testing.T) {
	for _, tc := range []struct{ seconds, want int }{{30, 5}, {45, 8}, {60, 10}, {180, 30}} {
		if got := affiliateShotCount(tc.seconds); got != tc.want {
			t.Errorf("affiliateShotCount(%d) = %d, want %d", tc.seconds, got, tc.want)
		}
	}
}

func TestRunAffiliateRejectsStoredShotDurationMismatchBeforeRendering(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	brief := AffiliateProductProfile{Category: "apparel", UseAction: "wear", Focus: "visible buttons"}
	p := AffiliateParams{
		Mode: AffiliateModeShots, Seconds: 60, PreparedProductBrief: &brief,
		PreparedShots: []ProShot{{Seconds: 6}, {Seconds: 6}, {Seconds: 6}, {Seconds: 6}, {Seconds: 6}},
	}
	id, err := st.insertJob(KindAffiliate, "mismatched shot plan", p)
	if err != nil {
		t.Fatal(err)
	}

	st.runAffiliate(context.Background(), id, p, false)
	job, ok := st.GetJob(id)
	if !ok || job.Status != StatusFailed || !strings.Contains(job.Log, "shot plan không phủ đúng 60 giây") {
		t.Fatalf("mismatched stored job was not rejected before rendering: %+v", job)
	}
}

func TestAffiliateShotPlanMatchesDurationRequiresExactSupportedCoverage(t *testing.T) {
	valid := []ProShot{{Seconds: 6}, {Seconds: 6}, {Seconds: 6}, {Seconds: 6}, {Seconds: 6}}
	if !affiliateShotPlanMatchesDuration(30, valid) {
		t.Fatal("five six-second shots should cover 30 seconds")
	}
	if affiliateShotPlanMatchesDuration(60, valid) {
		t.Fatal("30 seconds of shots must not claim 60-second coverage")
	}
	unsupported := append([]ProShot(nil), valid...)
	unsupported[0].Seconds = 9
	if affiliateShotPlanMatchesDuration(33, unsupported) {
		t.Fatal("shot longer than Muse limit must not pass plan coverage")
	}
}

func TestWriteAffiliateShotListRequiresDistinctEmotionalCoverage(t *testing.T) {
	reply := `{"scene_setting":"one calm cafe with soft window light","wardrobe_styling":"cream knit outfit that keeps the cardigan dominant","shots":[
	{"purpose":"hook","shot_size":"CU","camera_move":"static-focus","lens_light":"50mm soft window light","action":"Mẫu chạm tay lên hàng khuy và nhìn xuống","emotion_beat":"tò mò dịu","product_use_mode":"wear","product_interaction":"chạm hàng khuy cardigan đang mặc","product_focus":"hàng khuy phía trước","image_prompt":"model wearing the exact cardigan","video_prompt":"one continuous shot, subtle hand movement"},
	{"purpose":"build","shot_size":"WS","camera_move":"smooth gimbal tracking","lens_light":"35mm warm window light","action":"Mẫu bước qua quán trong cardigan","emotion_beat":"thư thái","product_use_mode":"wear","product_interaction":"đi bộ trong cardigan đang mặc","product_focus":"phom toàn thân của cardigan","image_prompt":"full body model wearing cardigan","video_prompt":"one continuous walking shot"},
	{"purpose":"build","shot_size":"MS","camera_move":"gentle pan","lens_light":"50mm soft side light","action":"Mẫu xoay người để lộ vạt áo","emotion_beat":"thích thú","product_use_mode":"wear","product_interaction":"xoay nhẹ để vạt cardigan chuyển động","product_focus":"vạt áo khi chuyển động","image_prompt":"model turns in cardigan","video_prompt":"one continuous turn"},
	{"purpose":"macro","shot_size":"ECU","camera_move":"locked-off tripod","lens_light":"macro softbox","action":"Tay mẫu cài một khuy áo","emotion_beat":"chăm chút","product_use_mode":"wear","product_interaction":"cài khuy đang thấy trên cardigan","product_focus":"chi tiết khuy nhìn thấy trong listing","image_prompt":"macro button detail","video_prompt":"one continuous close detail"},
	{"purpose":"payoff","shot_size":"MCU","camera_move":"locked-off tripod","lens_light":"50mm warm light","action":"Mẫu mỉm cười nhẹ sau khi chỉnh tay áo","emotion_beat":"tự tin, hài lòng","product_use_mode":"wear","product_interaction":"vuốt nhẹ tay áo cardigan đang mặc","product_focus":"tay áo và phom thân trên","image_prompt":"model smiles wearing cardigan","video_prompt":"one continuous final beat"}]}`
	profile := AffiliateProductProfile{Category: "apparel", UseAction: "wear the cardigan", Focus: "front buttons", Evidence: []string{"visible buttons"}}
	shots, err := WriteAffiliateShotList(context.Background(), &stubLLM{reply: reply}, "Cardigan", "fashion", 30, profile)
	if err != nil {
		t.Fatalf("valid quality plan rejected: %v", err)
	}
	if len(shots) != 5 || shots[0].Seconds+shots[1].Seconds+shots[2].Seconds+shots[3].Seconds+shots[4].Seconds != 30 {
		t.Fatalf("plan failed duration/coverage contract: %+v", shots)
	}
	if shots[0].SceneSetting == "" || shots[0].WardrobeStyling == "" || shots[0].EmotionBeat == "" {
		t.Fatalf("plan lost campaign continuity or emotional direction: %+v", shots[0])
	}
	duplicateAction := strings.Replace(reply, "Mẫu bước qua quán trong cardigan", "Mẫu chạm tay lên hàng khuy và nhìn xuống", 1)
	if _, err := WriteAffiliateShotList(context.Background(), &stubLLM{reply: duplicateAction}, "Cardigan", "fashion", 30, profile); err == nil {
		t.Fatal("duplicate action should be rejected")
	}
	var shortPlan struct {
		SceneSetting    string    `json:"scene_setting"`
		WardrobeStyling string    `json:"wardrobe_styling"`
		Shots           []ProShot `json:"shots"`
	}
	if err := json.Unmarshal([]byte(reply), &shortPlan); err != nil {
		t.Fatal(err)
	}
	shortPlan.Shots = shortPlan.Shots[:len(shortPlan.Shots)-1]
	shortReply, err := json.Marshal(shortPlan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteAffiliateShotList(context.Background(), &stubLLM{reply: string(shortReply)}, "Cardigan", "fashion", 30, profile); err == nil || !strings.Contains(err.Error(), "cần đúng 5") {
		t.Fatalf("wrong shot count error = %v, want exact five-shot rejection", err)
	}
}

func TestAffiliateQCRequiresThreeSharpProfessionalSamplesPerShot(t *testing.T) {
	shots := []ProShot{
		{Index: 1, Purpose: "hook", ShotSize: "CU"},
		{Index: 2, Purpose: "build", ShotSize: "WS"},
		{Index: 3, Purpose: "macro", ShotSize: "ECU"},
		{Index: 4, Purpose: "build", ShotSize: "MS"},
		{Index: 5, Purpose: "payoff", ShotSize: "MCU"},
	}
	report := passedAffiliateQCReport()
	if !affiliateQCFieldsPass(report, 15, shots) {
		t.Fatal("complete three-frame-per-shot QC should pass")
	}
	report.FrameEvidence[3].Focus = "soft"
	if affiliateQCFieldsPass(report, 15, shots) {
		t.Fatal("soft focus must block completion")
	}
}

func TestAffiliateQCRejectsUncertainIdentityEmotionOrContinuity(t *testing.T) {
	shots := []ProShot{
		{Index: 1, Purpose: "hook", ShotSize: "CU"},
		{Index: 2, Purpose: "build", ShotSize: "WS"},
		{Index: 3, Purpose: "macro", ShotSize: "ECU"},
		{Index: 4, Purpose: "build", ShotSize: "MS"},
		{Index: 5, Purpose: "payoff", ShotSize: "MCU"},
	}
	tests := []struct {
		name  string
		frame int
		set   func(*VisualQCFrameEvidence)
	}{
		{name: "non-macro identity", frame: 3, set: func(frame *VisualQCFrameEvidence) { frame.CharacterIdentity = "uncertain" }},
		{name: "non-macro emotion", frame: 4, set: func(frame *VisualQCFrameEvidence) { frame.EmotionMatch = "partial" }},
		{name: "non-macro continuity", frame: 0, set: func(frame *VisualQCFrameEvidence) { frame.Continuity = "uncertain" }},
		{name: "macro identity", frame: 6, set: func(frame *VisualQCFrameEvidence) { frame.CharacterIdentity = "uncertain" }},
		{name: "macro emotion", frame: 6, set: func(frame *VisualQCFrameEvidence) { frame.EmotionMatch = "uncertain" }},
		{name: "macro continuity", frame: 6, set: func(frame *VisualQCFrameEvidence) { frame.Continuity = "uncertain" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := passedAffiliateQCReport()
			tt.set(&report.FrameEvidence[tt.frame])
			if affiliateQCFieldsPass(report, 15, shots) {
				t.Fatal("affiliate QC passed without required frame evidence")
			}
		})
	}
}

func TestAffiliateQCAllowsUnobservableMacroEvidence(t *testing.T) {
	shots := []ProShot{
		{Index: 1, Purpose: "hook", ShotSize: "CU"},
		{Index: 2, Purpose: "build", ShotSize: "WS"},
		{Index: 3, Purpose: "macro", ShotSize: "ECU"},
		{Index: 4, Purpose: "build", ShotSize: "MS"},
		{Index: 5, Purpose: "payoff", ShotSize: "MCU"},
	}
	report := passedAffiliateQCReport()
	for i := 6; i < 9; i++ {
		report.FrameEvidence[i].CharacterIdentity = "not_visible"
		report.FrameEvidence[i].EmotionMatch = "not_applicable"
		report.FrameEvidence[i].Continuity = "not_applicable"
	}
	if !affiliateQCFieldsPass(report, 15, shots) {
		t.Fatal("affiliate QC rejected valid macro frames without observable face or surroundings")
	}
}

func TestAffiliateShotSampleTimelineUsesEarlyMiddleLateFrames(t *testing.T) {
	times, groups := affiliateShotSampleTimeline([]ProShot{{Seconds: 6}, {Seconds: 4}})
	wantTimes := []float64{1.2, 3, 4.8, 6.8, 8, 9.2}
	wantGroups := []int{1, 1, 1, 2, 2, 2}
	if len(times) != len(wantTimes) || len(groups) != len(wantGroups) {
		t.Fatalf("timeline lengths = %d/%d, want %d/%d", len(times), len(groups), len(wantTimes), len(wantGroups))
	}
	for i := range wantTimes {
		if math.Abs(times[i]-wantTimes[i]) > 0.0001 || groups[i] != wantGroups[i] {
			t.Errorf("sample %d = %.2f/group %d, want %.2f/group %d", i, times[i], groups[i], wantTimes[i], wantGroups[i])
		}
	}
}

func passedAffiliateQCReport() VisualQCReport {
	r := VisualQCReport{
		Decision: "pass", Summary: "all five shots show correct use and clean presentation",
		ProductMatch: "match", ProductUsage: "correct", ProductVisibility: "clear",
		CharacterIdentity: "match", Motion: "visible", ShotVariety: "varied",
		Sharpness: "sharp", Composition: "professional", EmotionalArc: "coherent", Continuity: "consistent",
	}
	for shot := 1; shot <= 5; shot++ {
		for sample := 0; sample < 3; sample++ {
			identity, emotion, continuity := "match", "match", "match"
			if shot == 3 {
				identity, emotion, continuity = "not_visible", "not_applicable", "not_applicable"
			}
			r.FrameEvidence = append(r.FrameEvidence, VisualQCFrameEvidence{
				Frame: len(r.FrameEvidence) + 1, Shot: shot,
				ProductVisibility: "clear", ProductUse: "correct",
				CharacterIdentity: identity, Focus: "sharp", Composition: "professional",
				EmotionMatch: emotion, Continuity: continuity,
				Evidence: fmt.Sprintf("shot %d sample %d", shot, sample+1),
			})
		}
	}
	return r
}
