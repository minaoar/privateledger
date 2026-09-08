# Build Instructions

Covers all units built on this project. Newest first.

---

# Unit: sic-mcc-categorization (UOW-1 through UOW-5)

**Branch**: `support-mcc-for-category` · **Date**: 2026-09-07 · **GitHub issue #5**

## Toolchain

| Item | Value |
|---|---|
| Language | Go, module declares `go 1.21`; built and verified on `go1.26.0 darwin/arm64` |
| Database driver | `modernc.org/sqlite v1.34.4` — pure Go, **no CGO** |
| HTTP | `github.com/gin-gonic/gin v1.10.0` |
| OFX parsing | `github.com/aclindsa/ofxgo v0.1.3` |
| Property testing | `pgregory.net/rapid v1.1.0` — **test-only** |

No dependency was added, removed or upgraded across UOW-1 through UOW-5. `go.mod` and `go.sum` are
unchanged by this issue's work.

The pure-Go SQLite driver is the reason cross-compilation needs no toolchain per target.

## Build

```bash
make build          # ./privateledger for the host platform
make build-all      # dist/ for linux, darwin, windows on amd64 and arm64
```

`build-all` produces five binaries. Version and build time are stamped through `-ldflags`, defaulting to
`VERSION=dev`; pass `VERSION=x.y.z` for a release build.

## What the binary contains

HTML templates and static assets are embedded with `go:embed` (`cmd/privateledger/embed.go`), so the
binary is self-contained. A template change is a **compile-time** change: rebuild to see it, and the
page tests read templates through `embeddedFiles`, so a template that fails to embed fails the build,
not the browser.

## First run

`config.json` is created beside the binary on first start. Defaults: port 8844, `start_of_month` 1,
file logging off.

`sic_mappings.csv`, if present beside the binary, is imported at startup **only when the mapping table
is empty**. A header carrying a UTF-8 BOM, lowercase names, or padded columns is accepted (UOW-4); a row
naming a category that no longer exists is rejected with a message identifying the value and the current
name behind its `Category_ID`.

## Verification before shipping

```bash
gofmt -l ./cmd ./internal     # must print nothing
go vet ./...                  # must be clean
go build ./...                # must be clean
```

`make clean` removes the binary, `dist/`, `config.json` **and `privateledger.db`**. It deletes local
data — do not run it against a directory holding real transactions.

---

# Unit: uncategorized-dashboard and earlier

**Branch**: `show-uncategorized-transactions` · **Date**: 2026-08-03. Retained verbatim; predates the per-unit heading convention.

## Prerequisites

- **Build Tool**: Go (module-based) + GNU Make
- **Go Version**: as declared in `go.mod`
- **CGO**: not required — the project uses `modernc.org/sqlite`, a pure-Go driver. This is what allows cross-compilation and lets tests open a real database with no system dependency.
- **Environment Variables**: none
- **System Requirements**: any OS Go supports (Linux, macOS, Windows); no database server, no network access

## Build Steps

### 1. Install Dependencies

```bash
go mod tidy
```

### 2. Configure Environment

No configuration is required to build. At **runtime** the binary reads `config.json` beside itself, creating a default one on first run.

### 3. Build

```bash
make build          # single binary for the host platform
make build-all      # Linux, macOS, Windows
```

Equivalent direct invocation:

```bash
go build -o privateledger ./cmd/privateledger
```

### 4. Verify Build Success

- **Expected output**: no compiler output; exit code 0
- **Artifact**: `./privateledger` (~33 MB — the pure-Go SQLite driver and embedded web assets account for the size)
- **Embedded assets**: HTML templates and static files are compiled in via `go:embed`. A template edit therefore requires a rebuild to take effect.
- **Acceptable warnings**: none expected; `go vet ./...` should also be silent

## Troubleshooting

### Build fails with dependency errors
- **Cause**: stale or incomplete module cache
- **Fix**: `go clean -modcache && go mod tidy`

### Build fails with compilation errors
- **Cause**: usually a type mismatch after editing `internal/model` or `internal/repository`, since `service` and `handler` depend on both
- **Fix**: build the packages bottom-up to find the origin — `go build ./internal/model/ ./internal/repository/ ./internal/service/ ./internal/handler/`

### Template change appears to have no effect
- **Cause**: templates are embedded at compile time
- **Fix**: rebuild. Note that a malformed template is **not** a build error — it panics at first render, because `parseTemplate` uses `template.Must`. See `unit-test-instructions.md` for how to catch this before shipping.
