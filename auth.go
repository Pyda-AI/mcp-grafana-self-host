package mcpgrafana

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

const (
	// SecureTokenBytes is the number of bytes for generated tokens (256-bit entropy)
	SecureTokenBytes = 32

	// AuthTokenEnvVar is the environment variable for configuring the Bearer token
	AuthTokenEnvVar = "MCP_AUTH_TOKEN"

	// AuthTokenAuto is the special value that triggers automatic token generation
	AuthTokenAuto = "auto"
)

// AuthConfig holds configuration for Bearer token authentication.
type AuthConfig struct {
	// Token is the Bearer token required for authentication.
	// If empty, authentication is disabled.
	Token string

	// ShowToken controls whether to print the token on startup.
	ShowToken bool

	// PublicPaths are paths that don't require authentication (e.g., /healthz)
	PublicPaths []string
}

// GenerateSecureToken generates a cryptographically secure random token.
// It uses crypto/rand to generate SecureTokenBytes (32) bytes of random data,
// providing 256 bits of entropy. The token is returned as a hex-encoded string.
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, SecureTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// MustGenerateSecureToken generates a secure token and panics on error.
// Use this only during initialization where failure should be fatal.
func MustGenerateSecureToken() string {
	token, err := GenerateSecureToken()
	if err != nil {
		panic(err)
	}
	return token
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

// ResolveAuthToken resolves the authentication token from the provided value.
// If the value is "auto", it generates a new secure token.
// If the value is empty, authentication is disabled.
// Otherwise, the provided value is used as the token.
func ResolveAuthToken(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.ToLower(value) == AuthTokenAuto {
		return GenerateSecureToken()
	}
	return value, nil
}

// PrintTokenInfo logs token information based on configuration.
// If showToken is true, it prints the full token for copying.
// Otherwise, it just confirms that authentication is enabled.
func PrintTokenInfo(token string, showToken bool) {
	if token == "" {
		slog.Info("Bearer token authentication is DISABLED - all requests will be accepted")
		return
	}

	if showToken {
		// Print token in a format that's easy to copy
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                        MCP SERVER AUTHENTICATION TOKEN                       ║")
		fmt.Println("╠══════════════════════════════════════════════════════════════════════════════╣")
		fmt.Printf("║ Token: %-70s ║\n", token)
		fmt.Println("╠══════════════════════════════════════════════════════════════════════════════╣")
		fmt.Println("║ Copy this token and provide it to your MCP client.                          ║")
		fmt.Println("║ Include it in the Authorization header as: Bearer <token>                   ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
		fmt.Println()
	} else {
		slog.Info("Bearer token authentication is ENABLED",
			"token_length", len(token),
			"hint", "use --show-token to display the token")
	}
}

