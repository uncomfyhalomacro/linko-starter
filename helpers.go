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
	if status == 401 || status == 403 || status == 500 {
		http.Error(w, http.StatusText(status), status)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), status)
	} else {
		http.Error(w, http.StatusText(status), status)
	}
}
