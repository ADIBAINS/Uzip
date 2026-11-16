package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func CORS(origins string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range strings.Split(origins, ",") {
		allowed[strings.TrimSpace(origin)] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-User-ID, X-Tenant-ID, X-Role, X-Email")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		Error(w, http.StatusBadRequest, "invalid_request", err.Error())
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		Error(w, http.StatusBadRequest, "invalid_request", "request must contain exactly one JSON value")
		return false
	}
	return true
}

func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				logger.Error("request panic", "value", value, "stack", string(debug.Stack()))
				Error(w, http.StatusInternalServerError, "internal_error", "unexpected server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func Log(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func Health(mux *http.ServeMux, ready func() error) {
	mux.HandleFunc("GET /live", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := ready(); err != nil {
			Error(w, http.StatusServiceUnavailable, "not_ready", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
}
