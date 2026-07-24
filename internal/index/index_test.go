package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshAndLookup(t *testing.T) {
	root := t.TempDir()

	mustWrite(t, filepath.Join(root, "Vacances 2023", "plage.jpg"), "jpg-bytes")
	mustWrite(t, filepath.Join(root, "Vacances 2023", "montagne.PNG"), "png-bytes")
	mustWrite(t, filepath.Join(root, "Vacances 2023", "notes.txt"), "ignored")
	mustWrite(t, filepath.Join(root, "Famille", "photo1.jpg"), "jpg-bytes")

	idx := New(root)
	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	albums := idx.Albums()
	if len(albums) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(albums))
	}

	// Sorted alphabetically by folder name: "Famille" before "Vacances 2023".
	if albums[0].Name != "Famille" || albums[1].Name != "Vacances 2023" {
		t.Fatalf("unexpected album order/names: %+v", albums)
	}

	vac, err := idx.Album(albums[1].ID)
	if err != nil {
		t.Fatalf("Album lookup: %v", err)
	}
	if len(vac.Photos) != 2 {
		t.Fatalf("expected 2 photos (txt ignored), got %d: %+v", len(vac.Photos), vac.Photos)
	}

	photo, err := idx.Photo(vac.ID, vac.Photos[0].ID)
	if err != nil {
		t.Fatalf("Photo lookup: %v", err)
	}
	if _, err := os.Stat(photo.Path()); err != nil {
		t.Fatalf("resolved path does not exist: %v", err)
	}

	if _, err := idx.Photo(vac.ID, "does-not-exist"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUniqueIDCollision(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a b", "x.jpg"), "1")
	mustWrite(t, filepath.Join(root, "a-b", "y.jpg"), "2")

	idx := New(root)
	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	albums := idx.Albums()
	if len(albums) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(albums))
	}
	if albums[0].ID == albums[1].ID {
		t.Fatalf("expected distinct IDs on slug collision, got %q twice", albums[0].ID)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
