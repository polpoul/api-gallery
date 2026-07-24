package thumbnail

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func writeTestJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGetGeneratesAndCaches(t *testing.T) {
	srcDir := t.TempDir()
	cacheDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "photo.jpg")
	writeTestJPEG(t, srcPath, 800, 600)

	info, err := os.Stat(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	c := New(cacheDir)

	path1, err := c.Get("album1", "photo1", srcPath, info.ModTime().UnixNano(), info.Size())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := os.Stat(path1); err != nil {
		t.Fatalf("thumbnail not written to disk: %v", err)
	}

	path2, err := c.Get("album1", "photo1", srcPath, info.ModTime().UnixNano(), info.Size())
	if err != nil {
		t.Fatalf("Get (cached): %v", err)
	}
	if path1 != path2 {
		t.Fatalf("expected identical cache path on second call, got %q vs %q", path1, path2)
	}

	// Changing size/modTime (simulating a replaced file) must bust the cache key.
	path3, err := c.Get("album1", "photo1", srcPath, info.ModTime().UnixNano()+1, info.Size())
	if err != nil {
		t.Fatalf("Get (busted): %v", err)
	}
	if path3 == path1 {
		t.Fatalf("expected different cache path after modTime change")
	}
}
