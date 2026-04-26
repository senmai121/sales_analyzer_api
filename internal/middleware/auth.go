package middleware

import (
	"context"
	"net/http"
	"strings"

	"sales_analyzer_api/internal/auth"
)

// contextKey is a private type for context keys in this package.
type contextKey string

const claimsKey contextKey = "user_claims"

// JWTAuth returns middleware that validates Bearer tokens.
// Valid claims are stored in the request context under the key "user_claims".
func JWTAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				unauthorized(w)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := auth.ValidateToken(tokenStr, secret)
			if err != nil {
				unauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClaims extracts JWT claims from the request context.
// Returns nil if no claims are present (e.g. unauthenticated request).
func GetClaims(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return claims
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}
