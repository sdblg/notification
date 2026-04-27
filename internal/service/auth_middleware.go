package service

import (
	"log/slog"
	"net/http"
	"strings"
)

// APIKeyAuthMiddleware protects handlers using Authorization bearer tokens.
//
// Expected header format:
//
//	Authorization: Bearer <api-key>
func APIKeyAuthMiddleware(expectedAPIKey string, next http.Handler, logger *slog.Logger) http.Handler {
	expectedAPIKey = strings.TrimSpace(expectedAPIKey)
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqLogger := LoggerForRequest(r, logger, "auth_middleware")
		reqLogger = reqLogger.With("path", r.URL.Path, "method", r.Method)

		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if auth == "" {
			reqLogger.Warn("missing authorization header")
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			reqLogger.Warn("invalid authorization scheme")
			http.Error(w, "invalid authorization scheme", http.StatusUnauthorized)
			return
		}

		got := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if got == "" || got != expectedAPIKey {
			reqLogger.Warn("invalid api key")
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
