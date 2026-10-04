//go:build parked

package content

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/agents/governance"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

func testCfg() governance.Config {
	return governance.Config{DryRun: false, DailyAPIBudgetUSD: 5.0}
}

func testLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.New(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestPickKind(t *testing.T) {
	cases := []struct {
		name string
		acc  *network.Account
		ytc  []string
		want string
	}{
		{"storyteller film allowed", &network.Account{Persona: "storyteller"}, []string{"short_film"}, "short_film"},
		{"storyteller film via tiktok/fb", &network.Account{Persona: "storyteller"}, nil, "short_film"},
		// Quirk faithfully ported from Python: for storyteller the
		// "short_film" entry always returns (TikTok/FB fallback), so the
		// "short_video" menu entry is unreachable — same as the original.
		{"storyteller yt without film", &network.Account{Persona: "storyteller"}, []string{"short_video"}, "short_film"},
		{"teacher", &network.Account{Persona: "teacher"}, nil, "short_video"},
		{"gamer", &network.Account{Persona: "gamer"}, nil, "short_video"},
		{"coder", &network.Account{Persona: "coder"}, nil, "short_video"},
		{"dancer", &network.Account{Persona: "dancer"}, nil, "short_video"},
		{"musician ai_music allowed", &network.Account{Persona: "musician"}, []string{"ai_music"}, "ai_music"},
		{"musician no music kinds", &network.Account{Persona: "musician"}, []string{"short_video"}, "short_video"},
		{"unknown persona", &network.Account{Persona: "astronaut"}, nil, "short_video"},
		{"empty persona", &network.Account{}, nil, "short_video"},
	}
	for _, tc := range cases {
		if got := PickKind(tc.acc, tc.ytc); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPickKindUsesAccountYoutubeTypes(t *testing.T) {
	acc := &network.Account{Persona: "musician", YoutubeContentTypes: []string{"ai_remix"}}
	if got := PickKind(acc, nil); got != "ai_remix" {
		t.Errorf("got %q, want ai_remix", got)
	}
}

// ---- fake publisher ----

type fakePublisher struct {
	name    string
	handles bool
	result  publishers.PublishResult
}

func (f fakePublisher) Name() string          { return f.name }
func (f fakePublisher) IsConfigured() bool    { return true }
func (f fakePublisher) Handles(_ string) bool { return f.handles }
func (f fakePublisher) Publish(_ context.Context, _, _, _, _ string) publishers.PublishResult {
	return f.result
}

func withFakePublishers(t *testing.T, pubs []publishers.Publisher) {
	t.Helper()
	old := newPublishers
	newPublishers = func(_, _ string, _ []string, _ ...string) []publishers.Publisher {
		return pubs
	}
	t.Cleanup(func() { newPublishers = old })
}

func TestDistribute(t *testing.T) {
	l := testLedger(t)
	withFakePublishers(t, []publishers.Publisher{
		fakePublisher{name: "fakepub", handles: true,
			result: publishers.PublishResult{Ok: true, Platform: "fakepub", RemoteID: "r1", Draft: true}},
		fakePublisher{name: "skipme", handles: false},
	})
	acc := &network.Account{Username: "u1", YoutubeChannel: "ch",
		YoutubeContentTypes: []string{"short_video"}}
	res := Distribute(context.Background(), acc, "short_video",
		"/tmp/v.mp4", "title", "desc", l)
	if len(res) != 1 {
		t.Fatalf("results = %v, want 1 entry", res)
	}
	if res[0]["platform"] != "fakepub" || res[0]["ok"] != true ||
		res[0]["remote_id"] != "r1" || res[0]["draft"] != true {
		t.Errorf("result = %v", res[0])
	}
}

func TestDistributeFailureRecorded(t *testing.T) {
	l := testLedger(t)
	withFakePublishers(t, []publishers.Publisher{
		fakePublisher{name: "fakepub", handles: true,
			result: publishers.PublishResult{Ok: false, Error: "boom"}},
	})
	acc := &network.Account{Username: "u1"}
	res := Distribute(context.Background(), acc, "short_video",
		"/tmp/v.mp4", "t", "d", l)
	if len(res) != 1 || res[0]["ok"] != false || res[0]["error"] != "boom" {
		t.Errorf("result = %v", res)
	}
}

// ---- fake TTS + valid WAV ----

type fakeTTS struct{ wav []byte }

func (f fakeTTS) Synthesize(_ context.Context, _, _ string) ([]byte, error) { return f.wav, nil }
func (f fakeTTS) Name() string                                              { return "fake-tts" }
func (f fakeTTS) Healthy(_ context.Context) bool                            { return true }

func testWav(seconds float64) []byte {
	const sr = 8000
	data := make([]byte, int(sr*seconds)*2) // 16-bit mono silence
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, []byte("RIFF"))
	binary.Write(buf, binary.LittleEndian, uint32(36+len(data)))
	binary.Write(buf, binary.LittleEndian, []byte("WAVE"))
	binary.Write(buf, binary.LittleEndian, []byte("fmt "))
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint32(sr))
	binary.Write(buf, binary.LittleEndian, uint32(sr*2))
	binary.Write(buf, binary.LittleEndian, uint16(2))
	binary.Write(buf, binary.LittleEndian, uint16(16))
	binary.Write(buf, binary.LittleEndian, []byte("data"))
	binary.Write(buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)
	return buf.Bytes()
}

func TestMakeShortVideo(t *testing.T) {
	workdir := filepath.Join(t.TempDir(), "work")
	m, err := MakeShortVideo(context.Background(), "máy xay sinh tố",
		fakeTTS{wav: testWav(1.0)}, DefaultSlide, workdir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if m["ok"] != true {
		t.Errorf("ok = %v", m["ok"])
	}
	path, _ := m["path"].(string)
	if path == "" {
		t.Fatal("empty path")
	}
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		t.Errorf("video missing/empty: %v", err)
	}
	if secs, _ := m["seconds"].(float64); secs < 0.9 || secs > 1.1 {
		t.Errorf("seconds = %v, want ~1.0", secs)
	}
}

func TestRunForAccountSkipsMusic(t *testing.T) {
	l := testLedger(t)
	acc := &network.Account{Username: "u1", Persona: "musician",
		YoutubeContentTypes: []string{"ai_music"}}
	res := RunForAccount(context.Background(), testCfg(), l, nil, nil, acc, nil)
	if res["ok"] != true || res["skipped"] != "ai_music" {
		t.Errorf("res = %v", res)
	}
}

func TestRunForAccountBlockedByGovernance(t *testing.T) {
	l := testLedger(t)
	cfg := testCfg()
	cfg.DryRun = true // evaluate_all blocks dry-run
	acc := &network.Account{Username: "u1", Persona: "teacher"}
	res := RunForAccount(context.Background(), cfg, l, nil, nil, acc, nil)
	if res["ok"] != false {
		t.Errorf("expected ok=false, got %v", res)
	}
}
