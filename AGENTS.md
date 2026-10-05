# AGENTS.md

## What this is

`hikvision-bridge` — Go binary that logs into a Hikvision DVR/camera over
HCNetSDK (private protocol, port 8000) via
[go-hikvision-sdk](https://github.com/elyor04/go-hikvision-sdk), pulls the
RealPlay stream, and serves it on TCP `:5000` for go2rtc, which demuxes and
re-publishes it as RTSP/WebRTC/HLS. Built for a TurboHD DVR that offers no
RTSP/ONVIF/ISAPI — only SDK login.

Stack: Go + cgo (C++ shim → HCNetSDK `.so`) → TCP → go2rtc (ffmpeg exec) →
RTSP `:8554` / WebRTC `:8555` / API `:1984`. Everything runs in Docker,
`linux/amd64` pinned (emulated via QEMU/Rosetta on Apple Silicon).

## Layout

```
main.go             bridge (login, persistent RealPlay, TCP fan-out)
Dockerfile          clones SDK wrapper, vendors sdk/, cgo build, runtime image
docker-compose.yml  bridge + go2rtc on camnet
go2rtc.yaml         exec/ffmpeg source -> rtsp/webrtc/hls
scan_channels.sh    probes channel range with .env creds
.env                local creds (gitignored; copy from .env.example)
sdk/                extracted HCNetSDK Linux64 (gitignored, YOU supply it)
docs/               development workflow + HCNetSDK field notes
```

## Commands

| Task | Command |
|---|---|
| build | `docker compose build bridge` (slow on arm64 — amd64 emulation) |
| run | `docker compose up -d` |
| logs | `docker compose logs -f bridge` |
| scan channels | `./scan_channels.sh [FROM] [TO]` |
| snapshot test | `curl -o /tmp/f.jpg "http://localhost:1984/api/frame.jpeg?src=kitty_cam"` |
| rtsp | `rtsp://localhost:8554/kitty_cam` |

There are no unit tests — verification is: build clean, login logs OK, probe
bytes over TCP, snapshot returns a JPEG.

## Non-negotiable invariants

- **Never stop/restart RealPlay per client.** HCNetSDK teardown SIGBUSes under
  amd64 emulation. One persistent session; clients attach/detach only.
- **Forward `StreamData` (type 2), not just `StdVideoData` (type 4).** TurboHD
  DVRs send PS-muxed private stream; filtering for type 4 yields zero bytes.
- **Credentials come from `.env` only.** No defaults in code or compose;
  compose uses `${VAR:?}` guards.
- **`sdk/` and `.env` must never be committed** (proprietary + secrets).
- Keep `platform: linux/amd64` — HCNetSDK ships amd64/arm-linux only; this
  repo vendors amd64.
- Commits: conventional, concise, **no attribution footers of any kind** —
  no `Co-Authored-By`, no `Generated with`, no agent/tooling credit. See
  [docs/COMMIT-RULES.md](docs/COMMIT-RULES.md).

## Details

- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) — build internals, debugging,
  manual probing, verification checklist
- [docs/HCNETSDK.md](docs/HCNETSDK.md) — SDK wrapper API used, frame types,
  error codes, channel numbering, vendoring mechanics
- [docs/PROJECT-ARCHITECTURE.md](docs/PROJECT-ARCHITECTURE.md) — components,
  data plane, design decisions, failure modes
- [docs/DEVELOPMENT-ROADMAP.md](docs/DEVELOPMENT-ROADMAP.md) — done list,
  limitations, next-up priorities
- [docs/CODING-STANDARDS.md](docs/CODING-STANDARDS.md) — Go/shell/docker
  rules for this repo
- [docs/COMMIT-RULES.md](docs/COMMIT-RULES.md) — message format, no
  attribution, flow
