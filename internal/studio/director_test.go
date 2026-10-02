package studio

import (
	"context"
	"strings"
	"testing"
)

type stubLLM struct{ reply string }

func (s *stubLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	return s.reply, nil
}
func (s *stubLLM) Name() string                     { return "stub" }
func (s *stubLLM) Healthy(ctx context.Context) bool { return true }

func TestWriteProShotList(t *testing.T) {
	reply := `{"shots": [
		{"index":1,"purpose":"hook","shot_size":"CU","camera_move":"dolly-in",
		 "lens_light":"35mm f/1.8, golden hour","seconds":5,
		 "action":"Mẫu giơ túi cười với camera",
		 "image_prompt":"Vietnamese model holding cream handbag, close-up",
		 "video_prompt":"model holds handbag, dolly in"},
		{"index":2,"purpose":"build","shot_size":"WS","camera_move":"tracking",
		 "lens_light":"35mm, daylight","seconds":2,
		 "action":"Mẫu bước trên phố",
		 "image_prompt":"model walking street with handbag",
		 "video_prompt":"tracking shot model walking"},
		{"index":3,"purpose":"build","shot_size":"ECU","camera_move":"static",
		 "lens_light":"macro 100mm, softbox","seconds":6,
		 "action":"Macro khóa túi",
		 "image_prompt":"macro of handbag clasp",
		 "video_prompt":"macro push on clasp"},
		{"index":4,"purpose":"payoff","shot_size":"MS","camera_move":"dolly-out",
		 "lens_light":"35mm, warm indoor","seconds":6,
		 "action":"Mẫu ôm túi cười ấm",
		 "image_prompt":"model hugging handbag warm smile",
		 "video_prompt":"model hugs bag, dolly out"}
	]}`
	shots, err := WriteProShotList(context.Background(), &stubLLM{reply: reply},
		"Túi kem", "thời trang", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 4 {
		t.Fatalf("want 4 shots, got %d", len(shots))
	}
	if shots[0].Purpose != "hook" || shots[0].ShotSize != "CU" {
		t.Fatalf("bad shot: %+v", shots[0])
	}
	// seconds=2 is clamped up to 5
	if shots[1].Seconds != 5 {
		t.Fatalf("seconds not clamped: %d", shots[1].Seconds)
	}
}

func TestWriteProShotListFenced(t *testing.T) {
	reply := "```json\n{\"shots\": [{\"index\":1,\"purpose\":\"hook\",\"shot_size\":\"CU\"," +
		"\"camera_move\":\"static\",\"lens_light\":\"50mm\",\"seconds\":5,\"action\":\"a\"," +
		"\"image_prompt\":\"img\",\"video_prompt\":\"vid\"}," +
		"{\"index\":2,\"purpose\":\"build\",\"shot_size\":\"MS\"," +
		"\"camera_move\":\"pan\",\"lens_light\":\"50mm\",\"seconds\":5,\"action\":\"b\"," +
		"\"image_prompt\":\"img2\",\"video_prompt\":\"vid2\"}," +
		"{\"index\":3,\"purpose\":\"build\",\"shot_size\":\"MS\"," +
		"\"camera_move\":\"pan\",\"lens_light\":\"50mm\",\"seconds\":5,\"action\":\"c\"," +
		"\"image_prompt\":\"img3\",\"video_prompt\":\"vid3\"}," +
		"{\"index\":4,\"purpose\":\"payoff\",\"shot_size\":\"WS\"," +
		"\"camera_move\":\"static\",\"lens_light\":\"50mm\",\"seconds\":5,\"action\":\"d\"," +
		"\"image_prompt\":\"img4\",\"video_prompt\":\"vid4\"}]}\n```"
	shots, err := WriteProShotList(context.Background(), &stubLLM{reply: reply},
		"Son", "mỹ phẩm", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 4 {
		t.Fatalf("want 4, got %d", len(shots))
	}
}

func TestWriteFilmScript(t *testing.T) {
	// Director 3 pha: pha 0 viết truyện, pha 1 chuyển thể kịch bản (chưa có
	// bible/shot), pha 2 breakdown trích bible + shot. Truyện là linh hồn,
	// kịch bản pha 1 là chuẩn chuyển thể.
	story := `{"title":"Đêm mưa","logline":"Một cuộc gặp gỡ.",` +
		`"text":"An ngồi bên cửa sổ quán cà phê cũ. Mưa gõ vào kính..."}`
	screenplay := `{"title":"Đêm mưa","logline":"Một cuộc gặp gỡ.",` +
		`"scenes":[{"index":1,"location":"old town cafe, Hanoi","time_of_day":"rainy evening",` +
		`"atmosphere":"warm tungsten light, rain on windows","seconds":18,` +
		`"action":"An ngồi bên cửa sổ, tay siết quai túi","dialogue":` +
		`[{"character":"An","text":"Anh đến rồi à?","emotion":"dịu dàng"}],` +
		`"narration":"Đêm mưa Hà Nội."}]}`
	breakdown := `{"characters":[{"name":"An",` +
		`"appearance":"Vietnamese woman, 25, long black hair, round face",` +
		`"wardrobe":"white dress","design_note":"váy trắng cảnh 1 = sự tinh khôi trước biến cố"}],` +
		`"scenes":[{"index":1,"location":"old town cafe, Hanoi — locked",` +
		`"time_of_day":"rainy evening","atmosphere":"warm tungsten","continuity":"white dress, long black hair",` +
		`"props":["handbag"],` +
		`"shots":[{"shot_size":"WS","camera_move":"slow dolly-in","lens_light":"35mm, warm",` +
		`"seconds":6,"action":"An ngồi bên cửa sổ","image_prompt":"woman in cafe window, rain",` +
		`"video_prompt":"slow push in","trailer_worthy":false}],` +
		`"image_prompt":"woman in cafe window, rain"}]}`
	llm := &seqLLM{replies: []string{story, screenplay, breakdown}}
	script, err := WriteFilmScript(context.Background(), llm, "tình yêu", "Tình cảm", 60, "9:16")
	if err != nil {
		t.Fatal(err)
	}
	if llm.calls != 3 {
		t.Fatalf("phim ngắn 3 pha = 3 calls, got %d", llm.calls)
	}
	if script.Title != "Đêm mưa" || len(script.Characters) != 1 || len(script.Scenes) != 1 {
		t.Fatalf("bad script: %+v", script)
	}
	sc := script.Scenes[0]
	if !strings.Contains(sc.SceneLockBlock(), "old town cafe") {
		t.Fatal("scene lock missing location")
	}
	// Kịch bản pha 1 là chuẩn: action từ pha 1.
	if !strings.Contains(sc.Action, "tay siết quai túi") {
		t.Fatalf("action phải từ kịch bản pha 1, got %q", sc.Action)
	}
	lock := script.Characters[0].LockBlock()
	if !strings.Contains(lock, "long black hair") || !strings.Contains(lock, "EXACT same face") {
		t.Fatal("character lock block incomplete")
	}
	// design_note từ pha 2 (lý giải thiết kế từ nhu cầu kịch bản).
	if !strings.Contains(script.Characters[0].DesignNote, "váy trắng") {
		t.Fatalf("thiếu design_note: %+v", script.Characters[0])
	}
	if len(sc.Dialogue) != 1 || sc.Dialogue[0].Emotion != "dịu dàng" {
		t.Fatalf("bad dialogue: %+v", sc.Dialogue)
	}
	if len(sc.Shots) != 1 || sc.Shots[0].CameraMove != "slow dolly-in" {
		t.Fatalf("shot phải từ breakdown pha 2: %+v", sc.Shots)
	}
	if len(sc.Props) != 1 || sc.Props[0] != "handbag" {
		t.Fatalf("props từ pha 2: %+v", sc.Props)
	}
}

func TestWriteFilmScriptEmpty(t *testing.T) {
	// Pha 0 truyện rỗng → lỗi ngay.
	_, err := WriteFilmScript(context.Background(),
		&stubLLM{reply: `{"title":"x","logline":"y","text":""}`},
		"t", "Tâm lý", 60, "9:16")
	if err == nil {
		t.Fatal("want error for empty story")
	}
}

func TestWriteStory(t *testing.T) {
	reply := `{"title":"Mưa","logline":"L","text":"Cơn mưa đầu mùa đổ xuống phố cũ. An chạy..."}`
	st, err := writeStory(context.Background(), &stubLLM{reply: reply}, "Mưa", "Tâm lý", 60, "16:9")
	if err != nil {
		t.Fatal(err)
	}
	if st.Title != "Mưa" || !strings.Contains(st.Text, "Cơn mưa") {
		t.Fatalf("bad story: %+v", st)
	}
	if _, err := writeStory(context.Background(),
		&stubLLM{reply: `{"title":"x","text":""}`}, "t", "Tâm lý", 60, "16:9"); err == nil {
		t.Fatal("truyện rỗng phải lỗi")
	}
}

func TestStoryWords(t *testing.T) {
	if storyWords(60) != 300 {
		t.Errorf("phim ngắn → tối thiểu 300 từ, got %d", storyWords(60))
	}
	if storyWords(900) != 1800 {
		t.Errorf("900s → 1800 từ, got %d", storyWords(900))
	}
	if storyWords(3600) != 6000 {
		t.Errorf("3600s → kẹp 6000 từ, got %d", storyWords(3600))
	}
}

func TestDirectorPhotoPlanSceneLock(t *testing.T) {
	reply := `{"location": "a cozy minimalist café in Saigon with rattan chairs, warm window light and monstera plants",
"photos": ["close-up of model holding the handbag, sitting by the window",
"full-body shot, model walking past the counter carrying the bag",
"macro of the bag clasp on the marble table"]}`
	s := &Studio{llm: &stubLLM{reply: reply}}
	plan, err := s.directorPhotoPlan(context.Background(),
		AffiliateParams{Seconds: 30, ProductName: "túi xách", Niche: "thời trang"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(plan.Location, "café") {
		t.Fatalf("location not parsed: %q", plan.Location)
	}
	if len(plan.Prompts) != 3 {
		t.Fatalf("want 3 prompts, got %d", len(plan.Prompts))
	}
	for i, pr := range plan.Prompts {
		// Every prompt must embed the locked location verbatim so all
		// photos share one consistent place.
		if !strings.Contains(pr, plan.Location) {
			t.Errorf("prompt %d missing location lock", i)
		}
		if !strings.Contains(pr, "4K") {
			t.Errorf("prompt %d missing 4K quality bar", i)
		}
		if !strings.Contains(strings.ToLower(pr), "no text") {
			t.Errorf("prompt %d missing no-text rule", i)
		}
	}
}

func TestDirectorPhotoPlanMissingLocation(t *testing.T) {
	s := &Studio{llm: &stubLLM{reply: `{"photos": ["a photo"]}`}}
	if _, err := s.directorPhotoPlan(context.Background(),
		AffiliateParams{Seconds: 30}); err == nil {
		t.Fatal("want error when the LLM skips the location lock")
	}
}
