package avatar

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Character is one AI persona's visual identity. The same character must
// look identical in every frame of every clip — that consistency is
// enforced by the IdentityLock: sha256(reference image bytes + seed).
// A render is refused when the lock does not match the image on disk,
// so a silently swapped reference photo can never drift the face mid-show.
type Character struct {
	ID int64 `json:"id"`
	// Name is the display name ("MC Linh").
	Name string `json:"name"`
	// ReferenceImage is the portrait the renderer is conditioned on.
	// Stored under <data>/avatars/; PNG or JPG.
	ReferenceImage string `json:"reference_image"`
	// Seed locks stochastic generation: same image + same seed = same
	// face, every frame, every clip.
	Seed int64 `json:"seed"`
	// IdentityLock = ComputeIdentityLock(image bytes, seed). Verified
	// before every render.
	IdentityLock string `json:"identity_lock"`
	// VoicePreset links the character to its TTS voice (e.g. "bac-nu-1").
	VoicePreset string `json:"voice_preset"`
	Notes       string `json:"notes"`
	CreatedAt   string `json:"created_at"`
}

// ComputeIdentityLock binds an image to a seed: hex(sha256(image || "|" || seed)).
func ComputeIdentityLock(imageBytes []byte, seed int64) string {
	h := sha256.New()
	h.Write(imageBytes)
	h.Write([]byte("|"))
	h.Write([]byte(strconv.FormatInt(seed, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyIdentityLock recomputes the lock from the image file on disk and
// compares it to the stored lock. A mismatch means the reference photo was
// replaced (or the seed changed) without re-locking — rendering must stop
// rather than silently produce a different face.
func (c Character) VerifyIdentityLock() error {
	if c.ReferenceImage == "" {
		return fmt.Errorf("character %q: no reference image", c.Name)
	}
	img, err := os.ReadFile(c.ReferenceImage)
	if err != nil {
		return fmt.Errorf("character %q: read reference image: %w", c.Name, err)
	}
	if got := ComputeIdentityLock(img, c.Seed); got != c.IdentityLock {
		return fmt.Errorf("character %q: identity lock mismatch — reference image or seed changed; re-lock before rendering", c.Name)
	}
	return nil
}

// Validate checks a character before it is stored or rendered.
func (c Character) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("character name is empty")
	}
	if c.ReferenceImage == "" {
		return fmt.Errorf("character %q: reference image is required", c.Name)
	}
	ext := strings.ToLower(filepath.Ext(c.ReferenceImage))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return fmt.Errorf("character %q: reference image must be PNG or JPG, got %q", c.Name, ext)
	}
	if _, err := os.Stat(c.ReferenceImage); err != nil {
		return fmt.Errorf("character %q: reference image not found: %w", c.Name, err)
	}
	if c.IdentityLock == "" {
		return fmt.Errorf("character %q: identity lock is empty — compute it at creation", c.Name)
	}
	return nil
}

// ShortLock returns the first 12 hex chars of the identity lock for display.
func (c Character) ShortLock() string {
	if len(c.IdentityLock) > 12 {
		return c.IdentityLock[:12]
	}
	return c.IdentityLock
}
