package automation

// Story automation tick (Đợt F): kiểm tra hàng đợi chủ đề → tạo job,
// cổng an toàn (kill/dry-run/tắt), publish fail-closed.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// fakeStoryRunner ghi lại params tạo job.
type fakeStoryRunner struct {
	params   []studio.StoryParams
	jobs     map[string]studio.Job
	appended []string
	nextID   string
}

func (f *fakeStoryRunner) CreateStoryJob(p studio.StoryParams) (string, error) {
	f.params = append(f.params, p)
	id := f.nextID
	if id == "" {
		id = "st-1"
	}
	if f.jobs == nil {
		f.jobs = map[string]studio.Job{}
	}
	f.jobs[id] = studio.Job{ID: id, Kind: studio.KindStory, Title: p.Topic, Status: studio.StatusQueued}
	return id, nil
}

func (f *fakeStoryRunner) GetJob(id string) (studio.Job, bool) {
	j, ok := f.jobs[id]
	return j, ok
}

func (f *fakeStoryRunner) AppendLog(id, line string) {
	f.appended = append(f.appended, id+":"+line)
}

func storyTestService(t *testing.T) (*Service, *fakeStoryRunner) {
	t.Helper()
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{} // không kill, không dry-run
	r := &fakeStoryRunner{}
	svc.Story = r
	return svc, r
}

func TestStoryTickEmptyQueue(t *testing.T) {
	svc, r := storyTestService(t)
	notes := svc.StoryTick(context.Background())
	if len(r.params) != 0 {
		t.Fatalf("params=%d, want 0 (hàng đợi trống)", len(r.params))
	}
	found := false
	for _, n := range notes {
		if contains(n, "hàng đợi chủ đề trống") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notes=%q, want cảnh báo hàng đợi trống", notes)
	}
}

func TestStoryTickPopsOneTopic(t *testing.T) {
	svc, r := storyTestService(t)
	_ = svc.Settings.Set(KeyStoryTopics, "tuổi thơ ở quê\nlần đầu đi chợ Tết")
	_ = svc.Settings.Set(KeyStoryWords, "700")
	notes := svc.StoryTick(context.Background())
	if len(r.params) != 1 {
		t.Fatalf("params=%d, want 1", len(r.params))
	}
	p := r.params[0]
	if p.Topic != "tuổi thơ ở quê" {
		t.Errorf("topic=%q", p.Topic)
	}
	if p.Words != 700 {
		t.Errorf("words=%d, want 700", p.Words)
	}
	// Chủ đề đã lấy bị loại khỏi hàng đợi.
	left, _ := svc.Settings.Get(KeyStoryTopics)
	if left != "lần đầu đi chợ Tết" {
		t.Errorf("queue=%q, want còn lại chủ đề 2", left)
	}
	if len(notes) == 0 {
		t.Error("notes trống, want ghi nhận tạo job")
	}
}

func TestStoryTickOff(t *testing.T) {
	svc, r := storyTestService(t)
	_ = svc.Settings.Set(KeyStoryTopics, "chủ đề x")
	_ = svc.Settings.Set(KeyStoryEnabled, "0")
	if notes := svc.StoryTick(context.Background()); len(notes) != 0 {
		t.Fatalf("notes=%q, want im lặng khi tắt", notes)
	}
	if len(r.params) != 0 {
		t.Fatal("vẫn tạo job khi công tắc tắt")
	}
}

func TestStoryTickNoRunner(t *testing.T) {
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	// Story nil → bỏ qua im lặng, không panic.
	if notes := svc.StoryTick(context.Background()); len(notes) != 0 {
		t.Fatalf("notes=%q, want im lặng", notes)
	}
}

func TestStoryTickDefaultOnWhenUnset(t *testing.T) {
	svc, r := storyTestService(t)
	_ = svc.Settings.Set(KeyStoryTopics, "chủ đề mặc định")
	svc.StoryTick(context.Background())
	if len(r.params) != 1 {
		t.Fatal("công tắc unset phải = BẬT (tiền lệ zero-touch Đợt 3)")
	}
}

// fakeTopicRunner = fakeStoryRunner + tự nghĩ chủ đề (test autofill Đợt K).
type fakeTopicRunner struct {
	fakeStoryRunner
	topics []string
	genErr error
}

func (f *fakeTopicRunner) GenerateTopics(ctx context.Context, genre string, n int) ([]string, error) {
	if f.genErr != nil {
		return nil, f.genErr
	}
	return f.topics, nil
}

func TestStoryTickAutofillsTopics(t *testing.T) {
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	r := &fakeTopicRunner{topics: []string{"chủ đề A", "chủ đề B"}}
	svc.Story = r
	notes := svc.StoryTick(context.Background())
	if len(r.params) != 1 || r.params[0].Topic != "chủ đề A" {
		t.Fatalf("phải tự refill rồi tạo job với chủ đề đầu, params=%+v notes=%v", r.params, notes)
	}
	rest, _ := svc.Settings.Get(KeyStoryTopics)
	if !contains(rest, "chủ đề B") {
		t.Errorf("chủ đề B phải còn trong hàng đợi: %q", rest)
	}
}

func TestStoryTickAutofillHonestOnError(t *testing.T) {
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	r := &fakeTopicRunner{genErr: fmt.Errorf("hết quota")}
	svc.Story = r
	notes := svc.StoryTick(context.Background())
	if len(r.params) != 0 {
		t.Fatalf("refill lỗi không được tạo job")
	}
	found := false
	for _, n := range notes {
		if strings.Contains(n, "tự refill lỗi") {
			found = true
		}
	}
	if !found {
		t.Errorf("phải note trung thực khi refill lỗi: %v", notes)
	}
}

func TestStoryAutoPublishDefaultOn(t *testing.T) {
	svc, _ := storyTestService(t)
	// Đợt K: mặc định BẬT (private-first). Chưa chọn kênh → note rõ.
	notes := svc.storyAutoPublish(context.Background())
	found := false
	for _, n := range notes {
		if contains(n, "auto_publish bật nhưng chưa chọn kênh") {
			found = true
		}
	}
	if !found {
		t.Errorf("mặc định phải bật auto_publish: %v", notes)
	}
}

func TestPublishStoryNowNotReady(t *testing.T) {
	svc, _ := storyTestService(t)
	if _, ok := svc.PublishStoryNow(context.Background(), "nope", "kenh"); ok {
		t.Fatal("ok=true cho job không tồn tại, want fail-closed")
	}
}

func TestPublishStoryNowWaitsForDone(t *testing.T) {
	svc, r := storyTestService(t)
	r.jobs = map[string]studio.Job{
		"st-q": {ID: "st-q", Kind: studio.KindStory, Title: "Chờ", Status: studio.StatusRunning, Output: ""},
	}
	if _, ok := svc.PublishStoryNow(context.Background(), "st-q", "kenh"); ok {
		t.Fatal("ok=true cho job chưa xong, want fail-closed")
	}
}

func TestPublishStoryNowNoAccount(t *testing.T) {
	svc, r := storyTestService(t)
	r.jobs = map[string]studio.Job{
		"st-d": {ID: "st-d", Kind: studio.KindStory, Title: "Xong", Status: studio.StatusDone, Output: "/tmp/x.mp4"},
	}
	if _, ok := svc.PublishStoryNow(context.Background(), "st-d", "kenh-la"); ok {
		t.Fatal("ok=true khi không có kênh/uploader, want fail-closed")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
