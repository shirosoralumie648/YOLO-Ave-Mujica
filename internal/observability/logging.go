package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	TraceIDHeader       = "X-Request-Id"
	CorrelationIDHeader = "X-Correlation-Id"
)

type traceIDKey struct{}

type JSONLogger struct {
	mu  sync.Mutex
	out io.Writer
	now func() time.Time
}

func NewJSONLogger(out io.Writer) *JSONLogger {
	if out == nil {
		return nil
	}
	return &JSONLogger{
		out: out,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (l *JSONLogger) Log(fields map[string]any) {
	if l == nil || l.out == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	if _, ok := fields["ts"]; !ok {
		fields["ts"] = l.now().Format(time.RFC3339Nano)
	}

	body, err := json.Marshal(fields)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(body, '\n'))
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(traceIDKey{}).(string)
	return strings.TrimSpace(value)
}

func TraceIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if traceID := TraceIDFromContext(r.Context()); traceID != "" {
		return traceID
	}
	if traceID := strings.TrimSpace(r.Header.Get(TraceIDHeader)); traceID != "" {
		return traceID
	}
	return strings.TrimSpace(r.Header.Get(CorrelationIDHeader))
}

func Middleware(logger *JSONLogger, metrics *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := TraceIDFromRequest(r)
			if traceID == "" {
				traceID = newTraceID()
			}

			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			rec.Header().Set(TraceIDHeader, traceID)
			rec.Header().Set(CorrelationIDHeader, traceID)

			start := time.Now()
			r = r.WithContext(WithTraceID(r.Context(), traceID))
			next.ServeHTTP(rec, r)
			duration := time.Since(start)

			route := routePattern(r)
			if metrics != nil {
				metrics.ObserveHTTPRequest(route, r.Method, rec.status, duration)
			}
			if logger != nil {
				logger.Log(map[string]any{
					"component":   "http_server",
					"event":       "http_request",
					"trace_id":    traceID,
					"method":      r.Method,
					"path":        r.URL.Path,
					"route":       route,
					"status":      rec.status,
					"bytes":       rec.bytes,
					"duration_ms": float64(duration.Microseconds()) / 1000.0,
				})
			}
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	size, err := r.ResponseWriter.Write(body)
	r.bytes += size
	return size, err
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "/"
	}
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		if route := strings.TrimSpace(routeCtx.RoutePattern()); route != "" {
			return route
		}
	}
	if r.URL != nil && strings.TrimSpace(r.URL.Path) != "" {
		return r.URL.Path
	}
	return "/"
}

func newTraceID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000000")))
	}
	return hex.EncodeToString(raw[:])
}
