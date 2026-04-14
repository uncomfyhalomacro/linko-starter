package slogger

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DeRuina/timberjack"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	slogsyslog "github.com/samber/slog-syslog/v2"
)

type contextKey string

const UserContextKey contextKey = "user"
const LogContextKey contextKey = "log_context"

type LogContext struct {
	Username string
	Error    error
}

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

type spyReadCloser struct {
	io.ReadCloser
	bytesRead int
}

type spyResponseWriter struct {
	http.ResponseWriter
	bytesWritten int
	statusCode   int
}

func (w *spyResponseWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += n
	return n, err
}

func (w *spyResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (r *spyReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.bytesRead += n
	return n, err
}

func RequestLogger(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logCtx := &LogContext{}
			r = r.WithContext(context.WithValue(r.Context(), LogContextKey, logCtx))
			start := time.Now()
			spyWriter := &spyResponseWriter{ResponseWriter: w}
			spyReader := &spyReadCloser{ReadCloser: r.Body}
			r.Body = spyReader
			next.ServeHTTP(spyWriter, r)
			attrs := []slog.Attr{
				slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.String("client_ip", r.RemoteAddr), slog.Duration("duration", time.Since(start)),
				slog.Int("response_status", spyWriter.statusCode),
				slog.Int("response_body_bytes", spyWriter.bytesWritten),
				slog.Int("request_body_bytes", spyReader.bytesRead),
			}
			reqId := r.Header.Get("X-Request-ID")
			if reqId != "" {
				attrs = append(attrs, slog.String("request_id", reqId))
			} else {
				reqId = rand.Text()
				attrs = append(attrs, slog.String("request_id", reqId))
			}

			if logCtx.Username != "" {
				attrs = append(attrs, slog.String("user", logCtx.Username))
			}

			if logCtx.Error != nil {
				attrs = append(attrs, slog.Group("error", slog.String("stack_trace", r.URL.Path), slog.String("message", logCtx.Error.Error())))
			}

			if spyWriter.statusCode >= 200 && spyWriter.statusCode < 300 {
				l.LogAttrs(r.Context(), slog.LevelInfo, "Served request", attrs...)
				return
			}
			if spyWriter.statusCode >= 400 {
				l.LogAttrs(r.Context(), slog.LevelError, "Served request", attrs...)
				return
			}

			l.LogAttrs(r.Context(), slog.LevelDebug, "Served request", attrs...)
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

func InitializeLogger() (*slog.Logger, io.WriteCloser, error) {
	toColor := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()) || isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	logPath := os.Getenv("LINKO_LOG_FILE")
	syslogWriter, err := net.Dial("udp", "localhost:9999")
	if err != nil {
		panic(err)
	}
	syslogOptions := &slogsyslog.Option{
		Level:  slog.LevelInfo,
		Writer: syslogWriter,
	}
	sysloghandler := syslogOptions.NewSyslogHandler()
	if logPath == "" {
		return slog.New(slog.NewMultiHandler(tint.NewHandler(os.Stdout, &tint.Options{
			AddSource:  false,
			Level:      slog.LevelInfo,
			TimeFormat: "",
			NoColor:    !toColor,
		}), sysloghandler)), nil, nil
	}
	curdir, err := os.Getwd()
	if err != nil {
		slog.Error(fmt.Sprintf("failed to get cwd: %v", err))
		return nil, nil, err
	}
	logPath = filepath.Join(curdir, logPath)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to open log file: %v", err))
		return nil, nil, err
	}
	logger := &timberjack.Logger{
		Filename:    logPath,
		MaxSize:     1,
		MaxAge:      28,
		MaxBackups:  10,
		LocalTime:   false,
		Compress:    true,
		Compression: "zstd",
	}
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

	infoHandler := slog.NewJSONHandler(logger, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,

		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			return a
		}})

	errorHandler := slog.NewJSONHandler(logger, &slog.HandlerOptions{
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
						if ae, ok := errors.AsType[*PathError](err); ok {
							errAttrs = append(errAttrs,
								slog.Group(
									key,
									slog.String("path", ae.Path),
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

	return slog.New(slog.NewMultiHandler(debugHandler, infoHandler, errorHandler, sysloghandler)), logger, nil
}
