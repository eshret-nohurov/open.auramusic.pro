// Command bm-open serves open.auramusic.pro.
//
//	bm-open serve        run the site (default)
//	bm-open mock         run the backend stub for development (127.0.0.1:3399)
//	bm-open stats        print opens and clicks per day (-days 7)
//	bm-open -version     print the build version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/backend"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/config"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/mockbackend"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/preview"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/site"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/stats"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	var err error
	switch cmd {
	case "-version", "--version", "version":
		fmt.Println(version)
	case "serve":
		err = serve(args)
	case "mock":
		err = mock(args)
	case "stats":
		err = report(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\nusage: bm-open [serve | mock | stats | -version]\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bm-open:", err)
		os.Exit(1)
	}
}

func serve(args []string) error {
	fl := flag.NewFlagSet("serve", flag.ExitOnError)
	envFile := fl.String("env", ".env", "settings file; the environment overrides it")
	fl.Parse(args)

	cfg, err := config.Load(*envFile)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	client := backend.New(backend.Options{
		BaseURL:     cfg.BackendBaseURL,
		Token:       cfg.SiteToken,
		Version:     version,
		Timeout:     cfg.BackendTimeout,
		MaxInflight: cfg.BackendMaxInflight,
		Logger:      log,
	})
	previews := preview.New(preview.Options{
		Fetcher:     client,
		FoundTTL:    cfg.PreviewTTL,
		NotFoundTTL: cfg.NotFoundTTL,
		FailTTL:     cfg.FailTTL,
		StaleMax:    cfg.StaleMax,
	})

	rec := stats.New(stats.Options{DataDir: cfg.DataDir, Logger: log, Cache: previews.TakeCounters})

	srv, err := site.New(site.Options{Config: cfg, Version: version, Logger: log, Previews: previews, Stats: rec})
	if err != nil {
		return err
	}

	hs := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("listening", "addr", cfg.ListenAddr, "mode", cfg.Mode, "version", version)
	go probeBackend(client, log)
	runDone := make(chan struct{})
	go func() { rec.Run(ctx); close(runDone) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	// pm2 waits kill_timeout (10 s) before SIGKILL; leave room to flush.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err = hs.Shutdown(shutdownCtx)
	// Run may be in the middle of writing a finished hour; returning before it
	// is done would lose that line.
	<-runDone
	if ferr := rec.Close(); ferr != nil {
		log.Error("write stats", "err", ferr)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("stopped")
	return nil
}

// probeBackend asks about a code that cannot exist. A 404 proves the URL,
// token and IP allowlist are right; anything else is worth a line in the
// log. The site keeps running either way: association files and /healthz
// do not need the backend.
func probeBackend(c *backend.Client, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.Fetch(ctx, "0000000000000000")
	switch {
	case err == nil, errors.Is(err, backend.ErrNotFound):
		log.Info("backend reachable")
	default:
		log.Warn("backend not usable at start, pages will show the fallback screen until it is", "err", err.Error())
	}
}

func mock(args []string) error {
	fl := flag.NewFlagSet("mock", flag.ExitOnError)
	addr := fl.String("addr", config.MockAddr, "listen address")
	dir := fl.String("dir", "testdata/mock", "fixtures, one {code}.json per share code")
	token := fl.String("token", config.MockToken, "expected bearer token")
	fl.Parse(args)

	if _, err := os.Stat(*dir); err != nil {
		return fmt.Errorf("fixtures: %w (run from the repo root or pass -dir)", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("app", "mock")
	log.Info("backend stub listening", "addr", *addr, "fixtures", *dir)
	hs := &http.Server{Addr: *addr, Handler: mockbackend.New(os.DirFS(*dir), *token, log), ReadHeaderTimeout: 5 * time.Second}
	return hs.ListenAndServe()
}

// report needs only DATA_DIR, so it works without a valid .env (no token).
func report(args []string) error {
	fl := flag.NewFlagSet("stats", flag.ExitOnError)
	days := fl.Int("days", 7, "how many days back")
	envFile := fl.String("env", ".env", "settings file with DATA_DIR")
	fl.Parse(args)

	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		if file, err := config.ReadDotEnv(*envFile); err == nil {
			dir = file["DATA_DIR"]
		}
	}
	if dir == "" {
		dir = "./data"
	}
	return stats.Report(os.Stdout, filepath.Join(dir, "stats.jsonl"), *days, time.Now())
}
