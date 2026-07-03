package storage

import (
	"reflect"
	"testing"

	"github.com/benweier/forza-telemetry/server/internal/tick"
)

func parquetTags(t *testing.T, typ reflect.Type) map[string]bool {
	t.Helper()
	tags := make(map[string]bool, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("parquet")
		if tag == "" {
			t.Fatalf("%s.%s has no parquet tag", typ.Name(), typ.Field(i).Name)
		}
		if tags[tag] {
			t.Fatalf("%s has duplicate parquet tag %q", typ.Name(), tag)
		}
		tags[tag] = true
	}
	return tags
}

// parquetRow must stay in tag-lockstep with tick.Tick (row.go's stated
// invariant). A field added to Tick but not mirrored here silently produces
// parquet files missing that column — breaking /ticks channels and aggregates
// for every stint recorded until someone notices.
func TestParquetRowTagsMirrorTick(t *testing.T) {
	tickTags := parquetTags(t, reflect.TypeOf(tick.Tick{}))
	rowTags := parquetTags(t, reflect.TypeOf(parquetRow{}))

	for tag := range tickTags {
		if !rowTags[tag] {
			t.Errorf("tick.Tick parquet tag %q missing from parquetRow — mirror it in row.go", tag)
		}
	}
	for tag := range rowTags {
		if !tickTags[tag] {
			t.Errorf("parquetRow parquet tag %q has no tick.Tick counterpart — stale mirror?", tag)
		}
	}
}

// Tag parity alone doesn't prove toParquetRow copies the field. Set every
// Tick field to a nonzero value and require every parquetRow field nonzero.
func TestToParquetRowCopiesEveryField(t *testing.T) {
	var tk tick.Tick
	fillNonZero(t, reflect.ValueOf(&tk).Elem())

	row := toParquetRow(&tk)
	rv := reflect.ValueOf(row)
	for i := 0; i < rv.NumField(); i++ {
		if rv.Field(i).IsZero() {
			t.Errorf("parquetRow.%s is zero after toParquetRow on a fully nonzero Tick — copy missing in row.go",
				rv.Type().Field(i).Name)
		}
	}
}

func fillNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			f.SetInt(int64(i + 1))
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			f.SetUint(uint64(i + 1))
		case reflect.Float32, reflect.Float64:
			f.SetFloat(float64(i + 1))
		case reflect.Array:
			for j := 0; j < f.Len(); j++ {
				f.Index(j).SetFloat(float64(i + j + 1))
			}
		default:
			t.Fatalf("fillNonZero: unhandled kind %s for field %s — extend the test", f.Kind(), v.Type().Field(i).Name)
		}
	}
}
