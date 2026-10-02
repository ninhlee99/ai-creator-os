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
	reply := `{"title":"Đêm mưa","logline":"Một cuộc gặp gỡ.",
	 "characters":[{"name":"An","appearance":"Vietnamese woman, 25, long black hair, round face",
	 "wardrobe":"white dress"}],
	 "scenes":[{"index":1,"location":"old town cafe, Hanoi","time_of_day":"rainy evening",
	 "atmosphere":"warm tungsten light, rain on windows","seconds":18,
	 "shots":[{"shot_size":"WS","camera_move":"slow dolly-in","lens_light":"35mm, warm",
	 "action":"An ngồi bên cửa sổ"}],
	 "dialogue":[{"character":"An","text":"Anh đến rồi à?","emotion":"dịu dàng"}],
	 "image_prompt":"woman in cafe window, rain",
	 "narration":"Đêm mưa Hà Nội."}]}`
	script, err := WriteFilmScript(context.Background(), &stubLLM{reply: reply}, "tình yêu", 60, "9:16")
	if err != nil {
		t.Fatal(err)
	}
	if script.Title != "Đêm mưa" || len(script.Characters) != 1 || len(script.Scenes) != 1 {
		t.Fatalf("bad script: %+v", script)
	}
	sc := script.Scenes[0]
	if !strings.Contains(sc.SceneLockBlock(), "old town cafe") {
		t.Fatal("scene lock missing location")
	}
	lock := script.Characters[0].LockBlock()
	if !strings.Contains(lock, "long black hair") || !strings.Contains(lock, "EXACT same face") {
		t.Fatal("character lock block incomplete")
	}
	if len(sc.Dialogue) != 1 || sc.Dialogue[0].Emotion != "dịu dàng" {
		t.Fatalf("bad dialogue: %+v", sc.Dialogue)
	}
}

func TestWriteFilmScriptEmpty(t *testing.T) {
	_, err := WriteFilmScript(context.Background(),
		&stubLLM{reply: `{"title":"x","logline":"y","characters":[],"scenes":[]}`},
		"t", 60, "9:16")
	if err == nil {
		t.Fatal("want error for empty script")
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
