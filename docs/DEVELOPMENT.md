# Development

## Build pipeline (why the Dockerfile looks odd)

`go-hikvision-sdk` cannot be used via plain `go get`: its cgo directives
(`hikvision/cgo.go`) point at `internal/sdklib/<os>_<arch>/{include,lib}`
inside the **module itself**, and that directory is gitignored — every consumer
must vendor HCNetSDK there before compiling. `go mod vendor` does not help
(non-Go files are skipped).

So the Dockerfile:

1. clones `go-hikvision-sdk` at tag `v1.2.3` to `/hiksdk`
2. copies your extracted SDK (`./sdk` = Linux64 root with `incEn/` + `lib/`)
   to `/sdk` and runs `/hiksdk/scripts/vendor-sdk.sh "" /sdk` — copies headers
   + shared libs into `/hiksdk/internal/sdklib/linux_amd64/`
3. builds with `replace github.com/elyor04/go-hikvision-sdk => /hiksdk`
   (see `go.mod`)
4. runtime stage recreates `/hiksdk/internal/sdklib` at the **same absolute
   path** — cgo bakes `-Wl,-rpath,<SRCDIR>/../internal/sdklib/linux_amd64/lib`
   into the binary at link time, so the path must survive into the runtime
   image. `LD_LIBRARY_PATH` is set to the same dir as belt-and-braces.

Note: `vendor-sdk.sh` is not executable in the clone — invoke via `bash`.

## Apple Silicon / amd64 emulation

- Everything pins `platform: linux/amd64`. Under QEMU/Rosetta the Go+cgo
  build takes ~40s; image pulls take minutes the first time.
- HCNetSDK itself is x86-64 Linux only — do not try a native arm64 build.
- The SDK is not fully emulation-safe: `StopRealPlay`/session teardown can
  SIGBUS. The bridge works around this by never tearing down (see
  `main.go` comment and `docs/HCNETSDK.md`).

## Debugging

### Probe the TCP output directly

```bash
docker run --rm --platform linux/amd64 --network hikkconnect-go_camnet \
  busybox sh -c 'timeout 5 nc bridge 5000 | wc -c'
```

> 0 bytes = stream open but silent; timeout = nothing listening.

### Run a one-off bridge (different channel/stream without touching compose)

```bash
docker run -d --name chscan --platform linux/amd64 \
  --network hikkconnect-go_camnet \
  -e HIK_HOST=... -e HIK_USER=... -e HIK_PASS=... \
  -e HIK_CHANNEL=11 -e HIK_STREAM=main \
  hikkconnect-go-bridge
docker logs -f chscan
docker rm -f chscan
```

### Capture raw stream to a file for hex inspection

```bash
docker run --rm --platform linux/amd64 -v /tmp:/out \
  --network hikkconnect-go_camnet \
  busybox sh -c 'timeout 8 nc bridge 5000 > /out/cap.bin'
xxd -l 128 /tmp/cap.bin   # PS packs start with 00 00 01 BA
```

### Useful log lines emitted by bridge

- `logged in: ... serial=... analog=N startCh=X ipCh=N startIPCh=Y` — device
  layout discovered at login (channel auto-pick uses this)
- `realplay started` / `realplay: <err>` — session up or SDK error code
- `drained N stale frames` — backlog dropped for a fresh client
- `frames: types=map[...] sent=NB` — live frame-type stats every 3s
- `write: broken pipe ...` — normal client disconnect, NOT a crash

## Changing go2rtc source config

`go2rtc.yaml` uses `exec:ffmpeg ... -f mpegps -i tcp://bridge:5000 ...` as the
primary producer because `ffmpeg:` sources with a `tcp://` URL ignore the
`#input=` template param (falls through to plain `-i <url>` — see
`internal/ffmpeg/ffmpeg.go` upstream). The `ffmpeg:` entry is kept as a
fallback producer; ffmpeg autodetects PS fine, just slower.

After editing `go2rtc.yaml`: `docker compose restart go2rtc`.

If the browser WebRTC fails and the UI silently falls back to MJPEG: check
`webrtc.candidates` — inside Docker the advertised candidate must be a
host-reachable IP, not the container IP.

## Verification checklist after changes

1. `docker compose build bridge` — clean cgo build
2. `docker compose up -d` — bridge logs `logged in` + `realplay started`
3. `curl http://localhost:1984/api/frame.jpeg?src=kitty_cam` → JPEG bytes
   (slow ~5-7s under emulation = decode+encode, not stream latency)
4. `docker compose logs bridge` — `frames: types=map[...]` growing, no
   SIGBUS/panic
