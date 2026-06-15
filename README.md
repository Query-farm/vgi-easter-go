# vgi-easter-go

[![CI](https://github.com/Query-farm/vgi-easter-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Query-farm/vgi-easter-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A tiny [VGI (Vector Gateway Interface)](https://github.com/Query-farm) worker —
written in **Go** — that gives DuckDB one SQL function, `easter_date(year)`,
returning the date of Western (Gregorian) Easter Sunday. It has no external data
and almost no code, which makes it a clean, copyable example of a VGI
scalar-function worker built on the [vgi-go](https://github.com/Query-farm/vgi-go)
SDK.

This is a faithful Go port of the reference Python worker, `vgi-easter-python`.

## Quick start

In DuckDB:

```sql
-- VGI isn't bundled with DuckDB yet, so load it from the community channel
-- (INSTALL is a one-time download; LOAD is once per session).
INSTALL vgi FROM community;
LOAD vgi;

ATTACH 'easter' (TYPE 'vgi', LOCATION '/path/to/vgi-easter-go');

SELECT easter.easter_date(2025);   -- 2025-04-20

SELECT year, easter.easter_date(year) AS easter
FROM range(2020, 2025) t(year);
```

DuckDB launches the worker for you (over the stdio transport), and `easter_date`
then behaves like a native function (a null year yields a null date).

## How it works

A VGI worker publishes catalogs, schemas, and functions that DuckDB can `ATTACH`
and query as if they were built in. Values cross the boundary as Apache Arrow,
so they stay columnar end to end.

This worker publishes a single function:

```
easter                                  (catalog)
└── main                                (schema)
    └── easter_date(year BIGINT) → DATE
```

The whole implementation lives in [`main.go`](main.go): the date calculation
(`easterSunday`, the Anonymous Gregorian *Computus*), a `ScalarFunction` that
maps an Arrow array of years to date32 values (with null propagation via
`vgi.MapColumn`), and a few lines wiring it into a catalog.

## Building and running

Requires Go 1.25+. The only dependency is the
[vgi-go](https://github.com/Query-farm/vgi-go) SDK (plus Apache Arrow v18).

```bash
go build -o vgi-easter-go .
```

The single binary supports every VGI transport, selected by flags:

| Invocation                                  | Transport | Use it when                                               |
| ------------------------------------------- | --------- | --------------------------------------------------------- |
| `vgi-easter-go`                             | stdio     | DuckDB spawns the worker as a subprocess (the quickstart) |
| `vgi-easter-go --http --addr 0.0.0.0:8080`  | HTTP      | you want a long-running server to attach to               |
| `vgi-easter-go --unix /path/to.sock`        | AF_UNIX   | the VGI launcher transport                                |

To run over HTTP, start the server and attach to its URL:

```bash
./vgi-easter-go --http --addr 0.0.0.0:8080
```

```sql
LOAD vgi;   -- after a one-time INSTALL vgi FROM community
ATTACH 'easter' (TYPE 'vgi', LOCATION 'http://localhost:8080');
```

## Configuration

| Variable                  | Purpose                                                                   |
| ------------------------- | ------------------------------------------------------------------------- |
| `VGI_EASTER_GIT_COMMIT`   | Reported as the catalog's `implementation_version` (preferred over build). |

The reported `implementation_version` prefers `VGI_EASTER_GIT_COMMIT`, then the
build-stamped version (`-ldflags "-X main.packageVersion=..."`), then `unknown`.
The catalog always advertises `data_version_spec = 1.0.0`.

## Development

```bash
go build ./...
go test ./...
```

The Go test suite covers the Easter calculation (including the March 22 /
April 25 extremes), the Arrow compute path (asserting the output is `date32`),
null propagation, and the version-resolution logic. A separate
[sqllogictest](https://duckdb.org/dev/sqllogictest/intro) suite in `test/sql/`
drives the worker through the **real** DuckDB `vgi` extension; CI runs both on
Linux, macOS, and Windows — see [`ci/README.md`](ci/README.md) and
[`ci/run-integration.sh`](ci/run-integration.sh).

## Deployment

[`Dockerfile`](Dockerfile) builds a static binary into a distroless runtime image
that serves the HTTP transport on port 8080, and [`fly.toml`](fly.toml) deploys
it to Fly.io with scale-to-zero.

## Built with

- **[DuckDB](https://duckdb.org)** attaches and queries the worker — install the
  VGI extension with `INSTALL vgi FROM community; LOAD vgi;`.
- **[vgi-go](https://github.com/Query-farm/vgi-go)** is the VGI worker SDK this is
  built on.
- **[Haybarn](https://github.com/Query-farm-haybarn)** provides the
  DuckDB-compatible `unittest` runner that drives the cross-platform CI.

## License

MIT — see [LICENSE](LICENSE). Copyright 2026 Query Farm LLC — https://query.farm
