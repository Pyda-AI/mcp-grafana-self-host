package mcpgrafana

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

const (
	// AuthTokenEnvVar is the environment variable for configuring the Bearer token
	AuthTokenEnvVar = "MCP_AUTH_TOKEN"
)

// AuthConfig holds configuration for Bearer token authentication.
type AuthConfig struct {
	// Token is the Bearer token required for authentication.
	// If empty, authentication is disabled.
	Token string

	// PublicPaths are paths that don't require authentication (e.g., /healthz)
	PublicPaths []string
}

// NewBearerAuthMiddleware creates HTTP middleware that validates Bearer token authentication.
// It checks the Authorization header for a valid Bearer token using constant-time comparison
// to prevent timing attacks. Requests to public paths (like /healthz) bypass authentication.
func NewBearerAuthMiddleware(config AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this path is public (no auth required)
			for _, publicPath := range config.PublicPaths {
				if r.URL.Path == publicPath || strings.HasPrefix(r.URL.Path, publicPath) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// If no token configured, skip authentication
			if config.Token == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract Bearer token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				slog.Debug("Missing Authorization header", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
				http.Error(w, "Unauthorized: missing Authorization header", http.StatusUnauthorized)
				return
			}

			// Check for Bearer prefix
			const bearerPrefix = "Bearer "
			if !strings.HasPrefix(authHeader, bearerPrefix) {
				slog.Debug("Invalid Authorization header format", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
				http.Error(w, "Unauthorized: invalid Authorization header format", http.StatusUnauthorized)
				return
			}

			// Extract and validate token using constant-time comparison
			providedToken := strings.TrimPrefix(authHeader, bearerPrefix)
			if !secureTokenCompare(config.Token, providedToken) {
				slog.Warn("Invalid Bearer token", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			// Token is valid, proceed with request
			next.ServeHTTP(w, r)
		})
	}
}

// secureTokenCompare performs a constant-time comparison of two tokens
// to prevent timing attacks. Returns true if the tokens are equal.
func secureTokenCompare(expected, provided string) bool {
	// Convert to byte slices for comparison
	expectedBytes := []byte(expected)
	providedBytes := []byte(provided)

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(expectedBytes, providedBytes) == 1
}

// LogAuthStatus logs the authentication status on startup.
func LogAuthStatus(token string) {
	if token == "" {
		slog.Info("Bearer token authentication is DISABLED - all requests will be accepted")
		return
	}
	slog.Info("Bearer token authentication is ENABLED", "token_length", len(token))
}

