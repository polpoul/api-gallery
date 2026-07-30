package config

import (
	"os"
	"strconv"
)

type Config struct {
	PhotosRoot           string
	CacheDir             string
	AuthServiceURL       string
	Port                 string
	CORSOrigins          string
	ThumbnailConcurrency int
}

func Load() Config {
	return Config{
		PhotosRoot:           getenv("PHOTOS_ROOT", "/data/photos"),
		CacheDir:             getenv("CACHE_DIR", "/data/cache"),
		AuthServiceURL:       getenv("AUTH_SERVICE_URL", "https://auth.vivalink.top"),
		Port:                 getenv("PORT", "8080"),
		CORSOrigins:          getenv("CORS_ORIGINS", "https://gallery.vivalink.top"),
		ThumbnailConcurrency: getenvInt("THUMBNAIL_CONCURRENCY", 2),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
