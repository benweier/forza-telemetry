// Package ingest owns the UDP listen loop and the parse→enrich→publish path.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/benweier/forza-telemetry/server/internal/config"
	"github.com/benweier/forza-telemetry/server/internal/ingest/parser"
	"github.com/benweier/forza-telemetry/server/internal/stream"
	"github.com/benweier/forza-telemetry/server/internal/tick"
)

const readBufSize = 2048

// Listener reads UDP packets, parses them into Ticks, enriches them, and
// publishes to the stream broker. One Listener per server process.
type Listener struct {
	addr       string
	registry   *parser.Registry
	broker     *stream.Broker
	logger     *slog.Logger
	captureLog *os.File

	mu       sync.Mutex
	prevTick tick.Tick
	hasPrev  bool
}

func NewListener(cfg config.IngestConfig, broker *stream.Broker, logger *slog.Logger) (*Listener, error) {
	reg := parser.DefaultRegistry()
	l := &Listener{
		addr:     cfg.Addr,
		registry: reg,
		broker:   broker,
		logger:   logger.With("component", "ingest"),
	}
	if cfg.FH6CaptureLog != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.FH6CaptureLog), 0o755); err != nil {
			return nil, fmt.Errorf("create fh6 capture dir: %w", err)
		}
		f, err := os.OpenFile(cfg.FH6CaptureLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open fh6 capture log: %w", err)
		}
		l.captureLog = f
		reg.Register(parser.FH6PacketSize, parser.NewFH6Dash(f))
		l.logger.Info("fh6 capture log enabled", "path", cfg.FH6CaptureLog)
	} else {
		reg.Register(parser.FH6PacketSize, parser.NewFH6Dash(nil))
	}
	return l, nil
}

func (l *Listener) Run(ctx context.Context) error {
	udpAddr, err := net.ResolveUDPAddr("udp", l.addr)
	if err != nil {
		return fmt.Errorf("resolve udp addr %q: %w", l.addr, err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()
	if l.captureLog != nil {
		defer l.captureLog.Close()
	}

	l.logger.Info("UDP listener bound", "addr", conn.LocalAddr().String())

	go func() {
		<-ctx.Done()
		_ = conn.SetReadDeadline(time.Now())
	}()

	buf := make([]byte, readBufSize)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			l.logger.Warn("udp read", "err", err)
			continue
		}

		packet := buf[:n]
		recvNS := time.Now().UnixNano()

		l.dispatch(packet, recvNS)
	}
}

func (l *Listener) dispatch(packet []byte, recvNS int64) {
	p, err := l.registry.Resolve(packet)
	if err != nil {
		l.logger.Debug("unknown packet", "size", len(packet))
		return
	}

	var t tick.Tick
	if err := p.Decode(packet, &t); err != nil {
		l.logger.Warn("decode", "err", err)
		return
	}

	t.GameVersion = p.GameVersion()
	t.PacketVariant = p.Variant()
	t.ServerRecvNS = recvNS

	l.mu.Lock()
	var prev *tick.Tick
	if l.hasPrev {
		prev = &l.prevTick
	}
	t.Enrich(prev)
	l.prevTick = t
	l.hasPrev = true
	l.mu.Unlock()

	l.broker.Publish(&t)
}
