package studio

import (
	"context"
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
		"Túi kem", "thời trang", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 4 {
		t.Fatalf("want 4 shots, got %d", len(shots))
	}
	if shots[0].Purpose != "hook" || shots[0].ShotSize != "CU" {
		t.Fatalf("bad shot: %+v", shots[0])
	}
	if shots[1].Seconds != 6 {
		t.Fatalf("4-8s shot durations must distribute to exact target: %d", shots[1].Seconds)
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
		"Son", "mỹ phẩm", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 4 {
		t.Fatalf("want 4, got %d", len(shots))
	}
}
