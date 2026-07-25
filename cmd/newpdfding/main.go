// Command newpdfding is the single binary that serves the newPdfDing API and
// the embedded SvelteKit frontend (ver refatoracao/01-arquitetura.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edalcin/newpdfding/internal/config"
	"github.com/edalcin/newpdfding/internal/server"
	"github.com/edalcin/newpdfding/internal/store"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "run a health check and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "newpdfding: configuration error: %v\n", err)
		os.Exit(1)
	}

	if *healthcheck {
		runHealthcheck(cfg.DBPath)
		return
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "newpdfding: database error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	log.Printf("schema ready at %s", cfg.DBPath)

	srv := server.New(cfg, db)

	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv.StartSessionCleanup(ctx)

	go func() {
		log.Printf("listening on %s", cfg.ListenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("newpdfding: server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("newpdfding: shutting down…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("newpdfding: shutdown error: %v", err)
	}
	log.Println("newpdfding: stopped")
}

// runHealthcheck opens the DB, runs SELECT 1, and exits 0 on success or 1 on
// failure. Used by the Docker HEALTHCHECK directive (distroless has no shell,
// so the check runs inside the binary itself).
func runHealthcheck(dbPath string) {
	db, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.QueryRow("SELECT 1").Err(); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
