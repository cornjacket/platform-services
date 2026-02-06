# Task 004: Configurable Log Level and Format

**Status:** Complete
**Created:** 2026-02-06
**Updated:** 2026-02-06

## Context

Structured JSON logging with `slog` is already implemented throughout the codebase. However, the log level (INFO) and format (JSON) are hardcoded in `main.go`. This task adds configuration options for flexibility.

## Functionality

- Log level configurable via `CJ_LOG_LEVEL` environment variable
- Log format configurable via `CJ_LOG_FORMAT` environment variable
- Defaults remain INFO/JSON for production use
- Text format available for local development readability

## Design

### Configuration

Add to `config.go`:

| Variable | Default | Description |
|----------|---------|-------------|
| `CJ_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `CJ_LOG_FORMAT` | `json` | Output format (json, text) |

### Implementation

Update `cmd/platform/main.go` to read config before creating logger:

```go
// Parse log level
var level slog.Level
switch cfg.LogLevel {
case "debug":
    level = slog.LevelDebug
case "warn":
    level = slog.LevelWarn
case "error":
    level = slog.LevelError
default:
    level = slog.LevelInfo
}

// Create handler based on format
opts := &slog.HandlerOptions{Level: level}
var handler slog.Handler
if cfg.LogFormat == "text" {
    handler = slog.NewTextHandler(os.Stdout, opts)
} else {
    handler = slog.NewJSONHandler(os.Stdout, opts)
}

logger := slog.New(handler)
slog.SetDefault(logger)
```

**Note:** Config must be loaded before logger creation, which requires reordering `main.go` slightly. Log level parsing errors should use a temporary logger or stderr.

## Files to Modify

- `internal/shared/config/config.go` — Add LogLevel, LogFormat fields
- `cmd/platform/main.go` — Use config for logger initialization
- `platform-docs/design-spec.md` — Add variables to Section 12

## Acceptance Criteria

- [x] `CJ_LOG_LEVEL` controls log verbosity (debug, info, warn, error)
- [x] `CJ_LOG_FORMAT` controls output format (json, text)
- [x] Defaults are info/json (no change to current behavior)
- [x] `CJ_LOG_FORMAT=text` produces human-readable output for local dev
- [x] Documentation updated in design-spec.md Section 12

## Notes

- Service name in log context is nice-to-have but not required for Phase 1
- Trace ID injection deferred to Phase 2 with Traefik
