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

	"github.com/jjones/e-biblioteca/internal/auth"
	"github.com/jjones/e-biblioteca/internal/config"
	"github.com/jjones/e-biblioteca/internal/db"
	"github.com/jjones/e-biblioteca/internal/http/handlers"
	"github.com/jjones/e-biblioteca/internal/service/bookdrop"
	"github.com/jjones/e-biblioteca/internal/service/scanner"
	"github.com/jjones/e-biblioteca/internal/store"
)

func main() {
	cfg := config.Load()
	for _, d := range []string{cfg.DataDir, cfg.BooksDir, cfg.BookdropDir, filepath.Join(cfg.DataDir, "covers")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Fatalf("mkdir %s: %v", d, err)
		}
	}

	ctx := context.Background()

	// Wait for DB a bit (docker compose)
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

	st := store.New(pool)
	am := auth.NewManager(pool, cfg.SecureCookies)
	scn := &scanner.Scanner{Store: st, DataDir: cfg.DataDir, BooksDir: cfg.BooksDir}
	bd := bookdrop.New(st, cfg.BookdropDir, cfg.DataDir)

	bg, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bd.Start(bg); err != nil {
		log.Printf("bookdrop watcher: %v", err)
	}

	app := &handlers.App{
		Cfg:      cfg,
		Store:    st,
		Auth:     am,
		Scanner:  scn,
		Bookdrop: bd,
	}

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("e-biblioteca listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("shutting down")
	cancel()
	shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	_ = srv.Shutdown(shutdownCtx)
}
