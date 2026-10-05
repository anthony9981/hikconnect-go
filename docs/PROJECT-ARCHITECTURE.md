# Project Architecture

## System context

```
                        ┌──────────────────────────────────────────┐
                        │ Docker network: camnet                   │
┌──────────────┐        │                                          │
│ Hikvision    │ :8000  │  ┌────────────────┐   ┌───────────────┐  │
│ DVR / camera │◀──────▶│  │ bridge (Go)    │──▶│ go2rtc        │  │
└──────────────┘ HCNetSDK│  │ hikvision-bridge│  │ alexxit/1.9.14│  │
   sakurakids... :8000   │  └────────────────┘   └───────┬───────┘  │
                        │            TCP :5000           │          │
                        └────────────────────────────────┼──────────┘
                                                         │
            ┌───────────────────────────┬────────────────┼───────────┐
            ▼                           ▼                ▼           ▼
       RTSP :8554                  WebRTC :8555      HTTP :1984   ffmpeg/jpeg
       (VLC, Frigate, HA)          (browser live)    (UI/API/HLS)  snapshots
```

Data plane: camera → HCNetSDK RealPlay → Go frames channel → TCP `:5000` →
ffmpeg (`-f mpegps` demux, `-c:v copy`) → go2rtc → consumers.
Control plane: env config only; bridge holds one login + one preview session.

## Bridge internals (`main.go`)

```
main
 └─ loadConfig            env → config (required: HIK_HOST/USER/PASS)
 └─ hikvision.Login       first login triggers NET_DVR_Init; dev.Info gives
                          channel layout (analog N @ startCh, ip N @ 33+)
 └─ channel auto-pick     HIK_CHANNEL=0 → StartChannel or StartIPChannel
 └─ net.Listen :5000
 └─ openStream()          ONE dev.RealPlay(ctx) for process lifetime
 └─ accept loop
      ├─ reopen stream if frames chan dead (SDK-side close)
      ├─ drainStale()      drop buffered frames → client starts at live edge
      └─ serve(conn)       for frame := range frames → write type 2/4 payloads
                           5s write deadline; 3s type-stats logger
```

Key structures: `config` (env-derived), `openStream` closure (owns RealPlay
handle via frames chan), `serve` (single consumer at a time).

## Design decisions (and why)

| Decision | Reason |
|---|---|
| Persistent RealPlay, never per-client stop | `NET_DVR_StopRealPlay` SIGBUS under amd64 emulation |
| Pass frame types 2 + 4 | TurboHD sends PS-muxed `StreamData`; type-4-only filtering = silent stream |
| TCP socket, not files/stdout | go2rtc producer isolation + reconnect without container restart |
| `exec:ffmpeg` producer in go2rtc | `ffmpeg:` source ignores `#input=` for `tcp://` URLs; exec gives full control (`-f mpegps`, nobuffer, small probesize) |
| `webrtc.candidates: 127.0.0.1` | container IP unreachable from host browser |
| Docker-only build | HCNetSDK is amd64-linux proprietary; SDK vendored into module in-image |
| Single client at a time | Simplest correct model; go2rtc is the only consumer |

## Lifecycle & failure modes

- **Startup**: login → realplay opens → listen. If login fails → fatal
  (restart policy retries).
- **Client disconnect**: write error → conn closed, stream persists, loop
  re-accepts. No SDK churn.
- **Frames channel closes** (SDK/network drop): `serve` returns `chanDead` →
  `openStream()` re-runs on next client.
- **Login session lost**: not currently handled — SDK has `OnException`
  callback (`hikvision.OnException`) for reconnect logic (roadmap).
- **SIGBUS on teardown**: if it happens at shutdown, container restart covers
  it; no state to corrupt.

## Ports

| Port | Owner | Purpose |
|---|---|---|
| 5000 | bridge (internal only) | raw stream to go2rtc |
| 8554 | go2rtc | RTSP |
| 8555 tcp+udp | go2rtc | WebRTC |
| 1984 | go2rtc | API/UI/HLS/MJPEG/snapshot |

## Trust boundaries / secrets

- `.env` → compose env → container env. No creds in code or images.
- `sdk/` injected at build only; runtime image contains the `.so` files
  (required) but the repo never redistributes them.
- go2rtc API/UI unauthenticated — bind to localhost only unless hardened.
