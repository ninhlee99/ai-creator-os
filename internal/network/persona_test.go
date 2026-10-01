package network

import "testing"

func TestAssignPersonaKeyword(t *testing.T) {
	cases := map[string]string{
		"học tiếng anh giao tiếp":   "teacher",
		"kể chuyện đêm khuya":       "storyteller",
		"live code mini-app python": "coder",
		"chơi game indie chill":     "gamer",
	}
	for hint, want := range cases {
		if got := AssignPersona(hint, nil); got != want {
			t.Errorf("AssignPersona(%q) = %s, want %s", hint, got, want)
		}
	}
}

func TestAssignPersonaNeverAbovePhase2(t *testing.T) {
	// Hints that match phase 3/4 personas must NOT select them.
	for _, hint := range []string{
		"nhảy dance cover", "múa thời trang", // dancer (phase 4)
		"nghe nhạc lofi", "hát karaoke", // musician (phase 3)
	} {
		got := AssignPersona(hint, map[string]int{})
		p, ok := Describe(got)
		if !ok {
			t.Fatalf("Describe(%s) unknown", got)
		}
		if p.Phase > 2 {
			t.Errorf("AssignPersona(%q) = %s (phase %d), want phase <= 2", hint, got, p.Phase)
		}
		if got == "dancer" || got == "musician" {
			t.Errorf("AssignPersona(%q) = %s, must never assign phase>2 persona", hint, got)
		}
	}
}

func TestAssignPersonaDiversity(t *testing.T) {
	// The only unused assignable persona wins.
	used := map[string]int{"storyteller": 1, "teacher": 1, "gamer": 1}
	if got := AssignPersona("", used); got != "coder" {
		t.Errorf("AssignPersona diversity = %s, want coder", got)
	}

	// All used: reuse the least-used one, ties alphabetical.
	usedAll := map[string]int{"storyteller": 2, "teacher": 1, "gamer": 1, "coder": 1}
	if got := AssignPersona("something unrelated", usedAll); got != "coder" {
		t.Errorf("AssignPersona least-used = %s, want coder", got)
	}

	// Keyword match wins, but never duplicates an in-use persona.
	usedT := map[string]int{"teacher": 1}
	if got := AssignPersona("học tiếng anh", usedT); got == "teacher" {
		t.Errorf("AssignPersona duplicated in-use teacher persona")
	}
}

func TestDescribe(t *testing.T) {
	p, ok := Describe("coder")
	if !ok {
		t.Fatal("Describe(coder) not found")
	}
	if p.Phase != 2 {
		t.Errorf("coder phase = %d, want 2", p.Phase)
	}
	if _, ok := Describe("nope"); ok {
		t.Error("Describe(nope) should report unknown")
	}
}
