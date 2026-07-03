package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// Envelope frames every outbound message on the WebSocket — HELLO included,
// everything is MessagePack (ADR 0004; an earlier draft said JSON for control
// frames, but nothing ever shipped that way).
type envelopeKind uint8

const (
	envHello     envelopeKind = 1
	envTickFrame envelopeKind = 2
)

type helloMsg struct {
	Kind       envelopeKind `msgpack:"k"`
	ServerTime int64        `msgpack:"st"`
	// RingReplay is how many buffered ticks precede live frames on this socket.
	RingReplay int `msgpack:"rr"`
	ProtoVer   int `msgpack:"pv"`
}

type tickFrame struct {
	Kind envelopeKind `msgpack:"k"`
	Tick *tick.Tick   `msgpack:"t"`
}

func (s *Server) handleLiveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Single-user LAN: explicit origin checking off by default. Tighten via config later.
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.logger.Warn("ws accept", "err", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// We never expect inbound data, but coder/websocket needs a reader running to
	// process pong/close control frames — without it conn.Ping never sees its pong
	// (times out every cycle) and client disconnects go undetected. CloseRead spawns
	// that reader and returns a ctx cancelled when the peer goes away.
	ctx = conn.CloseRead(ctx)

	sub := s.broker.Subscribe(true)
	defer sub.Close()

	hello := helloMsg{
		Kind:       envHello,
		ServerTime: time.Now().UnixNano(),
		RingReplay: len(sub.C()),
		ProtoVer:   1,
	}
	if err := writeMsgpack(ctx, conn, hello); err != nil {
		return
	}

	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pingTicker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				return
			}
		case t, ok := <-sub.C():
			if !ok {
				return
			}
			frame := tickFrame{Kind: envTickFrame, Tick: t}
			if err := writeMsgpack(ctx, conn, frame); err != nil {
				if !errors.Is(err, context.Canceled) {
					s.logger.Debug("ws write", "err", err)
				}
				return
			}
		}
	}
}

func writeMsgpack(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := msgpack.Marshal(v)
	if err != nil {
		return err
	}
	// Bound the write so a wedged TCP send window can't park the sole drainer
	// indefinitely (which would silently fill the broker buffer and drop ticks).
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageBinary, b)
}
