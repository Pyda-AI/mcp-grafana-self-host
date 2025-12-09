package mcpgrafana

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecureTokenCompare(t *testing.T) {
	t.Run("equal tokens return true", func(t *testing.T) {
		assert.True(t, secureTokenCompare("token123", "token123"))
	})

	t.Run("different tokens return false", func(t *testing.T) {
		assert.False(t, secureTokenCompare("token123", "token456"))
	})

	t.Run("different length tokens return false", func(t *testing.T) {
		assert.False(t, secureTokenCompare("short", "verylongtoken"))
	})

	t.Run("empty tokens return true", func(t *testing.T) {
		assert.True(t, secureTokenCompare("", ""))
	})
}

func TestBearerAuthMiddleware(t *testing.T) {
	// Handler that returns 200 OK
	successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	t.Run("allows request with valid token", func(t *testing.T) {
		config := AuthConfig{
			Token:       "test-token",
			PublicPaths: []string{"/healthz"},
		}
		middleware := NewBearerAuthMiddleware(config)
		handler := middleware(successHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "success", rec.Body.String())
	})

	t.Run("rejects request with invalid token", func(t *testing.T) {
		config := AuthConfig{
			Token:       "test-token",
			PublicPaths: []string{"/healthz"},
		}
		middleware := NewBearerAuthMiddleware(config)
		handler := middleware(successHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("rejects request without Authorization header", func(t *testing.T) {
		config := AuthConfig{
			Token:       "test-token",
			PublicPaths: []string{"/healthz"},
		}
		middleware := NewBearerAuthMiddleware(config)
		handler := middleware(successHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("rejects request with non-Bearer auth", func(t *testing.T) {
		config := AuthConfig{
			Token:       "test-token",
			PublicPaths: []string{"/healthz"},
		}
		middleware := NewBearerAuthMiddleware(config)
		handler := middleware(successHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("allows public path without auth", func(t *testing.T) {
		config := AuthConfig{
			Token:       "test-token",
			PublicPaths: []string{"/healthz"},
		}
		middleware := NewBearerAuthMiddleware(config)
		handler := middleware(successHandler)

		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("allows all requests when token is empty", func(t *testing.T) {
		config := AuthConfig{
			Token:       "",
			PublicPaths: []string{"/healthz"},
		}
		middleware := NewBearerAuthMiddleware(config)
		handler := middleware(successHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("public path prefix matching", func(t *testing.T) {
		config := AuthConfig{
			Token:       "test-token",
			PublicPaths: []string{"/public/"},
		}
		middleware := NewBearerAuthMiddleware(config)
		handler := middleware(successHandler)

		req := httptest.NewRequest(http.MethodGet, "/public/resource", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

