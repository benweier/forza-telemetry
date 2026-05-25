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

// Envelope frames every outbound message on the WebSocket. Control messages are
// JSON-encoded; telemetry frames are MessagePack (ADR 0004).
type envelopeKind uint8

const (
	envHello     envelopeKind = 1
	envTickFrame envelopeKind = 2
	envError     envelopeKind = 3
)

type helloMsg struct {
	Kind       envelopeKind `msgpack:"k"`
	ServerTime int64        `msgpack:"st"`
	RingReplay int          `msgpack:"rr"`
	ProtoVer   int          `msgpack:"pv"`
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

	sub := s.broker.Subscribe(true)
	defer sub.Close()

	hello := helloMsg{
		Kind:       envHello,
		ServerTime: time.Now().UnixNano(),
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
	return conn.Write(ctx, websocket.MessageBinary, b)
}
