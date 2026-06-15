# CI: the easter extension integration suite

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs the Go unit tests
and this repo's sqllogictest suite (`test/sql/*.test`) against the easter VGI
worker through the **real DuckDB `vgi` extension** on every push / PR — across
**Linux, macOS, and Windows**.

Both jobs run as an OS matrix. The integration job selects the matching
`haybarn_unittest-*` asset per platform (`linux-amd64`, `osx-arm64`,
`windows-amd64`); on Windows the bash steps run under Git Bash and the worker
LOCATION is the native path to the built `vgi-easter-go.exe` launcher.

## How it works (no C++ build)

Rather than building the vgi DuckDB extension from source, CI drives a
**prebuilt** standalone `haybarn-unittest` (the DuckDB/Haybarn sqllogictest
runner, published in Haybarn's releases) and installs the **signed** vgi
extension from the Haybarn community channel:

1. **Build the worker** — `go build -o vgi-easter-go .`. The resulting binary is
   a self-contained stdio worker the extension can spawn.
2. **Download the runner** — `haybarn_unittest-<platform>.zip` from the latest
   Haybarn release.
3. **Preprocess** — the standalone runner links none of the extensions the
   tests gate on, so [`preprocess-require.awk`](preprocess-require.awk) rewrites
   each `require <ext>` into an explicit signed `INSTALL <ext> FROM
   {community,core}; LOAD <ext>;`. `require-env` and everything else pass
   through untouched.
4. **Run** — [`run-integration.sh`](run-integration.sh) stages the preprocessed
   tree, points `VGI_EASTER_WORKER` at the built `vgi-easter-go` binary, warms
   the extension cache once, then runs the suite in a single `unittest`
   invocation. The CI log streams the runner's native report; any failed
   assertion exits non-zero and fails the job.

## Run it locally

```bash
go build -o vgi-easter-go .
REL=$(gh release view --repo Query-farm-haybarn/haybarn --json tagName --jq .tagName)
gh release download "$REL" --repo Query-farm-haybarn/haybarn \
  --pattern 'haybarn_unittest-osx-arm64.zip' --output /tmp/hb.zip --clobber
unzip -o /tmp/hb.zip -d /tmp/hb
HAYBARN_UNITTEST=/tmp/hb/haybarn-unittest \
VGI_EASTER_WORKER="$PWD/vgi-easter-go" \
  ci/run-integration.sh
```

(Swap the asset pattern for your platform: `haybarn_unittest-linux-amd64.zip`,
`haybarn_unittest-windows-amd64.zip`.)

## Haybarn version (resolved, not pinned)

The `resolve-haybarn` job in [`ci.yml`](../.github/workflows/ci.yml) queries the
**latest** published Haybarn release at run time and feeds that one tag to the
whole matrix — nothing is hardcoded. The `vgi` extension is pulled live from the
community channel (`INSTALL vgi FROM community`), which always serves the
currently published build, so CI verifies the worker against what users can
actually install today.
