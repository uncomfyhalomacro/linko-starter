package main

import (
	"fmt"
	"net/http"

	"boot.dev/linko/internal/slogger"
	"golang.org/x/crypto/bcrypt"
)

var allowedUsers = map[string]string{
	"frodo":   "$2a$10$B6O/n6teuCzpuh66jrUAdeaJ3WvXcxRkzpN0x7H.di9G9e/NGb9Me",
	"samwise": "$2a$10$EWZpvYhUJtJcEMmm/IBOsOGIcpxUnGIVMRiDlN/nxl1RRwWGkJtty",
	// frodo: "ofTheNineFingers"
	// samwise: "theStrong"
	"saruman": "invalidFormat",
}

func (s *server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			httpError(r.Context(), w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
			return
		}

		stored, exists := allowedUsers[username]
		if !exists {
			httpError(r.Context(), w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
			return
		}

		valid, err := s.validatePassword(password, stored)
		if err != nil {
			httpError(r.Context(), w, http.StatusInternalServerError, err)
			return
		}

		if !valid {
			httpError(r.Context(), w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
			return
		}

		logCtx, ok := r.Context().Value(slogger.LogContextKey).(*slogger.LogContext)
		if ok && logCtx != nil {
			(*logCtx).Username = username
		}
		// r = r.WithContext(context.WithValue(r.Context(), slogger.UserContextKey, &username))

		next.ServeHTTP(w, r)
	})
}

func (s *server) validatePassword(password, stored string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password))
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
