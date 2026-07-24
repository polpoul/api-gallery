package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/you/api-gallery/internal/authmw"
	"github.com/you/api-gallery/internal/config"
	"github.com/you/api-gallery/internal/httpapi"
	"github.com/you/api-gallery/internal/index"
	"github.com/you/api-gallery/internal/thumbnail"
)

func corsMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	origins := strings.Split(allowedOrigins, ",")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			for _, allowed := range origins {
				if strings.TrimSpace(allowed) == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					break
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	idx := index.New(cfg.PhotosRoot)
	if err := idx.Refresh(); err != nil {
		log.Fatalf("initial index refresh failed: %v", err)
	}
	log.Printf("indexed %d albums from %s", len(idx.Albums()), cfg.PhotosRoot)

	thumbs := thumbnail.New(cfg.CacheDir)
	auth := authmw.New(cfg.AuthServiceURL)

	srv := &httpapi.Server{Index: idx, Thumbnails: thumbs}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(corsMiddleware(cfg.CORSOrigins))

	r.Get("/health", srv.Health)

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/albums", srv.ListAlbums)
		r.Get("/albums/{albumId}/photos", srv.ListPhotos)
		r.Get("/albums/{albumId}/photos/{photoId}", srv.GetPhoto)
		r.Get("/albums/{albumId}/photos/{photoId}/thumb", srv.GetThumbnail)
		r.Post("/admin/refresh", srv.Refresh)
	})

	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	log.Printf("api-gallery listening on :%s", port)
	log.Printf("CORS origins: %s", cfg.CORSOrigins)
	log.Printf("auth-service: %s", cfg.AuthServiceURL)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
