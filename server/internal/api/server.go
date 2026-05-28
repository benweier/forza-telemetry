// Package api hosts the HTTP and WebSocket surface: REST endpoints for historical
// queries and a single WebSocket for the live stream + control envelope.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/benweier/forza-telemetry/server/internal/config"
	"github.com/benweier/forza-telemetry/server/internal/storage"
	"github.com/benweier/forza-telemetry/server/internal/stream"
	"github.com/benweier/forza-telemetry/server/internal/web"
)

type Server struct {
	addr   string
	broker *stream.Broker
	store  *storage.Store
	logger *slog.Logger
	mux    *http.ServeMux
}

func New(cfg config.APIConfig, broker *stream.Broker, store *storage.Store, logger *slog.Logger) *Server {
	s := &Server{
		addr:   cfg.Addr,
		broker: broker,
		store:  store,
		logger: logger.With("component", "api"),
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	s.mux.HandleFunc("GET /api/v1/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("PATCH /api/v1/sessions/{id}", s.handlePatchSession)
	s.mux.HandleFunc("POST /api/v1/sessions/{id}/downsample", s.handleDownsampleSession)

	s.mux.HandleFunc("GET /api/v1/stints/{id}", s.handleGetStint)
	s.mux.HandleFunc("GET /api/v1/stints/{id}/laps", s.handleListLaps)
	s.mux.HandleFunc("GET /api/v1/stints/{id}/hot-spots", s.handleListHotSpots)
	s.mux.HandleFunc("GET /api/v1/stints/{id}/turns", s.handleListTurns)
	s.mux.HandleFunc("GET /api/v1/stints/{id}/straights", s.handleListStraights)
	s.mux.HandleFunc("GET /api/v1/stints/{id}/preview", s.handleListPreview)
	s.mux.HandleFunc("GET /api/v1/stints/{id}/ticks", s.handleListTicks)
	s.mux.HandleFunc("GET /api/v1/stints/{id}/path", s.handleListPath)

	s.mux.HandleFunc("GET /api/v1/live", s.handleLiveWS)

	// SPA: serve the embedded build at root with SPA fallback.
	s.mux.Handle("/", web.Handler())
}

func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("HTTP server listening", "addr", s.addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
