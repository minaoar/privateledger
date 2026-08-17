# Build Instructions

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
