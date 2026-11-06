// Command snippit runs the Snippit pastebin server.
//
// Subcommands:
//
//	snippit                 run the HTTP server
//	snippit backup <file>   write a consistent database snapshot to <file>
//	snippit healthcheck     exit 0 when the local server is healthy (for Docker)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"snippit/internal/config"
	"snippit/internal/paste"
	"snippit/internal/store"
	"snippit/internal/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log, os.Args[1:]); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		switch args[0] {
		case "healthcheck":
			return healthcheck(cfg)
		case "backup":
			if len(args) != 2 {
				return errors.New("usage: snippit backup <file>")
			}
			db, err := store.Open(cfg.DBPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Backup(context.Background(), args[1]); err != nil {
				return err
			}
			log.Info("backup written", "path", args[1])
			return nil
		default:
			return fmt.Errorf("unknown command %q", args[0])
		}
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.QuickCheck(ctx); err != nil {
		return err
	}

	svc := paste.NewService(db, cfg.MaxPasteBytes, cfg.MaxTotalBytes)
	srv, err := web.New(cfg, svc, db, log)
	if err != nil {
		return err
	}

	go cleanupLoop(ctx, log, svc, cfg.CleanupInterval)
	go srv.SweepLimiters(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

func cleanupLoop(ctx context.Context, log *slog.Logger, svc *paste.Service, every time.Duration) {
	clean := func() {
		n, err := svc.Cleanup(ctx)
		if err != nil && ctx.Err() == nil {
			log.Error("cleanup failed", "err", err)
		} else if n > 0 {
			log.Info("expired pastes removed", "count", n)
		}
	}
	clean()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			clean()
		}
	}
}

func healthcheck(cfg config.Config) error {
	addr := cfg.Addr
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: %s", resp.Status)
	}
	return nil
}
