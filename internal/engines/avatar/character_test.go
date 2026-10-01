package avatar

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestImage(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "face.png")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIdentityLockDeterministic(t *testing.T) {
	img := []byte("fake-png-bytes")
	a := ComputeIdentityLock(img, 42)
	b := ComputeIdentityLock(img, 42)
	if a != b || a == "" {
		t.Error("lock must be deterministic and non-empty")
	}
	if c := ComputeIdentityLock(img, 43); c == a {
		t.Error("different seed must give different lock")
	}
	if c := ComputeIdentityLock([]byte("other"), 42); c == a {
		t.Error("different image must give different lock")
	}
}

func TestCharacterVerifyLock(t *testing.T) {
	img := []byte("fake-png-bytes")
	path := writeTestImage(t, img)
	ch := Character{Name: "MC Test", ReferenceImage: path, Seed: 7}
	ch.IdentityLock = ComputeIdentityLock(img, 7)
	if err := ch.VerifyIdentityLock(); err != nil {
		t.Errorf("valid lock must verify: %v", err)
	}
	// swap the image without re-locking → must refuse
	if err := os.WriteFile(path, []byte("different-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ch.VerifyIdentityLock(); err == nil {
		t.Error("swapped image must fail identity verification")
	}
}

func TestCharacterValidate(t *testing.T) {
	path := writeTestImage(t, []byte("x"))
	ch := Character{Name: "MC", ReferenceImage: path, Seed: 1, IdentityLock: "abc"}
	if err := ch.Validate(); err != nil {
		t.Errorf("valid character: %v", err)
	}
	for name, mutate := range map[string]func(*Character){
		"empty name":   func(c *Character) { c.Name = "" },
		"no image":     func(c *Character) { c.ReferenceImage = "" },
		"bad ext":      func(c *Character) { c.ReferenceImage = "/tmp/x.gif" },
		"missing file": func(c *Character) { c.ReferenceImage = "/tmp/does-not-exist.png" },
		"no lock":      func(c *Character) { c.IdentityLock = "" },
	} {
		bad := ch
		mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}
