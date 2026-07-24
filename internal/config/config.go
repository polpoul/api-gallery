package config

import "os"

type Config struct {
	PhotosRoot     string
	CacheDir       string
	AuthServiceURL string
	Port           string
	CORSOrigins    string
}

func Load() Config {
	return Config{
		PhotosRoot:     getenv("PHOTOS_ROOT", "/data/photos"),
		CacheDir:       getenv("CACHE_DIR", "/data/cache"),
		AuthServiceURL: getenv("AUTH_SERVICE_URL", "https://auth.vivalink.top"),
		Port:           getenv("PORT", "8080"),
		CORSOrigins:    getenv("CORS_ORIGINS", "https://gallery.vivalink.top"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
