package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jjones/e-biblioteca/internal/config"
	"github.com/jjones/e-biblioteca/internal/db"
)

func main() {
	cfg := config.Load()
	for _, d := range []string{cfg.DataDir, cfg.BooksDir, cfg.BookdropDir, filepath.Join(cfg.DataDir, "covers")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Fatalf("mkdir %s: %v", d, err)
		}
	}

	ctx := context.Background()
	var poolErr error
	pool, poolErr := db.Connect(ctx, cfg.DatabaseURL)
	for i := 0; i < 30 && poolErr != nil; i++ {
		log.Printf("waiting for database: %v", poolErr)
		time.Sleep(2 * time.Second)
		pool, poolErr = db.Connect(ctx, cfg.DatabaseURL)
	}
	if poolErr != nil {
		log.Fatalf("database: %v", poolErr)
	}
	defer pool.Close()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html data-theme="dark"><head>
<link rel="stylesheet" href="/static/css/themes.css"/>
<link rel="stylesheet" href="/static/css/base.css"/>
<title>e-biblioteca</title></head>
<body class="app-body auth-main"><div class="auth-card card">
<h1>e-biblioteca</h1>
<p class="muted">Scaffold is live. Auth and catalog land in later phases.</p>
<p><code>/healthz</code> is ready.</p>
</div></body></html>`))
	})

	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("e-biblioteca listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
