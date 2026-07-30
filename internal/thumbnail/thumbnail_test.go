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

	c := New(cacheDir, 2)

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

// TestGetRegeneratesStaleEmptyCacheFile guards against a real production bug:
// a 0-byte cache file (left over from an interrupted/failed generation before
// the current safeguards existed) must be treated as a cache miss and
// regenerated, not served as-is just because it exists on disk.
func TestGetRegeneratesStaleEmptyCacheFile(t *testing.T) {
	srcDir := t.TempDir()
	cacheDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "photo.jpg")
	writeTestJPEG(t, srcPath, 800, 600)

	info, err := os.Stat(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	c := New(cacheDir, 2)

	// Simulate a stale empty cache file at the exact path Get() would use.
	key := cacheKey(srcPath, info.ModTime().UnixNano(), info.Size())
	stalePath := filepath.Join(cacheDir, "album1", "photo1-"+key+".jpg")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := c.Get("album1", "photo1", srcPath, info.ModTime().UnixNano(), info.Size())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	resultInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("result file missing: %v", err)
	}
	if resultInfo.Size() == 0 {
		t.Fatalf("expected stale empty cache file to be regenerated, got 0 bytes")
	}
}

// TestVerifyDecodableRejectsTruncatedFile guards against the other class of
// interrupted-write corruption: a non-empty but truncated JPEG (valid header,
// scan data cut short) that a size-only check would wrongly accept.
func TestVerifyDecodableRejectsTruncatedFile(t *testing.T) {
	dir := t.TempDir()

	validPath := filepath.Join(dir, "valid.jpg")
	writeTestJPEG(t, validPath, 200, 150)
	if err := verifyDecodable(validPath); err != nil {
		t.Fatalf("expected valid JPEG to pass, got: %v", err)
	}

	fullBytes, err := os.ReadFile(validPath)
	if err != nil {
		t.Fatal(err)
	}
	truncatedPath := filepath.Join(dir, "truncated.jpg")
	if err := os.WriteFile(truncatedPath, fullBytes[:len(fullBytes)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyDecodable(truncatedPath); err == nil {
		t.Fatalf("expected truncated JPEG to fail decode validation")
	}
}
