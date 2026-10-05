# HCNetSDK field notes

Everything here was learned empirically against a `DS-7116HGHI-F1/N` TurboHD
DVR over the SDK private protocol on port 8000. The wrapper is
`github.com/elyor04/go-hikvision-sdk/hikvision` v1.2.3.

## Wrapper API used (not the sketch you'll see elsewhere)

```go
dev, err := hikvision.Login(hikvision.LoginOptions{
    Address string; Port uint16; Username, Password string; UseTLS bool
})
// first Login triggers NET_DVR_Init; there is no exported Init/Cleanup
defer dev.Close()          // not .Logout()

dev.Info                   // DeviceInfo captured at login
dev.RealPlay(ctx, channel int32, stream StreamType) (*Stream, error)
stream.Frames() <-chan Frame{Type StreamDataType, Data []byte}
stream.Close()
```

`DeviceInfo` fields available: `SerialNumber`, `AnalogChannels`,
`StartChannel`, `IPChannels`, `StartIPChannel`, `DeviceType`...

## Frame data types (Frame.Type)

| Value | Const | Meaning |
|---|---|---|
| 1 | `StreamSysHead` | 40-byte private system header (sent once) |
| 2 | `StreamData` | **private packed stream — MPEG-PS mux on TurboHD** |
| 3 | `StreamAudioData` | raw audio |
| 4 | `StreamStdVideoData` | elementary H.264/H.265/MJPEG |
| 5 | `StreamStdAudioData` | elementary audio |

**Gotcha:** TurboHD DVRs deliver type 2, never type 4. The payload is a
standard MPEG-PS stream: pack header `00 00 01 BA`, system header
`00 00 01 BC`, video PES `00 00 01 E0` containing annexb H.264
(`00 00 01 67` = SPS). ffmpeg demuxes with `-f mpegps`, or autodetects.
Filter frames to types 2+4 only — syshead/audio will corrupt the ES.

## Error codes seen (NET_DVR_GetLastError)

| Code | Message | Meaning here |
|---|---|---|
| 2 | "Don't have enough authority" | account lacks live-view right on that channel |
| 4 | "Channel number error" | channel index doesn't exist on device |
| 159 | "SSL connect failed" | device has no TLS transport — don't set `UseTLS` |

## Channel numbering

- TurboHD/analog channels: `StartChannel` = 1, count = `AnalogChannels`
- IP cameras on DVR/NVR: `StartIPChannel` (usually 33), count = `IPChannels`
- On the test DVR `ipCh=18`/`startIPCh=33` was *reported* but ch33+ returned
  code 4 — the IP slots existed but were unconfigured. Reported capability ≠
  usable channel; probe for real.
- A channel can open (`realplay started`) yet deliver zero frames — empty
  slot or offline camera. `scan_channels.sh` distinguishes this by measuring
  bytes over the TCP socket.
- Per-channel user rights are real: a sub-account may open only specific
  channels and get code 2 on the rest.

## Emulation quirk (Apple Silicon)

`NET_DVR_StopRealPlay` (and possibly other teardown paths) SIGBUS under QEMU:
`signal arrived during cgo execution`. The bridge therefore opens **one**
RealPlay session for process lifetime and never calls `stream.Close()` except
at shutdown. Do not reintroduce per-client start/stop.

## Vendoring mechanics

The wrapper needs, inside its own module dir:

```
internal/sdklib/linux_amd64/include/   <- SDK incEn/
internal/sdklib/linux_amd64/lib/       <- SDK lib/ (incl. HCNetSDKCom/)
```

Produced by `scripts/vendor-sdk.sh` in the wrapper repo; the Dockerfile runs
it. rpath is baked to `<module dir>/internal/sdklib/linux_amd64/lib`, so the
runtime image must recreate `/hiksdk/internal/sdklib` verbatim.

## Other traps

- Port 80/ISAPI may simply be closed — absence of HTTP API proves nothing.
- `Kitty2`-style sub-accounts created in Hik-Connect still log in fine over
  SDK; permissions are channel-scoped though.
- SDK prints `loop[2] find N mac and M ip` on init — normal noise, not an error.
