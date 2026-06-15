// Copyright 2026 Query Farm LLC - https://query.farm

// Command vgi-easter-go is a VGI worker that computes the date of Easter
// Sunday for a given year.
//
// It provides a single scalar function, easter_date(year), that returns the
// Gregorian (Western) date of Easter Sunday using the Anonymous Gregorian
// algorithm (a.k.a. the Meeus/Jones/Butcher Computus). Pure standard library —
// no external data is needed for the calculation.
//
// This is a faithful Go port of the reference Python worker (vgi-easter-python).
//
// Usage:
//
//	vgi-easter-go              # stdio transport (DuckDB spawns the worker)
//	vgi-easter-go --http       # HTTP transport (long-running server)
//	vgi-easter-go --unix PATH  # AF_UNIX launcher transport
//
//	SELECT easter_date(2025);
//	SELECT year, easter_date(year) AS easter FROM range(2020, 2031) t(year);
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Query-farm/vgi-go/vgi"
	"github.com/Query-farm/vgi-rpc-go/vgirpc"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

const (
	// CatalogName is the catalog this worker publishes.
	CatalogName = "easter"
	// DataVersion is the stable data version advertised by the catalog.
	DataVersion = "1.0.0"
)

// implementationVersion is the version reported as the catalog's
// implementation_version. It prefers an explicit git SHA from
// VGI_EASTER_GIT_COMMIT (handy in CI/dev builds); otherwise it falls back to
// the build-stamped packageVersion, and "unknown" only if neither is available.
//
// packageVersion may be overridden at build time via:
//
//	go build -ldflags "-X main.packageVersion=0.1.2"
var packageVersion = ""

func implementationVersion() string {
	if sha := os.Getenv("VGI_EASTER_GIT_COMMIT"); sha != "" {
		return sha
	}
	if packageVersion != "" {
		return packageVersion
	}
	return "unknown"
}

// easterSunday returns the Gregorian date of Easter Sunday for year.
//
// It uses the Anonymous Gregorian algorithm (Meeus/Jones/Butcher Computus),
// valid for any year in the Gregorian calendar (1583 onward). The integer
// arithmetic mirrors the Python reference exactly.
func easterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	ell := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*ell) / 451
	month := (h + ell - 7*m + 114) / 31
	day := ((h + ell - 7*m + 114) % 31) + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// EasterDateFunction is a scalar function returning the date of Easter Sunday
// for a given year.
//
// It demonstrates a minimal scalar function with:
//   - a single int64 column input,
//   - an explicit date32 output type, and
//   - null propagation (a null year yields a null date).
type EasterDateFunction struct{}

func (*EasterDateFunction) Name() string { return "easter_date" }

func (*EasterDateFunction) Metadata() vgi.FunctionMetadata {
	return vgi.FunctionMetadata{
		Description: "Date of Western (Gregorian) Easter Sunday for a given year",
		Stability:   vgi.StabilityConsistent,
		ReturnType:  arrow.FixedWidthTypes.Date32,
	}
}

func (*EasterDateFunction) ArgumentSpecs() []vgi.ArgSpec {
	return []vgi.ArgSpec{
		{Name: "year", Position: 0, ArrowType: "int64", Doc: "Year, e.g. 2025 (Gregorian, >= 1583)"},
	}
}

// Examples mirrors the Python reference worker's Meta.examples. The vgi-go SDK
// does not currently surface function examples through the catalog discovery
// path, so these are kept here for documentation parity. See README.
func (*EasterDateFunction) Examples() []vgi.CatalogExample {
	return []vgi.CatalogExample{
		{
			SQL:         "SELECT easter_date(2025)",
			Description: "Easter Sunday in 2025 (2025-04-20)",
		},
		{
			SQL:         "SELECT year, easter_date(year) AS easter FROM range(2020, 2031) t(year)",
			Description: "Easter dates for 2020 through 2030",
		},
	}
}

func (*EasterDateFunction) OnBind(_ *vgi.BindParams) (*vgi.BindResponse, error) {
	return vgi.BindResult(arrow.FixedWidthTypes.Date32)
}

func (*EasterDateFunction) Process(_ context.Context, params *vgi.ProcessParams, batch arrow.RecordBatch) (arrow.RecordBatch, error) {
	return vgi.MapColumn(params, batch, 0, array.NewDate32Builder,
		func(col arrow.Array, i int) arrow.Date32 {
			year := vgi.GetInt64Value(col, i)
			return arrow.Date32FromTime(easterSunday(int(year)))
		})
}

// newWorker builds the Easter worker with its catalog and scalar function
// registered. Extracted so tests can construct it as well.
func newWorker() *vgi.Worker {
	impl := implementationVersion()
	dvs := DataVersion

	w := vgi.NewWorker(
		vgi.WithCatalogName(CatalogName),
		vgi.WithCatalogComment("Computus: the date of Western (Gregorian) Easter Sunday"),
		vgi.WithSchemaComments(map[string]string{
			"main": "Computus: the date of Western (Gregorian) Easter Sunday",
		}),
		vgi.WithCatalogInfo(vgi.CatalogInfo{
			Name:                  CatalogName,
			ImplementationVersion: &impl,
			DataVersionSpec:       &dvs,
		}),
		// Faithful port of the Python catalog_attach override: report the
		// resolved data/implementation versions back to the client at ATTACH.
		vgi.WithAttachValidator(func(_ *vgi.CatalogAttachRequestWire, _ *vgirpc.CallContext) (*vgi.AttachDecision, error) {
			return &vgi.AttachDecision{
				ResolvedDataVersion:           DataVersion,
				ResolvedImplementationVersion: impl,
			}, nil
		}),
	)

	w.RegisterScalar(&EasterDateFunction{})
	return w
}

func main() {
	httpMode := flag.Bool("http", false, "Run as HTTP server instead of stdio")
	unixPath := flag.String("unix", "", "Bind to this AF_UNIX socket path (launcher transport); mutually exclusive with --http")
	idleTimeout := flag.Float64("idle-timeout", 300, "Self-shutdown after N seconds idle when serving --unix (0 = never)")
	httpAddr := flag.String("addr", "0.0.0.0:8080", "HTTP listen address (used with --http)")
	// Accepted for launcher compatibility (no-ops here).
	flag.Bool("describe", true, "Enable description pages (accepted for launcher compatibility)")
	flag.Bool("no-describe", false, "Disable description pages (accepted for launcher compatibility)")
	logFlags := vgi.RegisterLoggingFlags(flag.CommandLine)

	// The launcher varies a worker's argv to produce distinct cache keys for the
	// same binary; tolerate unknown flags rather than failing to start.
	flag.CommandLine.Parse(filterKnownFlags(os.Args[1:], map[string]bool{
		"unix":         true,
		"idle-timeout": true,
		"addr":         true,
		"log-level":    true,
		"log-format":   true,
		"log-logger":   true,
	}))

	if err := logFlags.Apply(); err != nil {
		log.Fatalf("logging flags: %v", err)
	}

	if *unixPath != "" && *httpMode {
		log.Fatal("--unix and --http are mutually exclusive")
	}

	w := newWorker()

	switch {
	case *unixPath != "":
		if err := w.RunUnix(*unixPath, time.Duration(*idleTimeout*float64(time.Second))); err != nil {
			log.Fatal(err)
		}
	case *httpMode:
		if err := w.RunHttp(*httpAddr); err != nil {
			log.Fatal(err)
		}
	default:
		w.RunStdio()
	}
}

// filterKnownFlags drops command-line tokens for flags this binary doesn't
// define, so launcher-injected argv-differentiation flags (e.g. --threaded,
// --quiet) don't abort flag parsing. Flags named in valueFlags consume the
// following token as their value (when not given in --flag=value form).
func filterKnownFlags(args []string, valueFlags map[string]bool) []string {
	defined := map[string]bool{}
	flag.CommandLine.VisitAll(func(f *flag.Flag) { defined[f.Name] = true })
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			continue // stray positional
		}
		name := strings.TrimLeft(a, "-")
		hasInlineValue := strings.ContainsRune(name, '=')
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if !defined[name] {
			continue // unknown flag — ignore
		}
		out = append(out, a)
		if valueFlags[name] && !hasInlineValue && i+1 < len(args) {
			i++
			out = append(out, args[i])
		}
	}
	return out
}
