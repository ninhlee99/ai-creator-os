//go:build parked

package studio

// Film Wave 3 — "Điện ảnh từ ảnh": tests cho capability probe, cinematic
// renderer (chuyển động/grain/vignette/letterbox/grade), xfade assembly,
// render mode, và keyframe prompt.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// failImageMG: GenerateImage luôn lỗi — kiểm tra probe fail.
type failImageMG struct{ stubMG }

func (m *failImageMG) GenerateImage(ctx context.Context, prompt string, refs []ImageRef, outPath string) error {
	return fmt.Errorf("quota exceeded")
}

func needFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
}

// testPNG vẽ 1 ảnh test 1280x720 bằng lavfi.
func testPNG(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:duration=1:rate=30",
		"-frames:v", "1", path)
	if err := cmd.Run(); err != nil {
		t.Fatalf("vẽ ảnh test: %v", err)
	}
}

// ---------------------------------------------------------------------------
// motion / grade / transition mapping
// ---------------------------------------------------------------------------

func TestCineMoveKind(t *testing.T) {
	cases := map[string]string{
		"slow dolly-in":  "in",
		"push in slowly": "in",
		"dolly-out":      "out",
		"pull back":      "out",
		"pan left":       "panleft",
		"tracking right": "panright",
		"tilt up":        "tiltup",
		"tilt down":      "tiltdown",
		"static":         "drift",
		"":               "drift",
		"crane shot":     "drift",
	}
	for in, want := range cases {
		if got := cineMoveKind(in); got != want {
			t.Errorf("cineMoveKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCineGrade(t *testing.T) {
	if g := cineGrade("late afternoon golden hour"); !strings.Contains(g, "saturation=1.18") {
		t.Errorf("warm mood grade sai: %s", g)
	}
	if g := cineGrade("cold rainy night"); !strings.Contains(g, "bs=0.12") {
		t.Errorf("cold mood grade sai: %s", g)
	}
	if g := cineGrade("dark noir alley"); !strings.Contains(g, "saturation=0.82") {
		t.Errorf("dark mood grade sai: %s", g)
	}
	if g := cineGrade("bright sunny day"); !strings.Contains(g, "saturation=1.10") {
		t.Errorf("bright mood grade sai: %s", g)
	}
	if g := cineGrade(""); !strings.Contains(g, "contrast=1.05") {
		t.Errorf("default grade sai: %s", g)
	}
}

func TestPickTransition(t *testing.T) {
	if got := pickTransition("pan left", "static"); got != "smoothleft" {
		t.Errorf("pan → smoothleft, got %q", got)
	}
	if got := pickTransition("static", "tracking right"); got != "smoothleft" {
		t.Errorf("tracking → smoothleft, got %q", got)
	}
	if got := pickTransition("slow dolly-in", "static"); got != "fade" {
		t.Errorf("dolly → fade, got %q", got)
	}
}

func TestCineZoompanExprs(t *testing.T) {
	z, x, y := cineZoompan("in", 59)
	if !strings.Contains(z, "1+0.18") || !strings.Contains(x, "iw/zoom") {
		t.Errorf("dolly-in expr sai: z=%s x=%s", z, x)
	}
	z, _, _ = cineZoompan("out", 59)
	if !strings.Contains(z, "1.18-0.18") {
		t.Errorf("dolly-out expr sai: %s", z)
	}
	_, x, _ = cineZoompan("panleft", 59)
	if !strings.Contains(x, "(iw-iw/1.12)") {
		t.Errorf("pan expr sai: %s", x)
	}
	_ = y
}

// ---------------------------------------------------------------------------
// capability probe
// ---------------------------------------------------------------------------

func TestCapabilityRoundtrip(t *testing.T) {
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	// Bản thường chỉ còn hàng image_gen (video_gen đã park khỏi bảng UI);
	// parked code đọc video_gen trực tiếp từ DB.
	caps := s.GetCapabilities()
	if len(caps) != 1 || caps[0].Key != CapImageGen || caps[0].Status != CapUnknown {
		t.Fatalf("mặc định phải 1 hàng image unknown: %+v", caps)
	}
	if err := s.SetCapability(CapImageGen, CapOK, "vẽ thử OK"); err != nil {
		t.Fatal(err)
	}
	if s.VideoCapOK() {
		t.Fatal("video chưa probe → VideoCapOK phải false")
	}
	if err := s.SetCapability(CapVideoGen, CapOK, "quay thử OK"); err != nil {
		t.Fatal(err)
	}
	if !s.VideoCapOK() {
		t.Fatal("video ok → VideoCapOK phải true")
	}
}

func TestRefreshVideoStatusFromHistory(t *testing.T) {
	newJob := func(t *testing.T, s *Studio, methods ...string) string {
		id, err := s.insertJob(KindFilm, "t", FilmParams{Topic: "t"})
		if err != nil {
			t.Fatal(err)
		}
		for i, m := range methods {
			aid := s.addAsset(id, i, "shot", "p")
			s.setAssetMethod(aid, m)
			s.setAsset(aid, StatusDone, "/tmp/x.mp4")
		}
		return id
	}
	// Toàn anh-tts → fail.
	s1 := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	newJob(t, s1, "anh-tts", "anh-tts", "cinematic")
	s1.RefreshVideoStatusFromHistory()
	if got := s1.videoCapStatus(); got != CapFail {
		t.Errorf("toàn fallback → fail, got %q", got)
	}
	// Có 1 veo → ok.
	s2 := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	newJob(t, s2, "anh-tts", "veo")
	s2.RefreshVideoStatusFromHistory()
	if got := s2.videoCapStatus(); got != CapOK {
		t.Errorf("có veo → ok, got %q", got)
	}
	// Chưa quay lần nào → unknown.
	s3 := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	s3.RefreshVideoStatusFromHistory()
	if got := s3.videoCapStatus(); got != CapUnknown {
		t.Errorf("chưa quay → unknown, got %q", got)
	}
	// Probe tay thắng lịch sử: đã fail thì refresh không ghi đè.
	s1.RefreshVideoStatusFromHistory()
	if got := s1.videoCapStatus(); got != CapFail {
		t.Errorf("đã fail thì giữ nguyên, got %q", got)
	}
}

func TestProbeImageGen(t *testing.T) {
	ctx := context.Background()
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	if err := s.ProbeImageGen(ctx); err != nil {
		t.Fatalf("stub image ok → probe phải ok: %v", err)
	}
	if got := s.GetCapabilities()[0].Status; got != CapOK {
		t.Errorf("image probe ok → %q", got)
	}
	s2 := newWave2Studio(t, &stubLLM{reply: "{}"}, &failImageMG{})
	if err := s2.ProbeImageGen(ctx); err == nil {
		t.Fatal("image lỗi → probe phải trả lỗi")
	}
	if got := s2.GetCapabilities()[0].Status; got != CapFail {
		t.Errorf("image probe fail → %q", got)
	}
}

func TestProbeVideoGenStub(t *testing.T) {
	// stubMG quay "thành công" → probe ok (không tốn tiền thật ở test).
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	if err := s.ProbeVideoGen(context.Background()); err != nil {
		t.Fatalf("stub video ok → probe phải ok: %v", err)
	}
	if !s.VideoCapOK() {
		t.Fatal("probe ok → VideoCapOK true")
	}
	s2 := newWave2Studio(t, &stubLLM{reply: "{}"}, &failImageMG{})
	if err := s2.ProbeVideoGen(context.Background()); err == nil {
		t.Fatal("keyframe lỗi → probe phải trả lỗi")
	}
	if s2.VideoCapOK() {
		t.Fatal("probe fail → VideoCapOK false")
	}
}

func TestResolveRenderMode(t *testing.T) {
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, &stubMG{})
	// DB mới → video unknown → auto thành cinematic.
	if got := s.resolveRenderMode(FilmParams{}); got != RenderModeCinematic {
		t.Errorf("auto + unknown → cinematic, got %q", got)
	}
	if got := s.resolveRenderMode(FilmParams{RenderMode: "bogus"}); got != RenderModeCinematic {
		t.Errorf("mode lạ → cinematic, got %q", got)
	}
	if got := s.resolveRenderMode(FilmParams{RenderMode: RenderModeVeo}); got != RenderModeVeo {
		t.Errorf("veo tường minh → veo, got %q", got)
	}
	if got := s.resolveRenderMode(FilmParams{RenderMode: RenderModeCinematic}); got != RenderModeCinematic {
		t.Errorf("cinematic tường minh → cinematic, got %q", got)
	}
	if err := s.SetCapability(CapVideoGen, CapOK, "test"); err != nil {
		t.Fatal(err)
	}
	if got := s.resolveRenderMode(FilmParams{}); got != RenderModeVeo {
		t.Errorf("auto + video ok → veo, got %q", got)
	}
}

func TestKeyframeCompBlock(t *testing.T) {
	b := keyframeCompBlock()
	if b == "" {
		t.Fatal("film_keyframe.txt phải load được")
	}
	if !strings.Contains(b, "HEADROOM") {
		t.Errorf("thiếu hướng dẫn headroom:\n%s", b)
	}
}

// ---------------------------------------------------------------------------
// cinematic render (ffmpeg thật)
// ---------------------------------------------------------------------------

func TestRenderCinematicShot(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	img := filepath.Join(dir, "key.png")
	testPNG(t, img)
	out := filepath.Join(dir, "cine.mp4")
	err := RenderCinematicShot(context.Background(), CinematicShot{
		ImagePath: img, Seconds: 2, CameraMove: "slow dolly-in",
		Mood: "warm golden hour",
	}, out)
	if err != nil {
		t.Fatalf("render cinematic: %v", err)
	}
	w, h := ProbeDims(context.Background(), out)
	if w != 1920 || h != 1080 {
		t.Errorf("khổ = %dx%d, want 1920x1080", w, h)
	}
	d := ProbeDuration(context.Background(), out)
	if d < 1.9 || d > 2.2 {
		t.Errorf("thời lượng = %.2f, want ~2s", d)
	}
	if ProbeHasAudio(context.Background(), out) {
		t.Error("RenderCinematicShot là video-only, không được có audio")
	}
	// fps phải 30 (Ninh chốt 24/30).
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate", "-of", "csv=p=0", out)
	fpsOut, _ := cmd.Output()
	if strings.TrimSpace(string(fpsOut)) != "30/1" {
		t.Errorf("fps = %q, want 30/1", strings.TrimSpace(string(fpsOut)))
	}
}

func TestAddShotAudio(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	img := filepath.Join(dir, "key.png")
	testPNG(t, img)
	v := filepath.Join(dir, "v.mp4")
	if err := RenderCinematicShot(context.Background(),
		CinematicShot{ImagePath: img, Seconds: 2, CameraMove: "static"}, v); err != nil {
		t.Fatal(err)
	}
	// Bản câm → có track audio silence.
	silent := filepath.Join(dir, "silent.mp4")
	if err := AddShotAudio(context.Background(), v, "", 2, silent); err != nil {
		t.Fatalf("gắn silence: %v", err)
	}
	if !ProbeHasAudio(context.Background(), silent) {
		t.Error("bản câm phải có track audio silence")
	}
}

func TestApplyLetterbox(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	img := filepath.Join(dir, "key.png")
	testPNG(t, img)
	v := filepath.Join(dir, "v.mp4")
	if err := RenderCinematicShot(context.Background(),
		CinematicShot{ImagePath: img, Seconds: 1, CameraMove: "static"}, v); err != nil {
		t.Fatal(err)
	}
	lb := filepath.Join(dir, "lb.mp4")
	if err := ApplyLetterbox(context.Background(), v, lb); err != nil {
		t.Fatalf("letterbox: %v", err)
	}
	w, h := ProbeDims(context.Background(), lb)
	if w != 1920 || h != 1080 {
		t.Errorf("letterbox giữ khổ 1920x1080, got %dx%d", w, h)
	}
	// Pixel góc trên-trái phải đen (viền).
	cmd := exec.Command("ffmpeg", "-y", "-v", "error", "-i", lb,
		"-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-")
	px, err := cmd.Output()
	if err != nil {
		t.Fatalf("đọc frame: %v", err)
	}
	if len(px) < 3 || px[0] > 16 || px[1] > 16 || px[2] > 16 {
		t.Errorf("góc trên-trái phải đen (viền letterbox), got %v", px[:3])
	}
}

func TestAssembleCinematic(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	ctx := context.Background()
	var clips []CinematicClip
	for i, mv := range []string{"slow dolly-in", "pan left"} {
		img := filepath.Join(dir, fmt.Sprintf("k%d.png", i))
		testPNG(t, img)
		v := filepath.Join(dir, fmt.Sprintf("v%d.mp4", i))
		if err := RenderCinematicShot(ctx,
			CinematicShot{ImagePath: img, Seconds: 3, CameraMove: mv}, v); err != nil {
			t.Fatal(err)
		}
		a := filepath.Join(dir, fmt.Sprintf("a%d.mp4", i))
		if err := AddShotAudio(ctx, v, "", 3, a); err != nil {
			t.Fatal(err)
		}
		clips = append(clips, CinematicClip{Path: a, CameraMove: mv})
	}
	out := filepath.Join(dir, "film.mp4")
	if err := AssembleCinematic(ctx, clips, out); err != nil {
		t.Fatalf("assemble cinematic: %v", err)
	}
	w, h := ProbeDims(ctx, out)
	if w != 1920 || h != 1080 {
		t.Errorf("khổ = %dx%d, want 1920x1080", w, h)
	}
	// 3+3s − 0.7s xfade ≈ 5.3s.
	d := ProbeDuration(ctx, out)
	if d < 5.0 || d > 5.6 {
		t.Errorf("thời lượng = %.2f, want ~5.3s (xfade 0.7s)", d)
	}
	if !ProbeHasAudio(ctx, out) {
		t.Error("phải có audio sau acrossfade")
	}
}

// ---------------------------------------------------------------------------
// subtitle cues trên timeline xfade
// ---------------------------------------------------------------------------

func TestCinematicCues(t *testing.T) {
	cues := cinematicCues(
		[]string{"Câu một.", "", "Câu hai dài hơn."},
		[]float64{8, 8, 8})
	if len(cues) != 2 {
		t.Fatalf("bỏ dòng trống → 2 cue, got %d", len(cues))
	}
	if cues[0].Start != 0 || cues[0].End < 7 || cues[0].End > 8 {
		t.Errorf("cue 0 sai: %+v", cues[0])
	}
	// shot 1 bắt đầu ở 8−0.7 = 7.3
	if cues[1].Start < 7.2 || cues[1].Start > 7.4 {
		t.Errorf("cue 1 start sai (xfade): %+v", cues[1])
	}
	srt := formatSRT(cues)
	if !strings.Contains(srt, "00:00:00,000 --> ") || !strings.Contains(srt, "Câu hai dài hơn.") {
		t.Errorf("SRT sai:\n%s", srt)
	}
}

// ---------------------------------------------------------------------------
// renderFilmShot chế độ cinematic (đi thẳng, không gọi Veo)
// ---------------------------------------------------------------------------

func TestRenderFilmShotCinematic(t *testing.T) {
	needFFmpeg(t)
	mg := &stubMG{}
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, mg)
	id, err := s.insertJob(KindFilm, "t", FilmParams{Topic: "t", Aspect: "16:9"})
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	sc := FilmScenePro{Index: 1, Location: "cafe", TimeOfDay: "evening",
		Atmosphere: "warm golden hour", Continuity: "red dress"}
	sh := RenderShot{Seq: 0, SceneIdx: 1, Seconds: 4, ShotSize: "WS",
		CameraMove: "slow dolly-in", ImagePrompt: "cafe interior",
		Dialogue: []DialogueLine{{Character: "An", Text: "Chào buổi tối.", Emotion: "dịu dàng"}}}
	rs, err := s.renderFilmShot(context.Background(), id,
		FilmParams{Aspect: "16:9"}, work, sc, "", nil, sh, "",
		&veoBudget{rate: 0.05}, RenderModeCinematic)
	if err != nil {
		t.Fatalf("render cinematic: %v", err)
	}
	if mg.videoCalls != 0 {
		t.Errorf("chế độ cinematic không được gọi Veo, got %d", mg.videoCalls)
	}
	w, h := ProbeDims(context.Background(), rs.MP4)
	if w != 1920 || h != 1080 {
		t.Errorf("khổ = %dx%d", w, h)
	}
	if !ProbeHasAudio(context.Background(), rs.MP4) {
		t.Error("shot phải có audio (voice hoặc silence)")
	}
	assets := s.ListAssets(id)
	found := false
	for _, a := range assets {
		if a.Idx == 0 && a.Kind == "shot" && a.Method == "cinematic" {
			found = true
		}
	}
	if !found {
		t.Errorf("asset phải ghi method=cinematic: %+v", assets)
	}
	// Keyframe prompt phải có khối bố cục (film_keyframe.txt).
	if !strings.Contains(mg.lastImagePrompt, "HEADROOM") {
		t.Errorf("keyframe thiếu khối bố cục: %.120s…", mg.lastImagePrompt)
	}
}

func TestRenderFilmShotAutoNoVeo(t *testing.T) {
	needFFmpeg(t)
	mg := &stubMG{}
	s := newWave2Studio(t, &stubLLM{reply: "{}"}, mg)
	// DB mới → video unknown → auto đi thẳng cinematic, KHÔNG thử Veo.
	id, err := s.insertJob(KindFilm, "t", FilmParams{Topic: "t", Aspect: "16:9"})
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	sc := FilmScenePro{Index: 1, Atmosphere: "warm"}
	sh := RenderShot{Seq: 0, SceneIdx: 1, Seconds: 4, ImagePrompt: "cafe"}
	_, err = s.renderFilmShot(context.Background(), id,
		FilmParams{Aspect: "16:9"}, work, sc, "", nil, sh, "",
		&veoBudget{rate: 0.05}, s.resolveRenderMode(FilmParams{}))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if mg.videoCalls != 0 {
		t.Errorf("auto + video unknown → không gọi Veo, got %d", mg.videoCalls)
	}
}

// ---------------------------------------------------------------------------
// Director 2 pha: "kịch bản trước, breakdown sau"
// ---------------------------------------------------------------------------

func testTwoPhaseScript() (Screenplay, Breakdown) {
	sp := Screenplay{Title: "T", Logline: "L", Genre: "Tâm lý", Scenes: []ScreenplayScene{
		{Index: 1, Location: "cafe", TimeOfDay: "evening", Atmosphere: "warm",
			Seconds: 18, Action: "An ngồi bên cửa sổ",
			Dialogue:  []DialogueLine{{Character: "An", Text: "Chào.", Emotion: "dịu dàng"}},
			Narration: "Đêm mưa."},
		{Index: 2, Location: "street", TimeOfDay: "night", Atmosphere: "cold",
			Seconds: 18, Action: "An bước đi trong mưa",
			Dialogue: []DialogueLine{{Character: "An", Text: "Đi thôi.", Emotion: "quyết tâm"}}},
	}}
	bd := Breakdown{
		Characters: []Character{{Name: "An", Appearance: "Vietnamese woman, 25",
			Wardrobe: "white dress", DesignNote: "váy trắng = tinh khôi"}},
		Scenes: []BreakdownScene{
			{Index: 1, Location: "cafe locked", TimeOfDay: "evening", Atmosphere: "warm",
				Continuity: "white dress", Props: []string{"cup"},
				Shots: []ProShot{{ShotSize: "WS", CameraMove: "static",
					Seconds: 6, Action: "khác", ImagePrompt: "img1"}},
				ImagePrompt: "key1"},
			{Index: 2, Location: "street locked", TimeOfDay: "night", Atmosphere: "cold",
				Continuity: "wet dress", Props: []string{"umbrella"},
				Shots: []ProShot{{ShotSize: "WS", CameraMove: "pan left",
					Seconds: 6, Action: "khác", ImagePrompt: "img2"}},
				ImagePrompt: "key2"},
		},
	}
	return sp, bd
}

func TestAssembleScriptPhaseRule(t *testing.T) {
	sp, bd := testTwoPhaseScript()
	script, err := assembleScript(sp, bd)
	if err != nil {
		t.Fatal(err)
	}
	if len(script.Scenes) != 2 || len(script.Characters) != 1 {
		t.Fatalf("merge sai: %+v", script)
	}
	sc := script.Scenes[0]
	// Kịch bản pha 1 là chuẩn: action/thoại/lời dẫn thắng pha 2.
	if sc.Action != "An ngồi bên cửa sổ" {
		t.Errorf("action phải từ pha 1, got %q", sc.Action)
	}
	if len(sc.Dialogue) != 1 || sc.Dialogue[0].Text != "Chào." {
		t.Errorf("dialogue phải từ pha 1: %+v", sc.Dialogue)
	}
	if sc.Narration != "Đêm mưa." {
		t.Errorf("narration phải từ pha 1, got %q", sc.Narration)
	}
	// Pha 2 cung cấp: bible, khóa, continuity, props, shot.
	if script.Characters[0].DesignNote != "váy trắng = tinh khôi" {
		t.Errorf("design_note từ pha 2: %+v", script.Characters[0])
	}
	if sc.Location != "cafe locked" || sc.Continuity != "white dress" {
		t.Errorf("khóa pha 2: %+v", sc)
	}
	if len(sc.Shots) != 1 || sc.Shots[0].ImagePrompt != "img1" {
		t.Errorf("shot từ pha 2: %+v", sc.Shots)
	}
	if len(sc.Props) != 1 || sc.Props[0] != "cup" {
		t.Errorf("props từ pha 2: %+v", sc.Props)
	}
}

func TestAssembleScriptFailClosed(t *testing.T) {
	sp, bd := testTwoPhaseScript()
	// Thiếu bible → lỗi.
	if _, err := assembleScript(sp, Breakdown{Scenes: bd.Scenes}); err == nil {
		t.Error("thiếu character bible phải lỗi")
	}
	// Thiếu cảnh → lỗi (không quay cảnh không có shot).
	bd2 := bd
	bd2.Scenes = bd2.Scenes[:1]
	if _, err := assembleScript(sp, bd2); err == nil {
		t.Error("thiếu breakdown cảnh 2 phải lỗi")
	}
	// Cảnh không có shot → lỗi.
	bd3 := bd
	bd3.Scenes[0].Shots = []ProShot{{ShotSize: "WS", ImagePrompt: ""}}
	if _, err := assembleScript(sp, bd3); err == nil {
		t.Error("cảnh không có shot phải lỗi")
	}
}

func TestScreenplayCast(t *testing.T) {
	sp, _ := testTwoPhaseScript()
	cast := screenplayCast(sp)
	if !strings.Contains(cast, "- An") {
		t.Errorf("cast trích từ thoại: %q", cast)
	}
	// Không trùng tên.
	if strings.Count(cast, "- An") != 1 {
		t.Errorf("tên không được lặp: %q", cast)
	}
	if got := screenplayCast(Screenplay{}); !strings.Contains(got, "tự đặt tên") {
		t.Errorf("chưa có thoại → gợi ý đặt tên: %q", got)
	}
}

func TestSanitizeShots(t *testing.T) {
	out := sanitizeShots([]ProShot{
		{ShotSize: "WS", ImagePrompt: ""},               // loại
		{ShotSize: "CU", ImagePrompt: "x", Seconds: 0},  // → 4
		{ShotSize: "MS", ImagePrompt: "y", Seconds: 99}, // → 8
	})
	if len(out) != 2 {
		t.Fatalf("loại shot rỗng prompt, got %d", len(out))
	}
	if out[0].Seconds != 4 || out[1].Seconds != 8 {
		t.Errorf("kẹp seconds: %+v", out)
	}
}

// ---------------------------------------------------------------------------
// Director 3 pha: bundle script.json (story/screenplay/breakdown/script)
// ---------------------------------------------------------------------------

func TestWriteFilmScriptBundlePhases(t *testing.T) {
	story := `{"title":"Mưa","logline":"L","text":"Cơn mưa đầu mùa..."}` +
		``
	screenplay := `{"title":"Mưa","logline":"L","scenes":[` +
		`{"index":1,"location":"cafe","time_of_day":"evening","atmosphere":"warm",` +
		`"seconds":18,"action":"An ngồi","dialogue":[],"narration":""}]}`
	breakdown := `{"characters":[{"name":"An","appearance":"woman","wardrobe":"dress",` +
		`"design_note":"note"}],"scenes":[{"index":1,"location":"cafe",` +
		`"time_of_day":"evening","atmosphere":"warm","continuity":"dress",` +
		`"props":[],"shots":[{"shot_size":"WS","camera_move":"static",` +
		`"seconds":6,"action":"An ngồi","image_prompt":"cafe","video_prompt":"static",` +
		`"trailer_worthy":false}],"image_prompt":"cafe"}]}`
	llm := &seqLLM{replies: []string{story, screenplay, breakdown}}
	b, err := WriteFilmScriptBundle(context.Background(), llm, "Mưa", "Tâm lý", 60, "16:9", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.Story.Text, "Cơn mưa") {
		t.Errorf("bundle thiếu story pha 0: %+v", b.Story)
	}
	if len(b.Screenplay.Scenes) != 1 {
		t.Errorf("bundle thiếu screenplay pha 1: %+v", b.Screenplay)
	}
	if len(b.Breakdown.Characters) != 1 {
		t.Errorf("bundle thiếu breakdown pha 2: %+v", b.Breakdown)
	}
	if len(b.Script.Scenes) != 1 || b.Script.Genre != "Tâm lý" {
		t.Errorf("bundle thiếu script ráp cuối: %+v", b.Script)
	}
}

func TestLoadScriptBundleRoundtrip(t *testing.T) {
	work := t.TempDir()
	sp, bd := testTwoPhaseScript()
	b := FilmScriptBundle{
		Story:      Story{Title: "T", Text: "Truyện..."},
		Screenplay: sp,
		Breakdown:  bd,
	}
	script, err := assembleScript(sp, bd)
	if err != nil {
		t.Fatal(err)
	}
	b.Script = script
	raw, _ := json.Marshal(b)
	if err := os.WriteFile(work+"/script.json", raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadScriptBundle(work)
	if err != nil {
		t.Fatal(err)
	}
	if got.Story.Text != "Truyện..." {
		t.Errorf("mất story: %+v", got.Story)
	}
	if len(got.Screenplay.Scenes) != 2 {
		t.Errorf("mất screenplay: %d cảnh", len(got.Screenplay.Scenes))
	}
	if len(got.Breakdown.Characters) != 1 {
		t.Errorf("mất breakdown: %+v", got.Breakdown)
	}
	if len(got.Script.Scenes) != 2 {
		t.Errorf("mất script ráp: %d cảnh", len(got.Script.Scenes))
	}
}

func TestLoadScriptBundleLegacy(t *testing.T) {
	// Job cũ (Wave 1–2): script.json chỉ có FilmScriptPro — vẫn đọc được.
	work := t.TempDir()
	sp, bd := testTwoPhaseScript()
	script, err := assembleScript(sp, bd)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(script)
	if err := os.WriteFile(work+"/script.json", raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadScriptBundle(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Script.Scenes) != 2 {
		t.Fatalf("legacy script.json phải đọc được: %d cảnh", len(got.Script.Scenes))
	}
	if got.Story.Text != "" || len(got.Screenplay.Scenes) != 0 {
		t.Errorf("legacy không có các pha — phải rỗng: %+v", got.Story)
	}
}
