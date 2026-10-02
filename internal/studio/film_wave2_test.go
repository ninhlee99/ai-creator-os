//go:build parked

package studio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// doubles
// ---------------------------------------------------------------------------

// seqLLM replays canned replies in order (for the 3-act director path).
type seqLLM struct {
	replies []string
	calls   int
}

func (s *seqLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	if s.calls >= len(s.replies) {
		return "", errNoMoreReplies
	}
	r := s.replies[s.calls]
	s.calls++
	return r, nil
}
func (s *seqLLM) Name() string                     { return "seq" }
func (s *seqLLM) Healthy(ctx context.Context) bool { return true }

var errNoMoreReplies = errTestSentinel()

func errTestSentinel() error { return &testSentinel{} }

type testSentinel struct{}

func (e *testSentinel) Error() string { return "no more replies" }

// stubMG implements MediaGen: images via ffmpeg testsrc, video ok/fail.
type stubMG struct {
	videoErr        error
	videoCalls      int
	lastPrompt      string
	lastFirstFrame  string
	lastSeconds     int
	lastImagePrompt string
}

func (m *stubMG) GenerateImage(ctx context.Context, prompt string, refs []ImageRef, outPath string) error {
	m.lastImagePrompt = prompt
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x240:duration=1:rate=10",
		"-frames:v", "1", outPath)
	return cmd.Run()
}

func (m *stubMG) GenerateVideo(ctx context.Context, prompt, firstFrame string, seconds int, aspect, outPath string) error {
	m.videoCalls++
	m.lastPrompt = prompt
	m.lastFirstFrame = firstFrame
	m.lastSeconds = seconds
	if m.videoErr != nil {
		return m.videoErr
	}
	w, h := AspectDims(aspect)
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=1920x1080:duration=2:rate=10",
		"-vf", "scale=1920:1080",
		"-pix_fmt", "yuv420p", outPath)
	_ = w
	_ = h
	return cmd.Run()
}

func (m *stubMG) Name() string { return "stub" }
func (m *stubMG) Healthy(ctx context.Context) bool {
	return true
}

func newWave2Studio(t *testing.T, llm LLM, mg MediaGen) *Studio {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "studio.db"), llm, mg, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ---------------------------------------------------------------------------
// shot expansion
// ---------------------------------------------------------------------------

func TestExpandSceneShotsBasic(t *testing.T) {
	sc := FilmScenePro{Index: 1, Seconds: 20, Shots: []ProShot{
		{ShotSize: "WS", Seconds: 10, ImagePrompt: "a"},
		{ShotSize: "CU", Seconds: 10, ImagePrompt: "b"},
	}}
	shots := expandSceneShots(sc, 0)
	if len(shots) != 4 {
		t.Fatalf("20s / 2 beats → 4 shots, got %d", len(shots))
	}
	total := 0
	for i, sh := range shots {
		if sh.Seconds > veoMaxSeconds {
			t.Fatalf("shot %d = %ds > %ds", i, sh.Seconds, veoMaxSeconds)
		}
		if sh.Seq != i {
			t.Fatalf("shot %d seq = %d", i, sh.Seq)
		}
		total += sh.Seconds
	}
	if total != 20 {
		t.Fatalf("total = %ds, want 20", total)
	}
}

func TestExpandSceneShotsSubdivide(t *testing.T) {
	sc := FilmScenePro{Index: 1, Seconds: 25, Shots: []ProShot{
		{ShotSize: "WS", ImagePrompt: "a"},
	}}
	shots := expandSceneShots(sc, 10)
	if len(shots) != 4 { // ceil(25/8)
		t.Fatalf("25s → 4 shots, got %d", len(shots))
	}
	total := 0
	for _, sh := range shots {
		if sh.Seconds > veoMaxSeconds {
			t.Fatalf("shot %ds > %ds", sh.Seconds, veoMaxSeconds)
		}
		total += sh.Seconds
	}
	if total != 25 {
		t.Fatalf("total = %ds, want 25", total)
	}
	if shots[0].Seq != 10 || shots[3].SubIdx != 3 {
		t.Fatalf("bad seq/subidx: %+v", shots[0])
	}
}

func TestExpandSceneShotsDialogueSplit(t *testing.T) {
	sc := FilmScenePro{
		Index: 1, Seconds: 16,
		Shots:     []ProShot{{ShotSize: "WS", ImagePrompt: "a"}},
		Dialogue:  []DialogueLine{{Character: "An", Text: "Một."}, {Character: "An", Text: "Hai."}, {Character: "An", Text: "Ba."}},
		Narration: "Lời dẫn dài.",
	}
	shots := expandSceneShots(sc, 0)
	if len(shots) != 2 {
		t.Fatalf("16s → 2 shots, got %d", len(shots))
	}
	got := 0
	for _, sh := range shots {
		got += len(sh.Dialogue)
	}
	if got != 3 {
		t.Fatalf("dialogue lines distributed = %d, want 3", got)
	}
	if strings.TrimSpace(shots[0].Narration) == "" && strings.TrimSpace(shots[1].Narration) == "" {
		t.Fatal("narration lost in subdivision")
	}
}

func TestShotSpokenText(t *testing.T) {
	sh := RenderShot{
		Dialogue:  []DialogueLine{{Text: "Chào em."}, {Text: "  "}},
		Narration: "Đêm mưa.",
	}
	if got := shotSpokenText(sh); got != "Chào em. Đêm mưa." {
		t.Fatalf("spoken = %q", got)
	}
}

func TestPerformanceBlock(t *testing.T) {
	sh := RenderShot{
		Action: "An siết chặt quai túi, mắt ráo hoảnh nhìn ra cửa.",
		Dialogue: []DialogueLine{
			{Character: "An", Text: "Anh đến rồi.", Emotion: "dịu dàng"},
			{Character: "Bình", Text: "Ừ.", Emotion: ""},
		},
	}
	got := performanceBlock(sh)
	if !strings.Contains(got, "PERFORMANCE") || !strings.Contains(got, "siết chặt quai túi") {
		t.Fatalf("performance block missing action:\n%s", got)
	}
	if !strings.Contains(got, "An shows dịu dàng") {
		t.Fatalf("performance block missing dialogue emotion:\n%s", got)
	}
	if strings.Contains(got, "Bình shows") {
		t.Fatalf("empty emotion must not emit:\n%s", got)
	}
	if got := performanceBlock(RenderShot{}); got != "" {
		t.Fatalf("empty shot must yield empty block, got %q", got)
	}
}

func TestContinuityBiblePromptLaws(t *testing.T) {
	// Pha 0 (truyện): story.txt — văn xuôi hoàn chỉnh, JSON.
	out, err := directorPrompt("story.txt", promptData{Topic: "T", Genre: "Tâm lý",
		Seconds: 90, StoryWords: 300, Orient: "horizontal 16:9"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PHA 0", `"text"`, "JSON"} {
		if !strings.Contains(out, want) {
			t.Fatalf("story.txt: missing %q", want)
		}
	}
	// Pha 1 (kịch bản): screenplay.txt + 3 hồi — luật chuyển thể, thoại trung thực.
	for _, n := range []string{"screenplay.txt", "film_act1.txt", "film_act2.txt", "film_act3.txt"} {
		out, err := directorPrompt(n, promptData{Topic: "T", Genre: "Tâm lý", Seconds: 90, N: 2,
			Orient: "horizontal 16:9", Story: "Truyện gốc..."})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"PHA 1", "THOẠI TRUNG THỰC", "TRUYỆN GỐC"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s: missing %q", n, want)
			}
		}
		// Pha 1 không viết shot — luật shot nằm ở pha 2.
		if strings.Contains(out, "trailer_worthy") {
			t.Fatalf("%s: pha 1 không được chứa luật shot", n)
		}
	}
	// Pha 2 (breakdown): luật shot + bible + continuity.
	out, err = directorPrompt("breakdown.txt", promptData{Screenplay: `{"scenes":[]}`, Orient: "horizontal 16:9"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PHA 2", "continuity", "ĐA DẠNG", "THOẠI TRUNG THỰC", "CENTER-SAFE", "trailer_worthy", "design_note"} {
		if !strings.Contains(out, want) {
			t.Fatalf("breakdown.txt: missing %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// budget
// ---------------------------------------------------------------------------

func TestVeoBudgetCheck(t *testing.T) {
	b := &veoBudget{rate: 0.05, cap: 0.10}
	if err := b.check(8); err == nil {
		t.Fatal("8s × $0.05 = $0.40 must exceed the $0.10 cap")
	}
	b2 := &veoBudget{rate: 0.05, cap: 0}
	if err := b2.check(3600); err != nil {
		t.Fatalf("no cap must never block: %v", err)
	}
	if veoRateUSD() <= 0 {
		t.Fatal("default rate must be positive")
	}
}

// ---------------------------------------------------------------------------
// trailers
// ---------------------------------------------------------------------------

func TestSelectTrailerGroups(t *testing.T) {
	var marked []renderedShot
	for i := 0; i < 3; i++ {
		marked = append(marked, renderedShot{
			Shot: RenderShot{Seq: i, Seconds: 8, TrailerWorthy: true},
			MP4:  "s.mp4", Dur: 8,
		})
	}
	groups := selectTrailerGroups(marked)
	if len(groups) != 2 {
		t.Fatalf("3 marked shots → 2 trailer groups, got %d", len(groups))
	}
	if len(groups[0]) != 2 || len(groups[1]) != 1 {
		t.Fatalf("groups = %d/%d, want 2/1", len(groups[0]), len(groups[1]))
	}
	var d0, d1 float64
	for _, m := range groups[0] {
		d0 += m.Dur
	}
	for _, m := range groups[1] {
		d1 += m.Dur
	}
	if d0 != 16 || d1 != 8 {
		t.Fatalf("durations = %.0f/%.0f, want 16/8", d0, d1)
	}
	if selectTrailerGroups(nil) != nil {
		t.Fatal("no marked shots must skip without error")
	}
}

func TestBuildTrailersSkipsWhenNone(t *testing.T) {
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	id, err := s.insertJob(KindFilm, "t", FilmParams{Topic: "t", Seconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	s.buildTrailers(context.Background(), id, FilmParams{}, t.TempDir(), nil)
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM studio_assets WHERE job_id=? AND kind='trailer'`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("no marked shots → 0 trailers, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// director: 3 acts + prompt files
// ---------------------------------------------------------------------------

func TestDirectorPromptFiles(t *testing.T) {
	names := []string{"affiliate_shotlist.txt", "story.txt", "screenplay.txt", "film_act1.txt", "film_act2.txt", "film_act3.txt"}
	data := promptData{
		Topic: "Cô gái và chiếc túi", Niche: "túi xách", Genre: "Tâm lý",
		Seconds: 90, TotalSeconds: 1800, ActSeconds: 360, N: 4,
		Orient: "horizontal 16:9", Characters: "- An: Vietnamese woman",
		Story: "Ngày xưa có một cô gái...", StoryWords: 300,
		Cast: "- An", Recap1: "r1", Recap2: "r2",
	}
	for _, n := range names {
		out, err := directorPrompt(n, data)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if strings.Contains(out, "{{") || strings.Contains(out, "<no value>") {
			t.Fatalf("%s: unrendered template markers", n)
		}
		if !strings.Contains(out, "Cô gái và chiếc túi") {
			t.Fatalf("%s: topic not interpolated", n)
		}
	}
	// breakdown.txt nhận kịch bản pha 1 (không có topic).
	bdData := promptData{Screenplay: `{"title":"T","scenes":[]}`, Orient: "horizontal 16:9", N: 0}
	out, err := directorPrompt("breakdown.txt", bdData)
	if err != nil {
		t.Fatalf("breakdown.txt: %v", err)
	}
	if strings.Contains(out, "{{") || strings.Contains(out, "<no value>") {
		t.Fatal("breakdown.txt: unrendered template markers")
	}
	if !strings.Contains(out, `"title":"T"`) {
		t.Fatal("breakdown.txt: screenplay not interpolated")
	}
	for _, n := range []string{"breakdown.txt"} {
		out, _ := directorPrompt(n, bdData)
		if !strings.Contains(out, "CENTER-SAFE") {
			t.Fatalf("%s: missing center-safe law", n)
		}
		if !strings.Contains(out, "trailer_worthy") {
			t.Fatalf("%s: missing trailer_worthy instruction", n)
		}
	}
}

func TestWriteFilmScriptThreeActs(t *testing.T) {
	// Director 3 pha: 1 call truyện + 3 calls kịch bản theo hồi + 1 call breakdown.
	story := `{"title":"Mưa","logline":"L","text":"Cơn mưa đầu mùa. An ngồi bên cửa sổ..."}`
	scene := `{"index":1,"location":"hanoi cafe","time_of_day":"evening","atmosphere":"warm","seconds":90,` +
		`"action":"An ngồi nhìn mưa, tay siết quai túi",` +
		`"dialogue":[{"character":"An","text":"Anh đến rồi.","emotion":"dịu dàng"}],` +
		`"narration":"Đêm mưa."}`
	breakdown := `{"characters":[{"name":"An","appearance":"Vietnamese woman, 25",` +
		`"wardrobe":"white dress","design_note":"váy trắng = tinh khôi"}],` +
		`"scenes":[{"index":1,"location":"hanoi cafe","time_of_day":"evening",` +
		`"atmosphere":"warm","continuity":"white dress","props":["cup"],` +
		`"shots":[{"shot_size":"WS","camera_move":"static","lens_light":"35mm","seconds":8,` +
		`"action":"An ngồi nhìn mưa","image_prompt":"woman in cafe","video_prompt":"static",` +
		`"trailer_worthy":true}],"image_prompt":"cafe keyframe"},` +
		`{"index":2,"location":"hanoi cafe","time_of_day":"evening",` +
		`"atmosphere":"warm","continuity":"white dress","props":["cup"],` +
		`"shots":[{"shot_size":"WS","camera_move":"static","lens_light":"35mm","seconds":8,` +
		`"action":"An ngồi nhìn mưa","image_prompt":"woman in cafe","video_prompt":"static",` +
		`"trailer_worthy":false}],"image_prompt":"cafe keyframe"},` +
		`{"index":3,"location":"hanoi cafe","time_of_day":"evening",` +
		`"atmosphere":"warm","continuity":"white dress","props":["cup"],` +
		`"shots":[{"shot_size":"WS","camera_move":"static","lens_light":"35mm","seconds":8,` +
		`"action":"An ngồi nhìn mưa","image_prompt":"woman in cafe","video_prompt":"static",` +
		`"trailer_worthy":false}],"image_prompt":"cafe keyframe"}]}`
	llm := &seqLLM{replies: []string{
		story,
		`{"title":"Mưa","logline":"L","scenes":[` + scene + `],"recap":"Hồi 1 xong. An là nữ chính."}`,
		`{"scenes":[` + scene + `],"recap":"Hồi 2 xong."}`,
		`{"scenes":[` + scene + `]}`,
		breakdown,
	}}
	script, err := WriteFilmScript(context.Background(), llm, "Mưa", "Tâm lý", 900, "16:9")
	if err != nil {
		t.Fatal(err)
	}
	if llm.calls != 5 {
		t.Fatalf("3-act film 3 pha = 5 LLM calls, got %d", llm.calls)
	}
	if len(script.Scenes) != 3 {
		t.Fatalf("scenes = %d, want 3", len(script.Scenes))
	}
	for i, sc := range script.Scenes {
		if sc.Act != i+1 {
			t.Fatalf("scene %d act = %d, want %d", i, sc.Act, i+1)
		}
		// Action từ kịch bản pha 1.
		if !strings.Contains(sc.Action, "tay siết quai túi") {
			t.Fatalf("scene %d: action phải từ pha 1, got %q", i, sc.Action)
		}
	}
	if len(script.Characters) != 1 || script.Characters[0].Name != "An" {
		t.Fatalf("bible từ pha 2: %+v", script.Characters)
	}
	if script.Genre != "Tâm lý" {
		t.Fatalf("genre = %q", script.Genre)
	}
	if !script.Scenes[0].Shots[0].TrailerWorthy {
		t.Fatal("trailer_worthy must survive JSON parsing")
	}
}

// ---------------------------------------------------------------------------
// budget stops the job (integration through runFilm)
// ---------------------------------------------------------------------------

func TestRunFilmBudgetStopsJob(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	reply := `{"title":"T","logline":"L","text":"An ngồi trong quán cà phê, chờ một người không đến.",` +
		`
	 "characters":[{"name":"An","appearance":"Vietnamese woman, 25","wardrobe":"white dress"}],` +
		`
	 "scenes":[{"index":1,"location":"cafe","time_of_day":"evening","atmosphere":"warm","seconds":16,` +
		`
	 "shots":[{"shot_size":"WS","camera_move":"static","lens_light":"35mm","seconds":8,"action":"ngồi","image_prompt":"cafe"}],` +
		`
	 "dialogue":[],"image_prompt":"cafe","narration":""}]}`
	mg := &stubMG{} // video succeeds → budget is charged
	s := newWave2Studio(t, &stubLLM{reply: reply}, mg)
	// $0.10 cap vs $0.05/s: the first 8s shot ($0.40) must stop the job.
	// Forced Veo mode — "auto" would resolve to cinematic on a fresh DB
	// (no video capability probe yet) and never touch the Veo budget.
	s.BudgetUSD = func() float64 { return 0.10 }
	id, err := s.CreateFilmJob(FilmParams{Topic: "T", Seconds: 16, Genre: "Tâm lý", RenderMode: RenderModeVeo})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		j, ok := s.GetJob(id)
		if !ok {
			t.Fatal("job gone")
		}
		if j.Status == StatusFailed || j.Status == StatusDone {
			if j.Status != StatusFailed {
				t.Fatalf("job should fail on budget, got %s", j.Status)
			}
			if !strings.Contains(j.Log, "trần chi phí") {
				t.Fatalf("log must name the budget cap:\n%s", j.Log)
			}
			if mg.videoCalls != 0 {
				t.Fatalf("no Veo call may happen past the cap, got %d", mg.videoCalls)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish in 60s")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// trailer shot seqs from cached script
// ---------------------------------------------------------------------------

func TestTrailerShotSeqs(t *testing.T) {
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	id, err := s.insertJob(KindFilm, "t", FilmParams{Topic: "t"})
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(s.WorkDir(), id)
	_ = os.MkdirAll(work, 0o755)
	script := `{"title":"T","characters":[],"scenes":[
	 {"index":1,"seconds":16,"shots":[
	   {"shot_size":"WS","image_prompt":"a","trailer_worthy":true},
	   {"shot_size":"CU","image_prompt":"b"}]},
	 {"index":2,"seconds":8,"shots":[
	   {"shot_size":"WS","image_prompt":"c","trailer_worthy":true}]}]}`
	if err := os.WriteFile(filepath.Join(work, "script.json"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	seqs := s.TrailerShotSeqs(id)
	// scene1: 16s/2 beats → beats of 8s → seqs 0,1 (beat0 worthy → seq 0)
	// scene2: 8s/1 beat → seq 2 (worthy)
	if len(seqs) != 2 || seqs[0] != 0 || seqs[1] != 2 {
		t.Fatalf("trailer seqs = %v, want [0 2]", seqs)
	}
}

// ---------------------------------------------------------------------------
// real ffmpeg: center-crop trailer + subtitle mux
// ---------------------------------------------------------------------------

func TestCropCenterVerticalRender(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	if err := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=1920x1080:duration=2:rate=10",
		"-pix_fmt", "yuv420p", src).Run(); err != nil {
		t.Skipf("cannot make fixture: %v", err)
	}
	dst := filepath.Join(dir, "vert.mp4")
	if err := CropCenterVertical(ctx, src, dst); err != nil {
		t.Fatal(err)
	}
	if w, h := ProbeDims(ctx, dst); w != 1080 || h != 1920 {
		t.Fatalf("cropped trailer = %dx%d, want 1080x1920", w, h)
	}
}

func TestMuxSubtitlesRender(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	if err := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=1920x1080:duration=2:rate=10",
		"-pix_fmt", "yuv420p", src).Run(); err != nil {
		t.Skipf("cannot make fixture: %v", err)
	}
	dst := filepath.Join(dir, "sub.mp4")
	srt := "1\n00:00:00,000 --> 00:00:02,000\nChào em.\n\n"
	if err := MuxSubtitles(ctx, src, srt, dst); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "s",
		"-show_entries", "stream=index", "-of", "csv=p=0", dst)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("subtitle stream missing: %v %q", err, out)
	}
}

// ---------------------------------------------------------------------------
// continuity bible + performance block reach the Veo prompt verbatim
// ---------------------------------------------------------------------------

func TestRenderFilmShotContinuityVerbatim(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	mg := &stubMG{}
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, mg)
	id, err := s.insertJob(KindFilm, "t", FilmParams{Topic: "t", Aspect: "16:9"})
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	sc := FilmScenePro{
		Index: 1, Location: "cafe", TimeOfDay: "evening", Atmosphere: "warm",
		Continuity: "Red dress, hair in bun, brass key prop, candle light from the left.",
	}
	sh := RenderShot{
		Seq: 0, SceneIdx: 1, Seconds: 8, ShotSize: "CU", CameraMove: "static",
		Action: "An nhìn thẳng vào ống kính, tay siết quai túi.", ImagePrompt: "woman portrait",
		Dialogue: []DialogueLine{{Character: "An", Text: "Nhìn này.", Emotion: "căng thẳng"}},
	}
	rs, err := s.renderFilmShot(context.Background(), id, FilmParams{Aspect: "16:9"},
		work, sc, "", nil, sh, "", &veoBudget{rate: 0.05}, RenderModeVeo)
	if err != nil {
		t.Fatal(err)
	}
	if rs.MP4 == "" {
		t.Fatal("no clip rendered")
	}
	p := mg.lastPrompt
	if !strings.Contains(p, "Red dress, hair in bun, brass key prop, candle light from the left.") {
		t.Fatalf("continuity bible must be verbatim in Veo prompt:\n%s", p)
	}
	if !strings.Contains(p, "PERFORMANCE") || !strings.Contains(p, "siết quai túi") {
		t.Fatalf("performance block missing action:\n%s", p)
	}
	if !strings.Contains(p, "An shows căng thẳng") {
		t.Fatalf("performance block missing dialogue emotion:\n%s", p)
	}
}

func TestRenderFilmShotFrameChain(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	mg := &stubMG{}
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, mg)
	id, err := s.insertJob(KindFilm, "t", FilmParams{Topic: "t", Aspect: "16:9"})
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	sc := FilmScenePro{Index: 1, Location: "cafe", TimeOfDay: "evening", Atmosphere: "warm"}
	ctx := context.Background()
	budget := &veoBudget{rate: 0.05}
	mk := func(seq int) RenderShot {
		return RenderShot{Seq: seq, SceneIdx: 1, Seconds: 8, ShotSize: "WS",
			CameraMove: "static", ImagePrompt: "cafe scene"}
	}
	r0, err := s.renderFilmShot(ctx, id, FilmParams{Aspect: "16:9"}, work, sc, "", nil, mk(0), "", budget, RenderModeVeo)
	if err != nil {
		t.Fatal(err)
	}
	if mg.lastFirstFrame == "" || !strings.HasSuffix(mg.lastFirstFrame, "shot0000.png") {
		t.Fatalf("shot 0 must use its own keyframe, got %q", mg.lastFirstFrame)
	}
	r1, err := s.renderFilmShot(ctx, id, FilmParams{Aspect: "16:9"}, work, sc, "", nil, mk(1), r0.MP4, budget, RenderModeVeo)
	if err != nil {
		t.Fatal(err)
	}
	if mg.lastFirstFrame == "" || !strings.HasSuffix(mg.lastFirstFrame, "shot0001.png") {
		t.Fatalf("shot 1 must use shot 0's last frame, got %q", mg.lastFirstFrame)
	}
	if _, err := os.Stat(mg.lastFirstFrame); err != nil {
		t.Fatalf("chained firstFrame must exist on disk: %v", err)
	}
	_ = r1
}
