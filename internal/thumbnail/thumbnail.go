// Package thumbnail generates resized JPEG previews of photos on first
// request and caches them on disk. The cache key embeds the source file's
// size and modification time, so replacing a file (e.g. via rsync) via a
// new file naturally busts the cache.
package thumbnail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"golang.org/x/sync/singleflight"
)

const (
	maxDimension = 400
	jpegQuality  = 82
)

type Cache struct {
	dir   string
	group singleflight.Group
	sem   chan struct{}
}

// New creates a thumbnail cache. maxConcurrent bounds how many thumbnails can
// be generated (decode + resize + encode) at the same time, regardless of how
// many requests come in at once - opening an album fires dozens of thumbnail
// requests in a burst, and generating all of them in parallel can overload a
// small VPS. Extra requests simply queue and wait their turn.
func New(dir string, maxConcurrent int) *Cache {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Cache{dir: dir, sem: make(chan struct{}, maxConcurrent)}
}

// Get returns the path to a cached thumbnail for the given source photo,
// generating it if necessary. albumID/photoID are used only to namespace
// the cache directory; modTime/size feed the cache-busting key.
func (c *Cache) Get(albumID, photoID, srcPath string, modTime, size int64) (string, error) {
	key := cacheKey(srcPath, modTime, size)
	dst := filepath.Join(c.dir, albumID, photoID+"-"+key+".jpg")

	if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
		return dst, nil
	}

	_, err, _ := c.group.Do(dst, func() (interface{}, error) {
		if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
			return nil, nil
		}
		c.sem <- struct{}{}
		defer func() { <-c.sem }()
		return nil, generate(srcPath, dst)
	})
	if err != nil {
		return "", err
	}
	return dst, nil
}

func generate(srcPath, dstPath string) error {
	img, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("decode %s: %w", srcPath, err)
	}

	thumb := imaging.Fit(img, maxDimension, maxDimension, imaging.Lanczos)

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}

	// Écrit dans un fichier temporaire puis renomme atomiquement: si le
	// processus est interrompu (redémarrage du conteneur, OOM...) pendant
	// l'encodage, le fichier final n'existe jamais dans un état tronqué -
	// soit il est absent (régénéré au prochain appel), soit il est complet.
	tmpFile, err := os.CreateTemp(filepath.Dir(dstPath), ".tmp-*.jpg")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // no-op si le rename a réussi (fichier déjà déplacé)

	if err := imaging.Encode(tmpFile, thumb, imaging.JPEG, imaging.JPEGQuality(jpegQuality)); err != nil {
		tmpFile.Close()
		return fmt.Errorf("encode thumbnail: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Garde-fou: valide que le fichier généré est un JPEG complet et
	// décodable avant de le mettre en cache. Une écriture interrompue peut
	// laisser un fichier non-vide mais tronqué (en-tête présent, données
	// de scan coupées) - une simple vérification de taille ne suffit pas
	// à détecter ce cas, il faut re-décoder pour être sûr.
	if err := verifyDecodable(tmpPath); err != nil {
		return fmt.Errorf("generated thumbnail is invalid for %s: %w", srcPath, err)
	}

	if err := os.Rename(tmpPath, dstPath); err != nil {
		return fmt.Errorf("rename temp file to %s: %w", dstPath, err)
	}
	return nil
}

func cacheKey(srcPath string, modTime, size int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", srcPath, modTime, size)))
	return hex.EncodeToString(h[:])[:16]
}

// verifyDecodable fully decodes the file to make sure it's a complete,
// valid image - catching truncated files (present but cut short mid-write)
// that a size-only check would miss.
func verifyDecodable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, _, err := image.Decode(f); err != nil {
		return fmt.Errorf("decode check failed: %w", err)
	}
	return nil
}
