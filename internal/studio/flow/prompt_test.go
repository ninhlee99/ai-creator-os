package flow

import "testing"

func TestResolveNodePromptUsesConnectedInputAndCustomTemplate(t *testing.T) {
	inputs := map[string]any{
		"prompt":        "Upstream shot description",
		"product_title": "Silk dress",
	}

	passThrough := &FlowNode{Params: map[string]any{"prompt": ""}}
	if got := resolveNodePrompt(passThrough, inputs, []string{"prompt"}, "fallback"); got != "Upstream shot description" {
		t.Fatalf("blank override = %q, want connected prompt", got)
	}

	custom := &FlowNode{Params: map[string]any{"prompt": "{{input}}. Feature {{product_title}}."}}
	if got := resolveNodePrompt(custom, inputs, []string{"prompt"}, "fallback"); got != "Upstream shot description. Feature Silk dress." {
		t.Fatalf("custom template = %q", got)
	}

	override := &FlowNode{Params: map[string]any{"prompt": "Use a static camera."}}
	if got := resolveNodePrompt(override, inputs, []string{"prompt"}, "fallback"); got != "Use a static camera." {
		t.Fatalf("custom override = %q, want node prompt", got)
	}
}

func TestResolveNodePromptUsesFallbackWithoutInput(t *testing.T) {
	got := resolveNodePrompt(&FlowNode{Params: map[string]any{"prompt": ""}}, nil, []string{"prompt"}, "Default generation prompt")
	if got != "Default generation prompt" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestExpandPromptTemplateResolvesNestedVariables(t *testing.T) {
	got := expandPromptTemplate("{{shot_prompt}}", map[string]any{
		"shot_prompt":  "Show {{product_name}} clearly.",
		"product_name": "Silk dress",
	})
	if got != "Show Silk dress clearly." {
		t.Fatalf("nested prompt = %q", got)
	}
}
