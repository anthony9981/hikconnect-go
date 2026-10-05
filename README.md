# hikvision-bridge

[![License: MIT](https://img.shields.io/github/license/anthony9981/hikconnect-go)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![Docker](https://img.shields.io/badge/Docker-linux%2Famd64-2496ED?logo=docker&logoColor=white)](Dockerfile)
[![go2rtc](https://img.shields.io/badge/go2rtc-1.9.14-orange)](go2rtc.example.yaml)
[![Platform](https://img.shields.io/badge/device-HCNetSDK%20port%208000-lightgrey)](docs/HCNETSDK.md)

**English** | [Tiếng Việt](README.vi.md)

Stream a Hikvision camera/DVR into **go2rtc** (RTSP, WebRTC, HLS, Home Assistant)
using the official **HCNetSDK** private protocol on port 8000 — no ISAPI, no
Hik-Connect cloud, no RTSP on the device required.

## Why this exists

I wanted my kid's kindergarten classroom camera in Home Assistant. The camera
lives behind a **TurboHD DVR** (`DS-7116HGHI-F1/N`) that exposes nothing
useful: no RTSP, no ONVIF, HTTP/ISAPI port closed, and the only credential I
had was a limited **Hik-Connect sub-account** (`Kitty2`) that the school
created for parents.

Turns out the only door still open is the SDK private protocol on **port 8000**,
which is what Hik-Connect itself logs in through. So: HCNetSDK in, go2rtc out,
done.

Bonus find along the way: TurboHD DVRs don't deliver elementary H.264 on
RealPlay — they send a **private-packed MPEG-PS stream** (`StreamData` type 2).
The bridge forwards it and ffmpeg demuxes it on the fly.

## Architecture

```
┌──────────┐  HCNetSDK :8000   ┌─────────┐  TCP :5000   ┌────────┐
│ DVR/Cam  │ ────────────────▶ │ bridge  │ ───────────▶ │ go2rtc │
└──────────┘   PS-muxed video  │  (Go)   │  PS stream   └────┬───┘
                             └─────────┘                    │
                rtsp://localhost:8554/kitty_cam  ◀──────────┤
                webrtc / hls / mjpeg @ :1984     ◀──────────┘
```

- `bridge` — ~150 lines of Go. Logs in via
  [go-hikvision-sdk](https://github.com/elyor04/go-hikvision-sdk) (cgo wrapper
  around HCNetSDK), starts one persistent `RealPlay` session, serves the raw
  stream on a TCP socket. Reconnects cleanly when clients drop.
- `go2rtc` — ffmpeg demuxes the PS stream (`-f mpegps -c:v copy`), exposes
  RTSP / WebRTC / HLS / MJPEG / snapshots.

## Requirements

- Docker (any platform — the build pins `linux/amd64` and runs under emulation
  on Apple Silicon)
- **Hikvision Device Network SDK for Linux64** — not vendored here (proprietary,
  non-redistributable). Download `EN-HCNetSDKV*_linux64` from
  <https://www.hikvision.com/en/support/download/sdk/>
- A device account with **Remote Live View** permission for the target channel
  (error `code 2` = missing permission; error `code 4` = wrong channel)

## Quickstart

```bash
# 1. Extract the Linux64 SDK into ./sdk (must contain incEn/ and lib/)
mkdir sdk && cp -r /path/to/EN-HCNetSDKV*_linux64/{incEn,lib} sdk/

# 2. Configure
cp .env.example .env && $EDITOR .env
cp go2rtc.example.yaml go2rtc.yaml   # adjust webrtc.candidates to your host IP

# 3. Build the bridge image (needed once for the channel scan too)
docker compose build bridge

# 4. Find which channels your account can stream (optional)
./scan_channels.sh          # analog/TurboHD channels 1-16
./scan_channels.sh 33 50    # NVR IP channels

# 5. Run
docker compose up -d
```

Watch:

| Client | URL |
|---|---|
| go2rtc UI | <http://localhost:1984> |
| WebRTC (lowest latency) | `http://localhost:1984/stream.html?src=kitty_cam` |
| VLC / RTSP | `rtsp://localhost:8554/kitty_cam` |
| HLS | `http://localhost:1984/api/stream.m3u8?src=kitty_cam` |
| Snapshot | `http://localhost:1984/api/frame.jpeg?src=kitty_cam` |

## Configuration

| Env | Default | Notes |
|---|---|---|
| `HIK_HOST` | — | device IP or DDNS hostname (**required**) |
| `HIK_PORT` | `8000` | SDK private protocol port |
| `HIK_USER` | — | (**required**) must have live-view right on the channel |
| `HIK_PASS` | — | (**required**) |
| `HIK_CHANNEL` | `0` | `0` = auto-detect from login `DeviceInfo` (analog start channel, or first IP channel e.g. 33 on NVRs) |
| `HIK_STREAM` | `main` | `main` \| `sub` \| `third` |
| `HIK_TLS` | off | `1`/`true` for TLS transport (rarely supported) |
| `LISTEN_ADDR` | `:5000` | bridge TCP socket consumed by go2rtc |

## Finding the right channel

`scan_channels.sh` logs in with your `.env` credentials and probes every
channel in a range. Output:

```
channel  result     detail
1        denied     realplay: ... Don't have enough authority (code 2)
11       OK         756316B received - video flowing
33       denied     realplay: ... Channel number error (code 4)
```

- `OK` — streamable, use it for `HIK_CHANNEL`
- `empty` — session opens but no frames (no camera attached or channel disabled)
- `denied code 2` — account lacks live-view permission
- `denied code 4` — channel doesn't exist on this device

Note: scanning while the stack is running uses extra logins — some DVRs cap
concurrent sessions per account. `docker compose stop bridge` first if scans
fail unexpectedly.

## Troubleshooting

- **`RealPlay: Don't have enough authority (code 2)`** — account lacks live-view
  permission on that channel. Grant it in DVR user management, or run
  `./scan_channels.sh` to find a channel the account can actually open.
- **`RealPlay: Channel number error (code 4)`** — channel doesn't exist. With
  `HIK_CHANNEL=0` the bridge logs the detected layout
  (`analog=N startCh=X ipCh=N startIPCh=Y`) at startup — pick from that.
- **Black/empty stream on analog channels** — TurboHD DVRs output PS-muxed
  frames (`StreamData`), which this project already handles. If you forked an
  older version that filtered for `StdVideoData` only, that's your bug.
- **SIGBUS/crash on client disconnect under Docker Desktop (Apple Silicon)** —
  an HCNetSDK teardown quirk under amd64 emulation. The bridge avoids it by
  keeping one persistent RealPlay session instead of start/stop per client.
- **WebRTC won't connect in Docker** — set `webrtc.candidates` in `go2rtc.yaml`
  to a host-reachable IP (see file).

## Files

```
main.go             bridge: HCNetSDK login -> RealPlay -> TCP :5000
Dockerfile          clones go-hikvision-sdk, vendors sdk/, cgo build
docker-compose.yml  bridge + go2rtc on one network
go2rtc.example.yaml go2rtc config template (copy to go2rtc.yaml, gitignored)
.env.example        config template (copy to .env)
scan_channels.sh    probe a channel range for streamable channels
sdk/                YOU extract the Linux64 SDK here (gitignored)
```

## License

MIT (this repo). HCNetSDK binaries/headers are proprietary Hikvision property —
supply your own copy; none are redistributed here.
