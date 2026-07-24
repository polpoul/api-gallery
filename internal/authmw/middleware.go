// Package authmw validates Bearer device tokens by calling auth-service's
// GET /auth/me. Successful validations are cached in-process for a short TTL
// so a single browsing session (many thumbnail/image requests) doesn't
// hammer auth-service with one HTTP call per request.
package authmw

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const cacheTTL = 5 * time.Minute

type contextKey string

const contextUserID contextKey = "user_id"

type cacheEntry struct {
	userID    string
	expiresAt time.Time
}

type Middleware struct {
	authServiceURL string
	client         *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

func New(authServiceURL string) *Middleware {
	return &Middleware{
		authServiceURL: strings.TrimRight(authServiceURL, "/"),
		client:         &http.Client{Timeout: 5 * time.Second},
		cache:          map[string]cacheEntry{},
	}
}

func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")

		userID, ok := m.validate(token)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), contextUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) validate(token string) (string, bool) {
	m.mu.Lock()
	entry, ok := m.cache[token]
	m.mu.Unlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.userID, true
	}

	userID, ok := m.checkAuthService(token)
	if !ok {
		m.mu.Lock()
		delete(m.cache, token)
		m.mu.Unlock()
		return "", false
	}

	m.mu.Lock()
	m.cache[token] = cacheEntry{userID: userID, expiresAt: time.Now().Add(cacheTTL)}
	m.mu.Unlock()
	return userID, true
}

func (m *Middleware) checkAuthService(token string) (string, bool) {
	req, err := http.NewRequest(http.MethodGet, m.authServiceURL+"/auth/me", nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := m.client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", false
	}
	return body.UserID, true
}

func GetUserID(ctx context.Context) string {
	id, _ := ctx.Value(contextUserID).(string)
	return id
}
