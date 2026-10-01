package studio

import (
	"context"
	"strings"
	"testing"
)

func TestWriteCaption(t *testing.T) {
	reply := `{"caption": "Túi này xinh xỉu 😍\nDáng quilted sang, đeo đi café auto nổi.\nLink giỏ hàng ở video nha!\n#tiktokshop #tuixach #affiliate #xuhuong #fashion"}`
	cap, err := WriteCaption(context.Background(), &stubLLM{reply: reply}, "Túi quilted kem", "túi xách")
	if err != nil {
		t.Fatalf("WriteCaption: %v", err)
	}
	if !strings.Contains(cap, "#tiktokshop") {
		t.Errorf("caption missing hashtags: %q", cap)
	}
	if len(cap) > 600 {
		t.Errorf("caption too long (%d chars)", len(cap))
	}
}

func TestWriteCaptionEmpty(t *testing.T) {
	if _, err := WriteCaption(context.Background(), nil, "x", "y"); err == nil {
		t.Error("expected error with nil LLM")
	}
	if _, err := WriteCaption(context.Background(), &stubLLM{reply: `{"caption": ""}`}, "x", "y"); err == nil {
		t.Error("expected error with empty caption")
	}
}

func TestCaptionColumnAndOnDone(t *testing.T) {
	s, err := New(t.TempDir()+"/studio.db", &stubLLM{reply: "{}"}, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	id, err := s.insertJob(KindAffiliate, "test", AffiliateParams{Mode: AffiliateModePhoto})
	if err != nil {
		t.Fatalf("insertJob: %v", err)
	}
	fired := make(chan string, 1)
	s.SetOnDone(func(jobID string) { fired <- jobID })
	s.setCaption(id, "cap #test")
	s.fireOnDone(id)
	j, ok := s.GetJob(id)
	if !ok {
		t.Fatal("GetJob not found")
	}
	if j.Caption != "cap #test" {
		t.Errorf("caption not persisted: %q", j.Caption)
	}
	select {
	case got := <-fired:
		if got != id {
			t.Errorf("onDone got %q, want %q", got, id)
		}
	default:
		t.Error("onDone did not fire")
	}
	// Re-open: additive migration must tolerate the existing column.
	s2, err := New(t.TempDir()+"/studio2.db", &stubLLM{reply: "{}"}, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("reopen New: %v", err)
	}
	s2.Close()
}
