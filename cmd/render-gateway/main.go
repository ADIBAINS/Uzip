package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/adilbains/uzip/internal/config"
	"github.com/adilbains/uzip/internal/httpx"
)

const demoID = "00000000-0000-0000-0000-000000000001"

func main() {
	cfg := config.Load("10000")
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) == 2 && os.Args[1] == "migrate" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := migrate(ctx, cfg.PostgresURL, os.Getenv("UZIP_MIGRATION_DIR")); err != nil {
			logger.Error("database migration failed", "error", err)
			os.Exit(1)
		}
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/links", proxy("8080", true))
	mux.Handle("/api/v1/links/", proxy("8080", true))
	mux.Handle("/api/v1/api-keys", proxy("8080", true))
	mux.Handle("/api/v1/api-keys/", proxy("8080", true))
	mux.Handle("/api/v1/analytics/", proxy("8083", true))
	mux.Handle("GET /s/{alias}", http.StripPrefix("/s", proxy("8081", false)))
	mux.HandleFunc("GET /ready", ready)
	mux.Handle("/", proxy("3000", false))
	if err := httpx.Serve(cfg.Addr(), mux, cfg.ShutdownTimeout, logger); err != nil {
		logger.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}

func proxy(port string, demoAuth bool) http.Handler {
	target := &url.URL{Scheme: "http", Host: "127.0.0.1:" + port}
	return &httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(target)
			req.SetXForwarded()
			for _, header := range []string{"X-User-ID", "X-Tenant-ID", "X-Role", "X-Email"} {
				req.Out.Header.Del(header)
			}
			if demoAuth {
				req.Out.Header.Set("X-User-ID", demoID)
				req.Out.Header.Set("X-Tenant-ID", demoID)
				req.Out.Header.Set("X-Role", "owner")
			}
			if port == "8081" {
				// Keep the immediate upstream's client IP and ignore earlier entries.
				client := req.In.Header.Get("X-Forwarded-For")
				if last := strings.LastIndexByte(client, ','); last >= 0 {
					client = client[last+1:]
				}
				if client = strings.TrimSpace(client); client != "" {
					req.Out.Header.Set("X-Forwarded-For", client)
				}
			}
		},
	}
}

func ready(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: time.Second}
	for _, service := range []string{"8080/ready", "8081/ready", "8083/ready", "3000/"} {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://127.0.0.1:"+service, nil)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response, err := client.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}
