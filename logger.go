package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type StructuredLog struct {
	Msg       *string
	Method    *string
	Path      *string
	ClientIp *string
	w io.Writer
	mu *sync.Mutex
}

func requestLogger(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			l.Info("Served request", "method", r.Method, "path", r.URL.Path, "client_ip", r.RemoteAddr)
		})
	}

}

func initializeLogger() (*slog.Logger, *bufio.Writer, *os.File, error) {
	logPath := os.Getenv("LINKO_LOG_FILE")
	if logPath == "" {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), nil, nil, nil
	}
	curdir, err := os.Getwd()
	if err != nil {
		slog.Error(fmt.Sprintf("failed to get cwd: %v", err))
		return nil, nil, nil, err
	}
	logPath = filepath.Join(curdir, logPath)
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to open log file: %v", err))
		return nil, nil, nil, err
	}
	bufferedFile := bufio.NewWriterSize(logFile, 8192)
	debugHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: false,
		Level: slog.LevelDebug,
	})
	infoHandler := slog.NewJSONHandler(bufferedFile, &slog.HandlerOptions{AddSource: false, Level: slog.LevelInfo})

	return slog.New(slog.NewMultiHandler(debugHandler, infoHandler)), bufferedFile, logFile, nil
}
