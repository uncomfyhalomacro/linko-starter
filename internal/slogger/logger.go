package slogger

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type multiErrors interface {
	error
	Unwrap() []error
}

type PathError struct {
	Path string
	Err  error
}

func (e PathError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Path
}

func (e PathError) Unwrap() error {
	return e.Err
}

type StructuredLog struct {
	Msg      *string
	Method   *string
	Path     *string
	ClientIp *string
	w        io.Writer
	mu       *sync.Mutex
}

func RequestLogger(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			l.Info("Served request", "method", r.Method, "path", r.URL.Path, "client_ip", r.RemoteAddr)
		})
	}

}

func LogAndUnwrap(l *slog.Logger, level slog.Level, msg string, e error, attrs ...slog.Attr) error {
	l.LogAttrs(context.Background(), level,
		msg,
		attrs...,
	)
	return e
}

func InitializeLogger() (*slog.Logger, *bufio.Writer, *os.File, error) {
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
	debugHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,

		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				val := a.Value.Any().(*slog.Source)
				trace := fmt.Sprintf("%s:%d", val.File, val.Line)
				return slog.GroupAttrs("error", slog.Attr{Key: "fn", Value: slog.StringValue(val.Function)}, slog.Attr{Key: "stack_trace", Value: slog.StringValue(trace)})
			}
			if a.Key == "error" {
				return slog.Any("cause", a.Value)
			}
			return a
		}})

	infoHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,

		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			return a
		}})

	errorHandler := slog.NewJSONHandler(bufferedFile, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelError,

		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				val := a.Value.Any().(*slog.Source)
				trace := fmt.Sprintf("%s:%d", val.File, val.Line)
				return slog.GroupAttrs("error", slog.Attr{Key: "fn", Value: slog.StringValue(val.Function)}, slog.Attr{Key: "stack_trace", Value: slog.StringValue(trace)})
			}
			if a.Key == "error" {
				var errAttrs []slog.Attr
				if errs, ok := a.Value.Any().(multiErrors); ok {

					for i, err := range errs.Unwrap() {
						key := fmt.Sprintf("error_%d", i+1)
						var pathErr PathError
						if errors.As(err, &pathErr) {
							errAttrs = append(errAttrs,
								slog.Group(
									key,
									slog.String("path", pathErr.Path),
								),
							)
						} else {
							errAttrs = append(errAttrs,
								slog.Group(key, slog.String("cause", err.Error())),
							)
						}

					}
					return slog.GroupAttrs("errors", errAttrs...)
				}
			}
			return a
		}})

	return slog.New(slog.NewMultiHandler(debugHandler, infoHandler, errorHandler)), bufferedFile, logFile, nil
}
