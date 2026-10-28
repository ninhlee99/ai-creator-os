package studio

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestModelLibraryStoresPortablePaths(t *testing.T) {
	sourceRoot := t.TempDir()
	sourceDB, err := sql.Open("sqlite", filepath.Join(sourceRoot, "studio.db"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewModelStore(sourceDB, filepath.Join(sourceRoot, "output", "studio"))
	if err != nil {
		sourceDB.Close()
		t.Fatal(err)
	}
	photoSource := filepath.Join(t.TempDir(), "reference.png")
	if err := os.WriteFile(photoSource, []byte("character reference"), 0o600); err != nil {
		sourceDB.Close()
		t.Fatal(err)
	}
	photo, err := store.Add(7, photoSource)
	if err != nil {
		sourceDB.Close()
		t.Fatal(err)
	}
	var storedPath string
	if err := sourceDB.QueryRow(`SELECT path FROM model_photos WHERE id=?`, photo.ID).Scan(&storedPath); err != nil {
		sourceDB.Close()
		t.Fatal(err)
	}
	wantStored := filepath.ToSlash(filepath.Join("models", "7", filepath.Base(photo.Path)))
	if storedPath != wantStored {
		sourceDB.Close()
		t.Fatalf("stored model path = %q, want %q", storedPath, wantStored)
	}
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	targetRoot := t.TempDir()
	targetDBPath := filepath.Join(targetRoot, "studio.db")
	dbBytes, err := os.ReadFile(filepath.Join(sourceRoot, "studio.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetDBPath, dbBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	targetPhoto := filepath.Join(targetRoot, "output", "studio", "models", "7", filepath.Base(photo.Path))
	if err := os.MkdirAll(filepath.Dir(targetPhoto), 0o755); err != nil {
		t.Fatal(err)
	}
	photoBytes, err := os.ReadFile(photo.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPhoto, photoBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	targetDB, err := sql.Open("sqlite", targetDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer targetDB.Close()
	targetStore, err := NewModelStore(targetDB, filepath.Join(targetRoot, "output", "studio"))
	if err != nil {
		t.Fatal(err)
	}
	photos, err := targetStore.List(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 1 || photos[0].Path != targetPhoto {
		t.Fatalf("restored model photos = %+v, want path %q", photos, targetPhoto)
	}
}
