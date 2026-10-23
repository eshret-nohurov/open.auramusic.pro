// Command bm-open serves open.auramusic.pro.
//
//	bm-open serve        run the site (default)
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
	"syscall"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/config"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/site"
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
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\nusage: bm-open [serve | -version]\n", cmd)
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

	srv, err := site.New(site.Options{Config: cfg, Version: version, Logger: log})
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

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	// pm2 waits kill_timeout (10 s) before SIGKILL; leave room to flush.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err = hs.Shutdown(shutdownCtx)
	srv.Close()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("stopped")
	return nil
}
