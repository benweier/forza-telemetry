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

	// Field without msgpack tag is skipped.
	Internal string ` + "`parquet:\"internal\"`" + `

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
		"Internal",  // no msgpack tag
		"Skipped",   // msgpack:"-"
		"internal:", // parquet tag should not leak
	}
	for _, w := range unwants {
		if strings.Contains(out, w) {
			t.Errorf("output contains unexpected %q\n---\n%s", w, out)
		}
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
