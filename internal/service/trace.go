package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// X-Trace-Id (common spelling). Incoming requests may set it; otherwise one is generated.
const TraceIDHeader = "X-Trace-Id"

type traceIDKeyT struct{}

var traceIDCtxKey = &traceIDKeyT{}

// TraceIDMiddleware ensures each request has a trace ID: from X-Trace-Id or a new UUID.
// It sets the same value on the response and stores it in [context.Context] for logging.
func TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(TraceIDHeader))
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(TraceIDHeader, id)
		ctx := context.WithValue(r.Context(), traceIDCtxKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TraceIDFromContext returns the trace id set by [TraceIDMiddleware], or "" if absent.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(traceIDCtxKey).(string)
	return v
}

// LoggerForRequest returns base with "trace_id" and optional "component" attributes when applicable.
func LoggerForRequest(r *http.Request, base *slog.Logger, component string) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	var log *slog.Logger
	if id := TraceIDFromContext(r.Context()); id != "" {
		log = base.With("trace_id", id)
	} else {
		log = base
	}
	if strings.TrimSpace(component) != "" {
		log = log.With("component", component)
	}
	return log
}
