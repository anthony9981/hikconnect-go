# Coding Standards

Applies to this repo. Small codebase — keep it boring and readable.

## Go

- Standard library first. New dependency needs a reason; check `go.mod`
  before importing anything new.
- `gofmt` clean always. `golint`-style naming: camelCase locals, PascalCase
  exports, no stutter (`bridge.Stream`, not `bridge.BridgeStream`).
- Early returns over nesting. Guard clauses at function top.
- Errors: wrap with context (`fmt.Errorf("login: %w", err)`), fatal at
  `main`/top level only via `log.Fatalf` — no panics for flow control.
- cgo boundary: keep it inside `hikvision` package calls. Never call the SDK
  concurrently for the same session object without reason — HCNetSDK is
  thread-hostile under emulation.
- Logging: `log.Printf("[bridge] ...")` prefix style, no log frameworks,
  state transitions + counters only. No creds/hosts-with-secrets in logs.
- No goroutine leaks: every `go func` needs an exit path (ctx/done chan).

## Shell (`scan_channels.sh` etc.)

- `set -euo pipefail`, `#!/usr/bin/env bash`.
- Quote every expansion. Env vars loaded via `set -a; source .env; set +a`.
- `docker run` cleanup: `--rm` or explicit `docker rm -f` on all paths.
- Always `--platform linux/amd64` on `docker run` for the bridge image —
  silences host-platform warnings and prevents accidental arm64 pulls.

## Docker / compose

- Pin image tags (`alexxit/go2rtc:1.9.14`, not `latest`).
- `platform: linux/amd64` on every service — HCNetSDK has no arm64 build.
- Secrets via `.env` + `${VAR:?msg}` — never literal values in compose.
- No published ports on `bridge` — internal network only.
- Build context stays small: `.dockerignore` must keep excluding `.git`,
  docs, local files. `sdk/` is the only large thing and it's required.

## Files & naming

- kebab-case files (`scan_channels.sh` predates this — grandfathered).
- Docs UPPERCASE (`docs/HCNETSDK.md`), code lowercase.
- Comments: explain *why*, not *what*. Non-obvious SDK/emulation behavior
  gets a comment; obvious lines don't.

## Don't

- Don't add frameworks (zap, viper, cobra...) for a single-binary tool.
- Don't mock HCNetSDK — it can't be faked meaningfully; verify on hardware.
- Don't "fix" frame-type filtering or per-client RealPlay — see invariants
  in `AGENTS.md`.
