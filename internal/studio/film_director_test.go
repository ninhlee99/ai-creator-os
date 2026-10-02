//go:build parked

package studio

// film_director_test.go — test director phim (đã park cùng pipeline phim).
// stubLLM dùng chung từ director_test.go (file test không tag luôn được
// biên dịch trong cả hai chế độ).

import (
	"context"
	"strings"
	"testing"
)

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
