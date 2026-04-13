package main

import (
	"crypto/rand"
	"net/http"
)

func setCustomResponseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqId := r.Header.Get("X-Request-ID")
		if reqId == "" {
			w.Header().Set("X-Request-ID", rand.Text())
		} else {
			w.Header().Set("X-Request-ID", reqId)
		}
		next.ServeHTTP(w, r)
	})
}
