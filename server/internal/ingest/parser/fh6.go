package parser

import (
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

// FH6PacketSize is the verified wire size of the Forza Horizon 6 "Car Dash"
// packet (Forza Support documentation, May 2026). FH6 has a single packet
// format — no Sled / Dash split.
const FH6PacketSize = 324

// FH6 wire layout, derived empirically from a capture of ~9.5k race-on
// packets sampled against the known FH5 layout (May 2026):
//
//	0   – 231  Sled prefix (byte-for-byte FH5 Sled)
//	232 – 235  CarGroup        i32   (FH6 addition)
//	236 – 239  SmashableVelDiff f32  (FH6 addition; non-zero on smashable hits)
//	240 – 243  SmashableMass    f32  (FH6 addition; non-zero on smashable hits)
//	244 – 322  Horizon tail, byte-for-byte FH5 layout shifted by +12
//	323        reserved u8 (always zero in captures; semantics unknown)
//
// The Forza Support docs say "CarGroup, SmashableVelDiff, SmashableMass inserted
// after NumCylinders and before PositionX" — that accounts for 12 bytes. The
// 13th byte is the trailing reserved at offset 323; treat as unknown until
// future captures reveal a use.
//
// CarGroup is decoded as i32 for consistency with the other Sled car-identity
// fields (CarOrdinal/Class/PI/Drivetrain/NumCylinders). Observed values fit
// inside u8 but reading 4 bytes is safe because bytes 233–235 are always zero
// in captures and there is no documented sub-field there yet.

// FH6Dash parses the 324-byte Forza Horizon 6 Car Dash packet.
//
// When captureWriter is non-nil, every successfully size-checked packet is
// appended as `<RFC3339Nano-UTC> <size> <hex>\n` so the trailing reserved
// byte and the (currently empty) CarGroup/Smashable fields can be analysed
// against future captures.
type FH6Dash struct {
	mu      sync.Mutex
	capture io.Writer
	clock   func() time.Time
}

// NewFH6Dash constructs the FH6 parser. If capture is nil, capture logging
// is disabled — decode still runs normally.
func NewFH6Dash(capture io.Writer) *FH6Dash {
	return &FH6Dash{capture: capture, clock: time.Now}
}

func (*FH6Dash) GameVersion() tick.GameVersion { return tick.GameFH6 }
func (*FH6Dash) Variant() tick.PacketVariant   { return tick.VariantHorizon6Dash }

func (p *FH6Dash) Decode(buf []byte, out *tick.Tick) error {
	if len(buf) != FH6PacketSize {
		return fmt.Errorf("fh6: expected %d bytes, got %d", FH6PacketSize, len(buf))
	}
	d := decoder{buf: buf}
	decodeSled(&d, out)
	decodeFH6Insertion(&d, out)
	decodeHorizonTail(&d, out)
	_ = d.u8() // byte 323: confirmed always 0 across 716k race-on packets — reserved padding, not a field

	if p.capture != nil {
		p.logPacket(buf)
	}
	return nil
}

// decodeFH6Insertion reads the 12-byte block between NumCylinders (end of
// Sled at offset 232) and PositionX (start of FH5-style tail at offset 244).
func decodeFH6Insertion(d *decoder, out *tick.Tick) {
	out.CarGroup = d.i32()
	out.SmashableVelDiff = d.f32()
	out.SmashableMass = d.f32()
}

// logPacket appends one capture line; errors swallowed so capture failure
// never breaks ingest.
func (p *FH6Dash) logPacket(buf []byte) {
	line := fmt.Sprintf("%s %d %s\n",
		p.clock().UTC().Format(time.RFC3339Nano),
		len(buf),
		hex.EncodeToString(buf),
	)
	p.mu.Lock()
	_, _ = io.WriteString(p.capture, line)
	p.mu.Unlock()
}
