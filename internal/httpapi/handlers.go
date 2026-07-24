package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/you/api-gallery/internal/index"
	"github.com/you/api-gallery/internal/thumbnail"
)

type Server struct {
	Index      *index.Index
	Thumbnails *thumbnail.Cache
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type albumDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PhotoCount    int    `json:"photoCount"`
	CoverThumbURL string `json:"coverThumbUrl,omitempty"`
}

type photoDTO struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	ThumbURL string `json:"thumbUrl"`
	FullURL  string `json:"fullUrl"`
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) ListAlbums(w http.ResponseWriter, r *http.Request) {
	albums := s.Index.Albums()
	out := make([]albumDTO, 0, len(albums))
	for _, a := range albums {
		dto := albumDTO{ID: a.ID, Name: a.Name, PhotoCount: len(a.Photos)}
		if len(a.Photos) > 0 {
			dto.CoverThumbURL = "/albums/" + a.ID + "/photos/" + a.Photos[0].ID + "/thumb"
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ListPhotos(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "albumId")
	album, err := s.Index.Album(albumID)
	if err != nil {
		http.Error(w, "album not found", http.StatusNotFound)
		return
	}

	out := make([]photoDTO, 0, len(album.Photos))
	for _, p := range album.Photos {
		out = append(out, photoDTO{
			ID:       p.ID,
			Filename: p.Filename,
			ThumbURL: "/albums/" + albumID + "/photos/" + p.ID + "/thumb",
			FullURL:  "/albums/" + albumID + "/photos/" + p.ID,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) GetPhoto(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "albumId")
	photoID := chi.URLParam(r, "photoId")

	photo, err := s.Index.Photo(albumID, photoID)
	if err != nil {
		http.Error(w, "photo not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, photo.Path())
}

func (s *Server) GetThumbnail(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "albumId")
	photoID := chi.URLParam(r, "photoId")

	photo, err := s.Index.Photo(albumID, photoID)
	if err != nil {
		http.Error(w, "photo not found", http.StatusNotFound)
		return
	}

	thumbPath, err := s.Thumbnails.Get(albumID, photoID, photo.Path(), photo.ModTime, photo.Size)
	if err != nil {
		http.Error(w, "failed to generate thumbnail", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, thumbPath)
}

func (s *Server) Refresh(w http.ResponseWriter, r *http.Request) {
	if err := s.Index.Refresh(); err != nil {
		http.Error(w, "refresh failed: "+strings.TrimSpace(err.Error()), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "index refreshed"})
}
