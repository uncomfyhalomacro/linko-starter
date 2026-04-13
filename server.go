package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"boot.dev/linko/internal/slogger"
	"boot.dev/linko/internal/store"
)

type server struct {
	httpServer *http.Server
	store      store.Store
	cancel     context.CancelFunc
	logger     *slog.Logger
}

func newServer(store store.Store, port int, cancel context.CancelFunc, logger *slog.Logger) *server {
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: slogger.RequestLogger(logger)(mux),
	}

	s := &server{
		httpServer: srv,
		store:      store,
		cancel:     cancel,
		logger:     logger,
	}

	mux.Handle("GET /", setCustomResponseHeaders(http.HandlerFunc(s.handlerIndex)))
	mux.Handle("POST /api/login", setCustomResponseHeaders(s.authMiddleware(http.HandlerFunc(s.handlerLogin))))
	mux.Handle("POST /api/shorten", setCustomResponseHeaders(s.authMiddleware(http.HandlerFunc(s.handlerShortenLink))))
	mux.Handle("GET /api/stats", setCustomResponseHeaders(s.authMiddleware(http.HandlerFunc(s.handlerStats))))
	mux.Handle("GET /api/urls", setCustomResponseHeaders(s.authMiddleware(http.HandlerFunc(s.handlerListURLs))))
	mux.Handle("GET /{shortCode}", setCustomResponseHeaders(http.HandlerFunc(s.handlerRedirect)))
	mux.Handle("POST /admin/shutdown", setCustomResponseHeaders(http.HandlerFunc(s.handlerShutdown)))

	return s
}

func (s *server) start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	if err := s.httpServer.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	addr := ln.Addr().(*net.TCPAddr)
	s.logger.Debug(fmt.Sprintf("Linko is running on http://localhost:%d", addr.Port))
	return nil
}

func (s *server) shutdown(ctx context.Context) error {
	s.logger.Debug("Linko is shutting down")
	return s.httpServer.Shutdown(ctx)
}

func (s *server) handlerShutdown(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("ENV") == "production" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Successfully shutdown"))
	go s.cancel()
}
