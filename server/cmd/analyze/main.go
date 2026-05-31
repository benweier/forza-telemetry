// Command analyze decodes a captured fh6 packet log and prints field statistics,
// used for reverse-engineering unknown packet fields against real telemetry.
// Not part of the server build; run manually:
//
//	go run ./cmd/analyze <logpath> [logpath...]
//
// Each log line is "<rfc3339> <size> <hex>". Gzip (.gz) inputs are handled.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
)

// Offsets within the 324-byte FH6 packet (little-endian). From the current
// parser: Sled[0:232], FH6 insertion[232:244], FH5 tail[244:323], byte[323].
const (
	offIsRaceOn     = 0
	offCarGroup     = 232
	offBestLap      = 296
	offLastLap      = 300
	offCurrentLap   = 304
	offCurrentRace  = 308
	offLapNumber    = 312
	offRacePosition = 314
	offSteer        = 320
	offDrivingLine  = 321
	offAIBrakeDiff  = 322
	offReserved323  = 323
	packetLen       = 324
)

func f32(b []byte, o int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b[o:])) }
func u16(b []byte, o int) uint16  { return binary.LittleEndian.Uint16(b[o:]) }
func s32(b []byte, o int) int32   { return int32(binary.LittleEndian.Uint32(b[o:])) }

type i8stats struct {
	min, max int
	seen     map[int]int
}

func newI8() *i8stats { return &i8stats{min: 1 << 30, max: -(1 << 30), seen: map[int]int{}} }
func (s *i8stats) add(v int) {
	if v < s.min {
		s.min = v
	}
	if v > s.max {
		s.max = v
	}
	if len(s.seen) < 64 {
		s.seen[v]++
	}
}
func (s *i8stats) distinct() []int {
	o := make([]int, 0, len(s.seen))
	for k := range s.seen {
		o = append(o, k)
	}
	sort.Ints(o)
	return o
}

type f32stats struct {
	min, max float32
	nonZero  int
	total    int
}

func newF32() *f32stats { return &f32stats{min: math.MaxFloat32, max: -math.MaxFloat32} }
func (s *f32stats) add(v float32) {
	s.total++
	if v != 0 {
		s.nonZero++
	}
	if v < s.min {
		s.min = v
	}
	if v > s.max {
		s.max = v
	}
}
func (s *f32stats) String() string {
	if s.total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("min=%.3f max=%.3f nonzero=%d/%d", s.min, s.max, s.nonZero, s.total)
}

func openMaybeGzip(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return struct {
			io.Reader
			io.Closer
		}{gz, f}, nil
	}
	return f, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: analyze <logpath> [logpath...]")
		os.Exit(1)
	}

	var total, raceOn, badLen int
	driving, aiBrake, reserved, racePos, steer := newI8(), newI8(), newI8(), newI8(), newI8()
	bestLap, lastLap, curLap, curRace := newF32(), newF32(), newF32(), newF32()
	lapNumMax := 0
	carGroups := map[int32]int{}
	var example []byte

	for _, path := range os.Args[1:] {
		rc, err := openMaybeGzip(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", path, err)
			continue
		}
		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			parts := strings.Fields(sc.Text())
			if len(parts) < 3 {
				continue
			}
			b, err := hex.DecodeString(parts[2])
			if err != nil || len(b) != packetLen {
				badLen++
				continue
			}
			total++
			if s32(b, offIsRaceOn) == 0 {
				continue // menu/idle packets carry zeros; skip
			}
			raceOn++
			driving.add(int(int8(b[offDrivingLine])))
			aiBrake.add(int(int8(b[offAIBrakeDiff])))
			reserved.add(int(b[offReserved323]))
			racePos.add(int(b[offRacePosition]))
			steer.add(int(int8(b[offSteer])))
			if ln := int(u16(b, offLapNumber)); ln > lapNumMax {
				lapNumMax = ln
			}
			bestLap.add(f32(b, offBestLap))
			lastLap.add(f32(b, offLastLap))
			curLap.add(f32(b, offCurrentLap))
			curRace.add(f32(b, offCurrentRace))
			carGroups[s32(b, offCarGroup)]++
			if example == nil && b[offRacePosition] > 0 {
				example = append([]byte(nil), b...)
			}
		}
		rc.Close()
	}

	fmt.Printf("packets=%d race_on=%d bad_len=%d\n", total, raceOn, badLen)
	fmt.Printf("lap_number   u16@312: max=%d\n", lapNumMax)
	fmt.Printf("race_position u8@314: min=%d max=%d distinct=%v\n", racePos.min, racePos.max, racePos.distinct())
	fmt.Printf("steer         s8@320: min=%d max=%d\n", steer.min, steer.max)
	fmt.Printf("driving_line  s8@321: min=%d max=%d distinct=%v\n", driving.min, driving.max, driving.distinct())
	fmt.Printf("ai_brakediff  s8@322: min=%d max=%d distinct=%v\n", aiBrake.min, aiBrake.max, aiBrake.distinct())
	fmt.Printf("reserved      u8@323: min=%d max=%d distinct=%v\n", reserved.min, reserved.max, reserved.distinct())
	fmt.Printf("best_lap     f32@296: %s\n", bestLap)
	fmt.Printf("last_lap     f32@300: %s\n", lastLap)
	fmt.Printf("current_lap  f32@304: %s\n", curLap)
	fmt.Printf("current_race f32@308: %s\n", curRace)
	cg := make([]int, 0, len(carGroups))
	for k := range carGroups {
		cg = append(cg, int(k))
	}
	sort.Ints(cg)
	fmt.Printf("car_group    s32@232: distinct=%v\n", cg)

	if example != nil {
		fmt.Printf("\nexample race packet (pos=%d lap=%d) bytes 312..323:\n", example[offRacePosition], u16(example, offLapNumber))
		for i := 312; i < packetLen; i++ {
			fmt.Printf("  [%d]=0x%02x (%d)\n", i, example[i], int8(example[i]))
		}
	}
}
