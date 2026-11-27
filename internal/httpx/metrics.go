package httpx

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Metrics struct {
	requests atomic.Uint64
	active   atomic.Int64
	errors   atomic.Uint64
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.requests.Add(1)
		m.active.Add(1)
		defer m.active.Add(-1)
		next.ServeHTTP(w, r)
	})
}

func (m *Metrics) Error() { m.errors.Add(1) }

func (m *Metrics) AddErrors(count uint64) { m.errors.Add(count) }

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, "# TYPE uzip_http_requests_total counter\nuzip_http_requests_total %d\n", m.requests.Load())
		_, _ = fmt.Fprintf(w, "# TYPE uzip_http_active_requests gauge\nuzip_http_active_requests %d\n", m.active.Load())
		_, _ = fmt.Fprintf(w, "# TYPE uzip_errors_total counter\nuzip_errors_total %d\n", m.errors.Load())
	})
}
