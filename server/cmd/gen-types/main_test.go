package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `package tick

type GameVersion uint8

const (
	GameUnknown GameVersion = iota
	GameFH5
)

type PacketVariant uint8

const (
	VariantUnknown PacketVariant = iota
	VariantSled
)

type Tick struct {
	// --- Metadata ---
	GameVersion   GameVersion   ` + "`msgpack:\"gv\"`" + `
	PacketVariant PacketVariant ` + "`msgpack:\"pv\"`" + `
	GameTSMillis  uint32        ` + "`msgpack:\"gts\"`" + `

	// --- Race state ---
	IsRaceOn  bool   ` + "`msgpack:\"race\"`" + `
	LapNumber uint16 ` + "`msgpack:\"lap\"`" + `

	// --- Per-wheel ---
	TireTemp [4]float32 ` + "`msgpack:\"tt\"`" + `

	// Explicitly excluded — the only sanctioned way to keep a field off the wire.
	Internal string ` + "`msgpack:\"-\" parquet:\"internal\"`" + `

	// Explicit skip.
	Skipped float32 ` + "`msgpack:\"-\"`" + `
}
`

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tick.go")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := generate(path, &buf); err != nil {
		t.Fatalf("generate: %v", err)
	}
	out := buf.String()

	wants := []string{
		"export const GameVersion = {",
		"Unknown: 0,",
		"FH5: 1,",
		"export type GameVersion = (typeof GameVersion)[keyof typeof GameVersion];",
		"export const PacketVariant = {",
		"Sled: 1,",
		"export type Quad = readonly [number, number, number, number];",
		"export interface TickFrame {",
		"  // Metadata",
		"  gv: GameVersion;",
		"  pv: PacketVariant;",
		"  gts: number;",
		"  race: boolean;",
		"  lap: number;",
		"  tt: Quad;",
		"}",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}

	unwants := []string{
		"Internal",  // msgpack:"-"
		"Skipped",   // msgpack:"-"
		"internal:", // parquet tag should not leak
	}
	for _, w := range unwants {
		if strings.Contains(out, w) {
			t.Errorf("output contains unexpected %q\n---\n%s", w, out)
		}
	}
}

// A field the generator can't emit must be a hard error, never a silent skip —
// exit 0 with a missing TS field is exactly the drift gen-types exists to stop.
func TestGenerateRejectsUnemittableFields(t *testing.T) {
	cases := map[string]struct {
		field   string
		wantErr string
	}{
		"missing msgpack tag": {"Internal string `parquet:\"internal\"`", "no msgpack tag"},
		"missing tag entirely": {"Internal string", "no struct tag"},
		"unmapped type":        {"Weird [2]float32 `msgpack:\"wd\"`", "no TS mapping"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := strings.Replace(fixture,
				"// Explicitly excluded — the only sanctioned way to keep a field off the wire.\n\tInternal string `msgpack:\"-\" parquet:\"internal\"`",
				tc.field, 1)
			path := filepath.Join(t.TempDir(), "tick.go")
			if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			err := generate(path, &buf)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestGenerateRealTickFile(t *testing.T) {
	// Smoke-test against the actual tick.go so drift between the canonical
	// schema and the generator is caught in CI, not in production.
	var buf bytes.Buffer
	if err := generate("../../internal/tick/tick.go", &buf); err != nil {
		t.Fatalf("generate real tick.go: %v", err)
	}
	out := buf.String()
	for _, w := range []string{
		"gv: GameVersion;",
		"pv: PacketVariant;",
		"FH5: 1,",
		"FH6: 2,",
		"Sled: 1,",
		"HorizonDash: 2,",
		"tsr: Quad;",
		"stm: Quad;",
		"lg: number;", // LateralG enriched
		"ld: number;", // LapDistanceM enriched
	} {
		if !strings.Contains(out, w) {
			t.Errorf("real tick.go output missing %q", w)
		}
	}
}
