package network

import (
	"context"
	"errors"
	"testing"
)

type fakeLLM struct {
	reply string
	err   error
}

func (f *fakeLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	return f.reply, f.err
}
func (f *fakeLLM) Name() string { return "fake-llm" }

func TestResolveTopicWeakHintKeepsHint(t *testing.T) {
	// LLM returns garbage -> fallback keeps the weak hint, source="hint".
	llm := &fakeLLM{reply: "xin lỗi, tôi không hiểu yêu cầu"}
	plan := ResolveTopic(context.Background(), llm, "truyện cười ngắn", "storyteller", nil)
	if plan.Source != SourceHint {
		t.Errorf("source = %s, want hint", plan.Source)
	}
	if plan.Niche != "truyện cười ngắn" {
		t.Errorf("niche = %q, want the hint kept", plan.Niche)
	}
	if len(plan.Topics) != 0 {
		t.Errorf("topics = %v, want empty on fallback", plan.Topics)
	}
}

func TestResolveTopicFencedJSON(t *testing.T) {
	llm := &fakeLLM{reply: "```json\n{\"niche\": \"kể chuyện ma\", \"topics\": [\"ma làng quê\", \"ma đô thị\"]}\n```"}
	plan := ResolveTopic(context.Background(), llm, "truyện ma", "storyteller", nil)
	if plan.Source != SourceHint {
		t.Errorf("source = %s, want hint", plan.Source)
	}
	if plan.Niche != "kể chuyện ma" {
		t.Errorf("niche = %q, want parsed niche", plan.Niche)
	}
	if len(plan.Topics) != 2 || plan.Topics[0] != "ma làng quê" {
		t.Errorf("topics = %v, want parsed topics", plan.Topics)
	}
}

func TestResolveTopicAuto(t *testing.T) {
	llm := &fakeLLM{reply: "{\"niche\": \"tiếng Anh qua phim\", \"topics\": [\"t1\", \"t2\", \"t3\"]}"}
	plan := ResolveTopic(context.Background(), llm, "", "teacher",
		[]string{"tiếng Anh qua truyện ngắn"})
	if plan.Source != SourceAuto {
		t.Errorf("source = %s, want auto", plan.Source)
	}
	if plan.Niche != "tiếng Anh qua phim" {
		t.Errorf("niche = %q", plan.Niche)
	}
}

func TestResolveTopicFallback(t *testing.T) {
	// No LLM at all -> persona default, source="fallback".
	plan := ResolveTopic(context.Background(), nil, "", "teacher", nil)
	if plan.Source != SourceFallback {
		t.Errorf("source = %s, want fallback", plan.Source)
	}
	if plan.Niche != FALLBACK_NICHES["teacher"] {
		t.Errorf("niche = %q, want persona fallback", plan.Niche)
	}
	// LLM error with a hint -> hint kept, source="hint".
	llm := &fakeLLM{err: errors.New("boom")}
	plan = ResolveTopic(context.Background(), llm, "game indie", "gamer", nil)
	if plan.Source != SourceHint || plan.Niche != "game indie" {
		t.Errorf("got %+v, want hint fallback", plan)
	}
}
