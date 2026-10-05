# Development Roadmap

## Done

- [x] HCNetSDK login + RealPlay via go-hikvision-sdk (cgo, vendored SDK)
- [x] PS-muxed (`StreamData`) forwarding — TurboHD compatibility
- [x] Persistent single RealPlay session (avoids emulation SIGBUS)
- [x] Client reconnect + stale-frame drain
- [x] Channel auto-detect from `DeviceInfo`; `scan_channels.sh` tooling
- [x] go2rtc exec/ffmpeg low-latency producer → RTSP/WebRTC/HLS/snapshot
- [x] WebRTC host-reachable candidates for Docker
- [x] `.env` secrets, compose `${VAR:?}` guards, gitignored SDK

## Known limitations

- **Single client at a time** — accept loop serializes consumers. go2rtc is
  the only expected client, but a second TCP client blocks (by design).
- **No session-loss recovery** — if the device drops the login (network flap,
  DVR reboot), the frames channel dies but nothing re-logins; container
  restart is the only path. `hikvision.OnException` is the hook.
- **amd64 emulation only** — no arm64 HCNetSDK for Linux exists in this SDK
  drop; native ARM build impossible until Hikvision ships one.
- **No audio** — type 3/5 frames dropped. DVR mic audio (if any) discarded.
- **Snapshot latency ~6s** under QEMU (decode+JPEG encode), fine on native x86.

## Next up (priority order)

1. **Reconnect resilience** — `hikvision.OnException` → detect
   `ExceptionReLogin`/network errors → re-login + reopen stream in-process
   instead of relying on container restart.
2. **Multi-camera support** — map `HIK_CHANNEL` list → per-channel TCP ports
   or a small HTTP/SRT fan-out; go2rtc stream per channel. Refactor: accept
   loop → `map[chan]stream`.
3. **Multi-client broadcast** — fan frames to N TCP clients (ring buffer per
   client) instead of single-consumer serialization.
4. **Health endpoint** — tiny `:8080/healthz` reporting login/stream/frame
   state for compose healthcheck + restart-on-stall.
5. **CI build** — GitHub Actions: lint/build check gated on SDK vendoring
   problem (can't commit SDK → CI needs it as artifact/secret or skip cgo).
6. **Home Assistant card/recipes** — docs for webrtc-camera / Frigate intake.

## Ideas / icebox

- In-bridge PS demux (pure Go) → emit elementary H.264 → drop ffmpeg entirely,
  go2rtc consumes `h264` pipe directly. Removes emulation-heavy ffmpeg hop.
- PTZ / snapshot passthrough via `dev.PTZControl` / `dev.CaptureJPEG`
  (wrapper already exposes them).
- Playback API (`dev.Playback`/`FindRecordings`) → on-demand clip HTTP.
- Alarm/event subscription (`dev.Alarms`) → MQTT publish for HA motion events.
- Native `Stream`→go2rtc without TCP when on same host (shared socket/fifo).
