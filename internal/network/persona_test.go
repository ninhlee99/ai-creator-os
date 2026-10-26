package network

import "testing"

func TestAssignPersonaAlwaysCreator(t *testing.T) {
	if got := AssignPersona("thời trang", nil); got != "creator" {
		t.Errorf("AssignPersona = %s, want creator", got)
	}
	if got := AssignPersona("", map[string]int{"creator": 5}); got != "creator" {
		t.Errorf("AssignPersona = %s, want creator", got)
	}
}

func TestDescribePersona(t *testing.T) {
	p, ok := Describe("creator")
	if !ok {
		t.Fatal("Describe(creator) should be ok")
	}
	if p.Label != "Nhân vật kênh" {
		t.Errorf("p.Label = %s, want Nhân vật kênh", p.Label)
	}
	// Fallback for any unknown persona is creator
	p2, ok2 := Describe("unknown")
	if !ok2 || p2.Label != "Nhân vật kênh" {
		t.Errorf("fallback for unknown should be creator: %+v, ok=%v", p2, ok2)
	}
}
