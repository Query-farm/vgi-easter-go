// Copyright 2026 Query Farm LLC - https://query.farm

package main

import (
	"context"
	"testing"
	"time"

	"github.com/Query-farm/vgi-go/vgi"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// date is a small helper to build a UTC date for comparisons.
func date(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// knownEasters are known Western (Gregorian) Easter Sunday dates, mirroring the
// Python test suite (including the March 22 / April 25 extremes).
var knownEasters = []struct {
	year     int
	expected time.Time
}{
	{2020, date(2020, 4, 12)},
	{2021, date(2021, 4, 4)},
	{2022, date(2022, 4, 17)},
	{2023, date(2023, 4, 9)},
	{2024, date(2024, 3, 31)},
	{2025, date(2025, 4, 20)},
	{2026, date(2026, 4, 5)},
	{2027, date(2027, 3, 28)},
	{2030, date(2030, 4, 21)},
	{1818, date(1818, 3, 22)}, // earliest possible Easter (March 22)
	{1943, date(1943, 4, 25)}, // latest possible Easter (April 25)
}

func TestEasterSunday(t *testing.T) {
	for _, tc := range knownEasters {
		got := easterSunday(tc.year)
		if !got.Equal(tc.expected) {
			t.Errorf("easterSunday(%d) = %s; want %s",
				tc.year, got.Format("2006-01-02"), tc.expected.Format("2006-01-02"))
		}
	}
}

// computeEaster runs the EasterDateFunction over a single int64 input column
// and returns the resulting date32 array, so tests can assert type and values.
// nulls[i]==true makes row i a NULL input year.
func computeEaster(t *testing.T, years []int64, nulls []bool) *array.Date32 {
	t.Helper()
	mem := memory.NewGoAllocator()

	b := array.NewInt64Builder(mem)
	defer b.Release()
	for i, y := range years {
		if nulls != nil && nulls[i] {
			b.AppendNull()
		} else {
			b.Append(y)
		}
	}
	col := b.NewArray()
	defer col.Release()

	inputSchema := arrow.NewSchema([]arrow.Field{
		{Name: "year", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil)
	inBatch := array.NewRecordBatch(inputSchema, []arrow.Array{col}, int64(len(years)))
	defer inBatch.Release()

	outputSchema := arrow.NewSchema([]arrow.Field{
		{Name: "result", Type: arrow.FixedWidthTypes.Date32},
	}, nil)

	fn := &EasterDateFunction{}
	out, err := fn.Process(context.Background(), &vgi.ProcessParams{OutputSchema: outputSchema}, inBatch)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	t.Cleanup(out.Release)

	if out.NumCols() != 1 {
		t.Fatalf("expected 1 output column, got %d", out.NumCols())
	}
	result, ok := out.Column(0).(*array.Date32)
	if !ok {
		t.Fatalf("expected *array.Date32 output, got %T", out.Column(0))
	}
	return result
}

func TestComputeBatch(t *testing.T) {
	result := computeEaster(t, []int64{2024, 2025, 2026}, nil)

	// Output column must be date32.
	if result.DataType().ID() != arrow.DATE32 {
		t.Fatalf("expected date32 output, got %s", result.DataType())
	}

	want := []time.Time{date(2024, 3, 31), date(2025, 4, 20), date(2026, 4, 5)}
	if result.Len() != len(want) {
		t.Fatalf("expected %d rows, got %d", len(want), result.Len())
	}
	for i, w := range want {
		got := result.Value(i).ToTime()
		if !got.Equal(w) {
			t.Errorf("row %d = %s; want %s", i, got.Format("2006-01-02"), w.Format("2006-01-02"))
		}
	}
}

func TestComputeNullPropagation(t *testing.T) {
	result := computeEaster(t, []int64{2025, 0, 2026}, []bool{false, true, false})

	if result.IsNull(0) {
		t.Errorf("row 0 should not be null")
	}
	if !result.IsNull(1) {
		t.Errorf("row 1 (null year) should be null")
	}
	if result.IsNull(2) {
		t.Errorf("row 2 should not be null")
	}
	if got := result.Value(0).ToTime(); !got.Equal(date(2025, 4, 20)) {
		t.Errorf("row 0 = %s; want 2025-04-20", got.Format("2006-01-02"))
	}
	if got := result.Value(2).ToTime(); !got.Equal(date(2026, 4, 5)) {
		t.Errorf("row 2 = %s; want 2026-04-05", got.Format("2006-01-02"))
	}
}

func TestImplementationVersionPrefersGitCommit(t *testing.T) {
	t.Setenv("VGI_EASTER_GIT_COMMIT", "deadbeef")
	if got := implementationVersion(); got != "deadbeef" {
		t.Errorf("implementationVersion() = %q; want %q", got, "deadbeef")
	}
}

func TestImplementationVersionFallsBack(t *testing.T) {
	t.Setenv("VGI_EASTER_GIT_COMMIT", "")

	// With no git SHA and no build-stamped version, it reports "unknown".
	orig := packageVersion
	packageVersion = ""
	defer func() { packageVersion = orig }()
	if got := implementationVersion(); got != "unknown" {
		t.Errorf("implementationVersion() = %q; want %q", got, "unknown")
	}

	// With a build-stamped package version set, it reports that.
	packageVersion = "0.1.2"
	if got := implementationVersion(); got != "0.1.2" {
		t.Errorf("implementationVersion() = %q; want %q", got, "0.1.2")
	}
}

// TestNewWorker exercises the worker construction path so registration wiring
// is covered by the unit suite.
func TestNewWorker(t *testing.T) {
	w := newWorker()
	if w == nil {
		t.Fatal("newWorker() returned nil")
	}
}
