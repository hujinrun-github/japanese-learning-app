package user

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"japanese-learning-app/internal/httputil"
)

// contextKeyUserID is the key type for storing userID in context.
type contextKeyUserID struct{}

// AuthMiddleware validates the Bearer JWT and injects the userID into the request context.
// Requests with a missing or invalid token receive a 401 response.
func AuthMiddleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "missing authorization header", "")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "invalid authorization header format", "")
			return
		}

		token := parts[1]
		userID, err := VerifyToken(token, secret)
		if err != nil {
			slog.Debug("AuthMiddleware: VerifyToken failed", "err", err)
			httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "invalid or expired token", "")
			return
		}

		ctx := context.WithValue(r.Context(), contextKeyUserID{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext extracts the userID injected by AuthMiddleware.
// This is exported so other packages can use the user module's context key.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	v := ctx.Value(contextKeyUserID{})
	id, ok := v.(int64)
	return id, ok
}
