package main

import (
	"context"
	"net/http"

	"boot.dev/linko/internal/slogger"
)

func httpError(ctx context.Context, w http.ResponseWriter, status int, err error) {
	if logCtx, ok := ctx.Value(slogger.LogContextKey).(*slogger.LogContext); ok {
		logCtx.Error = err
	}
	http.Error(w, err.Error(), status)
}
