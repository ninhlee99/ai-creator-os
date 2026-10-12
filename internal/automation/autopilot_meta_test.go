package automation

import (
	"context"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// fakePub ghi lại mô tả nhận được; ok=false → trả lỗi.
type fakePub struct {
	name  string
	ok    bool
	calls int
	desc  string
}

func (f *fakePub) Name() string          { return f.name }
func (f *fakePub) IsConfigured() bool    { return true }
func (f *fakePub) Handles(k string) bool { return true }
func (f *fakePub) Publish(_ context.Context, _, _, desc, _ string) publishers.PublishResult {
	f.calls++
	f.desc = desc
	if !f.ok {
		return publishers.PublishResult{Platform: f.name, Error: "boom"}
	}
	return publishers.PublishResult{Ok: true, Platform: f.name, RemoteID: f.name + "-1", Draft: f.name == "facebook"}
}

func TestAutoPublishMetaLegs(t *testing.T) {
	fb := &fakePub{name: "facebook", ok: true}
	ig := &fakePub{name: "instagram", ok: false}
	th := &fakePub{name: "threads", ok: true}
	tt := &fakePub{name: "tiktok", ok: true} // không phải chân Meta → bỏ qua
	old := metaPublishersFor
	metaPublishersFor = func(*network.Account) []publishers.Publisher {
		return []publishers.Publisher{tt, fb, ig, th}
	}
	t.Cleanup(func() { metaPublishersFor = old })

	js := &fakeJobStore{}
	s := &Service{JobStore: js}
	j := studio.Job{ID: "j1", Title: "Túi", Output: "/x.mp4", Caption: "Đẹp lắm #tui"}
	p := studio.AffiliateParams{ProductName: "Túi da", AffLink: "https://s.at/abc"}
	s.autoPublishMetaAffiliate(context.Background(), "j1", j, p, &network.Account{Username: "shop"})

	if tt.calls != 0 {
		t.Fatal("tiktok không thuộc chân Meta")
	}
	if fb.calls != 1 || th.calls != 1 || ig.calls != 1 {
		t.Fatalf("calls fb=%d ig=%d th=%d", fb.calls, ig.calls, th.calls)
	}
	if js.PublishLegStatus("j1", "facebook") != studio.LegDone ||
		js.PublishLegStatus("j1", "instagram") != studio.LegFailed ||
		js.PublishLegStatus("j1", "threads") != studio.LegDone {
		t.Fatalf("legs = %v", js.legs)
	}
	for _, want := range []string{"Đẹp lắm #tui", "https://s.at/abc"} {
		if !contains(th.desc, want) {
			t.Errorf("mô tả thiếu %q: %q", want, th.desc)
		}
	}
	// Chạy lại: chân đã xong không đăng lại, chân lỗi thử lại.
	ig.ok = true
	s.autoPublishMetaAffiliate(context.Background(), "j1", j, p, &network.Account{Username: "shop"})
	if fb.calls != 1 || th.calls != 1 || ig.calls != 2 {
		t.Fatalf("idempotency sai: fb=%d ig=%d th=%d", fb.calls, ig.calls, th.calls)
	}
}
