package tts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeVieNeuServer records the request body and returns a canned WAV.
type vieNeuCapture struct {
	body map[string]any
	wav  []byte
}

func startFakeVieNeu(t *testing.T, cap *vieNeuCapture, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Write([]byte(`{"status":"ok"}`))
		case "/v1/audio/speech":
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &cap.body)
			w.Header().Set("Content-Type", "audio/wav")
			w.WriteHeader(status)
			w.Write(cap.wav)
		case "/v1/voices":
			w.Write([]byte(`{"object":"list","data":[{"id":"Hải Đăng","name":"Hải Đăng","gender":"female"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
}

// makeRepoPresent creates <dir>/third_party/vieneu-tts/apps/openai_speech.py.
func makeRepoPresent(t *testing.T, dataDir string) {
	t.Helper()
	p := filepath.Join(dataDir, "third_party", "vieneu-tts", "apps")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "openai_speech.py"), []byte("# stub"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVieNeuEmotionCuesPreserved(t *testing.T) {
	dataDir := t.TempDir()
	makeRepoPresent(t, dataDir)
	cap := &vieNeuCapture{wav: pcmToWav([]byte{1, 2, 3, 4}, 24000)}
	srv := startFakeVieNeu(t, cap, 200)
	defer srv.Close()

	v := NewVieNeuProvider(dataDir, nil)
	v.SetBaseURL(srv.URL)

	text := "Xin chào cả nhà [cười] hôm nay vui quá [thở dài] rồi [hắng giọng] nhé"
	got, err := v.Synthesize(context.Background(), text, "default")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(got) != string(cap.wav) {
		t.Fatal("returned bytes differ from server WAV")
	}
	// The request body must carry the emotion cues VERBATIM.
	input, _ := cap.body["input"].(string)
	for _, cue := range []string{"[cười]", "[thở dài]", "[hắng giọng]"} {
		if !strings.Contains(input, cue) {
			t.Fatalf("cue %q stripped from input %q", cue, input)
		}
	}
	if input != text {
		t.Fatalf("input was modified: %q", input)
	}
	if cap.body["model"] != "vieneu-v3-turbo" {
		t.Fatalf("model = %v", cap.body["model"])
	}
	if cap.body["response_format"] != "wav" {
		t.Fatalf("response_format = %v", cap.body["response_format"])
	}
	if cap.body["voice"] != "Hải Đăng" {
		t.Fatalf("voice = %v, want default preset Hải Đăng", cap.body["voice"])
	}
	// We ask the server for 24kHz natively (no ffmpeg round-trip needed).
	if sr, _ := cap.body["sample_rate"].(float64); sr != 24000 {
		t.Fatalf("sample_rate = %v, want 24000", cap.body["sample_rate"])
	}
}

func TestVieNeuMissingRepo(t *testing.T) {
	v := NewVieNeuProvider(t.TempDir(), nil) // empty data dir: no repo
	if v.Name() != "vieneu" {
		t.Fatalf("name = %q", v.Name())
	}
	if v.Healthy(context.Background()) {
		t.Fatal("Healthy should be false without the repo")
	}
	_, err := v.Synthesize(context.Background(), "xin chào", "default")
	if err == nil || !strings.Contains(err.Error(), "VieNeu") {
		t.Fatalf("err = %v, want a clear 'chưa tải VieNeu' error", err)
	}
}

func TestVieNeuSetVoice(t *testing.T) {
	v := NewVieNeuProvider(t.TempDir(), nil)
	if v.Voice() != "Hải Đăng" {
		t.Fatalf("default voice = %q", v.Voice())
	}
	v.SetVoice("Mai Anh")
	if v.Voice() != "Mai Anh" {
		t.Fatalf("voice = %q", v.Voice())
	}
	if got := v.resolveVoice("default"); got != "Mai Anh" {
		t.Fatalf("resolveVoice(default) = %q", got)
	}
	if got := v.resolveVoice(""); got != "Mai Anh" {
		t.Fatalf("resolveVoice('') = %q", got)
	}
	// persona aliases still map
	if got := v.resolveVoice("warm-female"); got != "Mai Anh" {
		t.Fatalf("resolveVoice(warm-female) = %q", got)
	}
	// unknown strings pass through verbatim (real preset names)
	if got := v.resolveVoice("Trúc Ly"); got != "Trúc Ly" {
		t.Fatalf("resolveVoice(Trúc Ly) = %q", got)
	}
}

func TestVieNeuVoices(t *testing.T) {
	dataDir := t.TempDir()
	makeRepoPresent(t, dataDir)
	cap := &vieNeuCapture{}
	srv := startFakeVieNeu(t, cap, 200)
	defer srv.Close()
	v := NewVieNeuProvider(dataDir, nil)
	v.SetBaseURL(srv.URL)
	voices, err := v.Voices(context.Background())
	if err != nil {
		t.Fatalf("Voices: %v", err)
	}
	if len(voices) != 1 || voices[0].ID != "Hải Đăng" {
		t.Fatalf("voices = %+v", voices)
	}
}

func TestVieNeuServerDown(t *testing.T) {
	dataDir := t.TempDir()
	makeRepoPresent(t, dataDir)
	v := NewVieNeuProvider(dataDir, nil)
	v.SetBaseURL("http://127.0.0.1:1") // nothing listening
	if v.Healthy(context.Background()) {
		t.Fatal("Healthy should be false when server is down")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := v.Synthesize(ctx, "xin chào", "default")
	if err == nil || !strings.Contains(err.Error(), "chưa chạy") {
		t.Fatalf("err = %v, want 'sidecar chưa chạy'", err)
	}
}

func TestClassifyVieNeuState(t *testing.T) {
	state, _ := classifyVieNeuState(true, false, false, nil)
	if state != "downloading" {
		t.Fatalf("state = %q", state)
	}
	state, _ = classifyVieNeuState(false, false, false, nil)
	if state != "stopped" {
		t.Fatalf("state = %q", state)
	}
	state, detail := classifyVieNeuState(false, true, true, nil)
	if state != "running" || detail != "healthy" {
		t.Fatalf("state = %q detail = %q", state, detail)
	}
	// running but not healthy, stdout shows a download -> downloading
	state, _ = classifyVieNeuState(false, true, false,
		[]string{"⏳ loading VieNeu-TTS v3 Turbo", "Downloading model: 150M/334M"})
	if state != "downloading" {
		t.Fatalf("state = %q, want downloading", state)
	}
	// running but not healthy, no download keywords -> error
	state, detail = classifyVieNeuState(false, true, false, []string{"Traceback: boom"})
	if state != "error" || !strings.Contains(detail, "boom") {
		t.Fatalf("state = %q detail = %q", state, detail)
	}
}

func TestParseDownloadProgress(t *testing.T) {
	d, tot, ok := parseDownloadProgress("Downloading model: 150M/334M [00:12<00:15]")
	if !ok {
		t.Fatal("not parsed")
	}
	if d != 150*1024*1024 || tot != 334*1024*1024 {
		t.Fatalf("got %d/%d", d, tot)
	}
	if _, _, ok := parseDownloadProgress("just a log line"); ok {
		t.Fatal("false positive")
	}
	if _, _, ok := parseDownloadProgress("500M/100M"); ok {
		t.Fatal("downloaded > total should not parse")
	}
}
