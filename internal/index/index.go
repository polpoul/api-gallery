// Package index maintains an in-memory listing of albums (top-level folders
// under the photos root) and photos (image files directly inside an album
// folder). There is no database — the filesystem is the source of truth and
// the index is rebuilt from scratch on Refresh.
package index

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var supportedExt = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

var ErrNotFound = errors.New("not found")

type Photo struct {
	ID       string
	Filename string
	Size     int64
	ModTime  int64 // unix nanoseconds, used for cache-busting thumbnail keys
	path     string
}

type Album struct {
	ID     string
	Name   string
	Photos []Photo
}

// Index is safe for concurrent use: Refresh swaps in a new snapshot under a
// write lock, readers take a read lock.
type Index struct {
	root string

	mu     sync.RWMutex
	albums []Album
	byID   map[string]int // album ID -> index into albums
}

func New(root string) *Index {
	return &Index{root: root, byID: map[string]int{}}
}

// Refresh walks the photos root and rebuilds the album/photo listing.
func (idx *Index) Refresh() error {
	entries, err := os.ReadDir(idx.root)
	if err != nil {
		return err
	}

	seenAlbumIDs := map[string]bool{}
	albums := make([]Album, 0, len(entries))

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		albumPath := filepath.Join(idx.root, e.Name())
		photos, err := readPhotos(albumPath)
		if err != nil {
			continue // skip unreadable folders rather than failing the whole refresh
		}

		album := Album{
			ID:     uniqueID(seenAlbumIDs, slugify(e.Name())),
			Name:   e.Name(),
			Photos: photos,
		}
		seenAlbumIDs[album.ID] = true
		albums = append(albums, album)
	}

	sort.Slice(albums, func(i, j int) bool { return albums[i].Name < albums[j].Name })

	byID := make(map[string]int, len(albums))
	for i, a := range albums {
		byID[a.ID] = i
	}

	idx.mu.Lock()
	idx.albums = albums
	idx.byID = byID
	idx.mu.Unlock()

	return nil
}

func readPhotos(albumPath string) ([]Photo, error) {
	entries, err := os.ReadDir(albumPath)
	if err != nil {
		return nil, err
	}

	seenPhotoIDs := map[string]bool{}
	photos := make([]Photo, 0, len(entries))

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !supportedExt[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		photos = append(photos, Photo{
			ID:       uniqueID(seenPhotoIDs, slugify(e.Name())),
			Filename: e.Name(),
			Size:     info.Size(),
			ModTime:  info.ModTime().UnixNano(),
			path:     filepath.Join(albumPath, e.Name()),
		})
	}

	sort.Slice(photos, func(i, j int) bool { return photos[i].Filename < photos[j].Filename })
	return photos, nil
}

// Albums returns a snapshot of all albums (without exposing filesystem paths).
func (idx *Index) Albums() []Album {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]Album, len(idx.albums))
	copy(out, idx.albums)
	return out
}

// Album returns a single album by ID.
func (idx *Index) Album(albumID string) (Album, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	i, ok := idx.byID[albumID]
	if !ok {
		return Album{}, ErrNotFound
	}
	return idx.albums[i], nil
}

// Photo returns a single photo by album+photo ID, including its resolved
// filesystem path for serving/thumbnailing.
func (idx *Index) Photo(albumID, photoID string) (Photo, error) {
	album, err := idx.Album(albumID)
	if err != nil {
		return Photo{}, err
	}
	for _, p := range album.Photos {
		if p.ID == photoID {
			return p, nil
		}
	}
	return Photo{}, ErrNotFound
}

// Path exposes the resolved filesystem path of a photo (unexported field
// accessor, kept out of JSON responses).
func (p Photo) Path() string { return p.path }

var slugInvalid = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	s := strings.ToLower(base)
	s = slugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "item"
	}
	return s
}

// uniqueID appends a numeric suffix if the slug already exists in seen.
func uniqueID(seen map[string]bool, base string) string {
	if !seen[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !seen[candidate] {
			return candidate
		}
	}
}
