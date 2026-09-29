package middleware

import (
	"context"
	"net/http"
	"strings"

	"mood-tracker/internal/auth"
)

type ctxKey string

const UserIDKey ctxKey = "userID"

func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, "no token", 401)
				return
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			userID, err := auth.ParseToken(tokenStr, secret)
			if err != nil {
				http.Error(w, "invalid token", 401)
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
