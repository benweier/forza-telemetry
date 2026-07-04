package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/benweier/forza-telemetry/server/internal/api"
	"github.com/benweier/forza-telemetry/server/internal/config"
	"github.com/benweier/forza-telemetry/server/internal/ingest"
	"github.com/benweier/forza-telemetry/server/internal/storage"
	"github.com/benweier/forza-telemetry/server/internal/stream"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return usage()
	}

	switch os.Args[1] {
	case "serve":
		return serve(os.Args[2:])
	case "version":
		fmt.Println("forza-telemetry dev")
		return nil
	default:
		return usage()
	}
}

func usage() error {
	fmt.Fprintf(os.Stderr, "usage: %s <command>\n\ncommands:\n  serve    run the telemetry server\n  version  print build version\n", os.Args[0])
	return errors.New("unknown command")
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file (default: $XDG_CONFIG_HOME/forza-telemetry/config.toml)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	broker := stream.NewBroker(cfg.Stream.RingSize)

	store, err := storage.New(cfg.Storage.DataDir, logger)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() {
		if err := store.Close(context.Background()); err != nil {
			logger.Error("close storage", "err", err)
		}
	}()

	writer := store.NewWriter()
	// The writer is a durability consumer: give it a ring-sized buffer so a
	// multi-second stall (stint-close aggregation) can't overflow it and
	// silently punch a hole in the raw capture.
	writerSub := broker.SubscribeBuffered(cfg.Stream.RingSize, false)

	listener, err := ingest.NewListener(cfg.Ingest, broker, logger)
	if err != nil {
		return fmt.Errorf("create UDP listener: %w", err)
	}

	srv := api.New(cfg.API, broker, store, logger)

	errCh := make(chan error, 3)
	go func() { errCh <- listener.Run(ctx) }()
	go func() { errCh <- srv.Run(ctx) }()
	go func() { errCh <- writer.Run(ctx, writerSub) }()

	// Sessions are data-driven (ADR 0012): the writer opens one when the first
	// packet arrives, so there is no session ID to report at boot.
	logger.Info("forza-telemetry server started",
		"udp_addr", cfg.Ingest.Addr,
		"http_addr", cfg.API.Addr,
		"data_dir", cfg.Storage.DataDir,
	)

	received := 0
	var firstErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		received++
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("component failed, shutting down", "err", err)
			firstErr = err
		}
	}
	cancel()

	// Drain ALL goroutines before returning — on the error path too. The
	// writer only finalizes the active stint's parquet footer when Run
	// returns; bailing early here used to lose the in-flight stint.
	for ; received < 3; received++ {
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("shutdown error", "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
