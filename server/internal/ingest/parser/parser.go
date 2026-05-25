// Package parser owns Forza UDP packet decoding. A Parser maps a raw byte slice
// to a Tick in the canonical superset schema.
//
// Parsers are selected by inbound packet size (Registry.Resolve). Adding a new
// game version is a Register call with a Parser implementation — no other code
// changes (ADR 0003).
package parser

import (
	"errors"
	"fmt"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

var ErrUnknownPacket = errors.New("unknown packet format")

// Parser decodes one inbound UDP packet into a Tick.
// Implementations MUST be stateless and safe for concurrent use.
type Parser interface {
	GameVersion() tick.GameVersion
	Variant() tick.PacketVariant
	Decode(buf []byte, out *tick.Tick) error
}

// Registry maps packet sizes to parsers. Forza historically uses fixed-size
// packets per variant, so size is a sufficient discriminator.
type Registry struct {
	bySize map[int]Parser
}

func NewRegistry() *Registry {
	return &Registry{bySize: make(map[int]Parser)}
}

func (r *Registry) Register(packetSize int, p Parser) {
	r.bySize[packetSize] = p
}

func (r *Registry) Resolve(buf []byte) (Parser, error) {
	p, ok := r.bySize[len(buf)]
	if !ok {
		return nil, fmt.Errorf("%w: size=%d", ErrUnknownPacket, len(buf))
	}
	return p, nil
}

// DefaultRegistry returns a registry pre-populated with all built-in parsers.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(SledSize, NewFH5Sled())
	r.Register(HorizonDashSize, NewFH5Dash())
	// TODO: register FH6 parser once packet format is verified.
	return r
}
