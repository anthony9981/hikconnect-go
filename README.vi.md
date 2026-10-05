# hikvision-bridge

[![License: MIT](https://img.shields.io/github/license/anthony9981/hikconnect-go)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![Docker](https://img.shields.io/badge/Docker-linux%2Famd64-2496ED?logo=docker&logoColor=white)](Dockerfile)
[![go2rtc](https://img.shields.io/badge/go2rtc-1.9.14-orange)](go2rtc.yaml)
[![Platform](https://img.shields.io/badge/device-HCNetSDK%20port%208000-lightgrey)](docs/HCNETSDK.md)

[English](README.md) | **Tiếng Việt**

Stream camera/đầu ghi Hikvision vào **go2rtc** (RTSP, WebRTC, HLS,
Home Assistant) qua **HCNetSDK** — giao thức riêng trên cổng 8000. Không cần
ISAPI, không cần Hik-Connect cloud, không cần RTSP trên thiết bị.

## Lý do repo ra đời

Mình muốn gắn camera lớp học của con vào Home Assistant. Nhưng camera nằm sau
**đầu ghi TurboHD** (`DS-7116HGHI-F1/N`) chẳng mở gì cả: không RTSP, không
ONVIF, cổng HTTP/ISAPI đóng kín, và tài khoản duy nhất có được là **tài khoản
phụ Hik-Connect** mà nhà trường cấp cho phụ huynh.

Cửa duy nhất còn mở là giao thức SDK riêng trên **cổng 8000** — cũng chính là
đường mà app Hik-Connect dùng để đăng nhập. Vậy nên: HCNetSDK vào, go2rtc ra,
xong.

Phát hiện thú vị: đầu ghi TurboHD **không** trả về H.264 thuần khi RealPlay —
nó gửi **private-packed stream dạng MPEG-PS** (`StreamData` type 2). Bridge
forward nguyên stream và ffmpeg tự demux.

## Kiến trúc

```
┌──────────┐  HCNetSDK :8000   ┌─────────┐  TCP :5000   ┌────────┐
│ DVR/Cam  │ ────────────────▶ │ bridge  │ ───────────▶ │ go2rtc │
└──────────┘   stream PS mux   │  (Go)   │  PS stream   └────┬───┘
                             └─────────┘                    │
                rtsp://localhost:8554/kitty_cam  ◀──────────┤
                webrtc / hls / mjpeg @ :1984     ◀──────────┘
```

- `bridge` — ~150 dòng Go. Đăng nhập qua
  [go-hikvision-sdk](https://github.com/elyor04/go-hikvision-sdk) (cgo wrapper
  cho HCNetSDK), mở một phiên `RealPlay` bền, phục vụ raw stream qua TCP.
  Client rớt kết nối vẫn reconnect sạch.
- `go2rtc` — ffmpeg demux PS (`-f mpegps -c:v copy`), xuất RTSP / WebRTC /
  HLS / MJPEG / snapshot.

## Yêu cầu

- Docker (mọi nền tảng — build pin `linux/amd64`, Apple Silicon chạy qua
  emulation)
- **Hikvision Device Network SDK cho Linux64** — repo không kèm theo (bản quyền
  riêng, không được phân phối lại). Tải `EN-HCNetSDKV*_linux64` tại
  <https://www.hikvision.com/en/support/download/sdk/>
- Tài khoản thiết bị có quyền **Remote Live View** trên kênh cần xem
  (lỗi `code 2` = thiếu quyền; `code 4` = sai kênh)

## Bắt đầu nhanh

```bash
# 1. Giải nén SDK Linux64 vào ./sdk (cần có incEn/ và lib/)
mkdir sdk && cp -r /path/to/EN-HCNetSDKV*_linux64/{incEn,lib} sdk/

# 2. Cấu hình
cp .env.example .env && $EDITOR .env

# 3. Build image (bước này cũng cần cho việc quét kênh)
docker compose build bridge

# 4. Tìm kênh mà tài khoản được phép xem (không bắt buộc)
./scan_channels.sh          # kênh analog/TurboHD 1-16
./scan_channels.sh 33 50    # kênh IP trên NVR

# 5. Chạy
docker compose up -d
```

Xem:

| Client | URL |
|---|---|
| go2rtc UI | <http://localhost:1984> |
| WebRTC (độ trễ thấp nhất) | `http://localhost:1984/stream.html?src=kitty_cam` |
| VLC / RTSP | `rtsp://localhost:8554/kitty_cam` |
| HLS | `http://localhost:1984/api/stream.m3u8?src=kitty_cam` |
| Snapshot | `http://localhost:1984/api/frame.jpeg?src=kitty_cam` |

## Cấu hình

| Env | Mặc định | Ghi chú |
|---|---|---|
| `HIK_HOST` | — | IP hoặc hostname DDNS của thiết bị (**bắt buộc**) |
| `HIK_PORT` | `8000` | cổng giao thức SDK |
| `HIK_USER` | — | (**bắt buộc**) cần quyền live-view trên kênh |
| `HIK_PASS` | — | (**bắt buộc**) |
| `HIK_CHANNEL` | `0` | `0` = tự phát hiện từ `DeviceInfo` lúc login (kênh analog đầu, hoặc kênh IP đầu — thường 33 trên NVR) |
| `HIK_STREAM` | `main` | `main` \| `sub` \| `third` |
| `HIK_TLS` | off | `1`/`true` bật TLS (hiếm thiết bị hỗ trợ) |
| `LISTEN_ADDR` | `:5000` | socket TCP mà go2rtc kết nối tới |

## Tìm đúng kênh

`scan_channels.sh` đăng nhập bằng credential trong `.env` rồi thử từng kênh
trong dải. Kết quả:

```
channel  result     detail
1        denied     realplay: ... Don't have enough authority (code 2)
11       OK         756316B received - video flowing
33       denied     realplay: ... Channel number error (code 4)
```

- `OK` — stream được, đặt vào `HIK_CHANNEL`
- `empty` — phiên mở được nhưng không có frame (chưa gắn cam / kênh tắt)
- `denied code 2` — tài khoản thiếu quyền live-view
- `denied code 4` — kênh không tồn tại trên thiết bị

Lưu ý: quét khi stack đang chạy tốn thêm phiên đăng nhập — một số DVR giới hạn
số phiên đồng thời mỗi tài khoản. `docker compose stop bridge` trước nếu quét
lỗi bất thường.

## Troubleshooting

- **`RealPlay: Don't have enough authority (code 2)`** — tài khoản thiếu quyền
  xem trên kênh đó. Cấp quyền trong quản lý user của DVR, hoặc chạy
  `./scan_channels.sh` tìm kênh mở được.
- **`RealPlay: Channel number error (code 4)`** — kênh không tồn tại. Với
  `HIK_CHANNEL=0` bridge log cấu trúc kênh phát hiện được
  (`analog=N startCh=X ipCh=N startIPCh=Y`) khi khởi động — chọn theo đó.
- **Stream đen/rỗng trên kênh analog** — TurboHD trả về frame dạng PS mux
  (`StreamData`); repo đã xử lý. Nếu bạn fork bản cũ chỉ nhận `StdVideoData`,
  đó chính là bug.
- **SIGBUS/crash khi client ngắt trên Docker Desktop (Apple Silicon)** — quirk
  teardown của HCNetSDK dưới amd64 emulation. Bridge tránh bằng cách giữ một
  phiên RealPlay bền thay vì start/stop mỗi client.
- **WebRTC không kết nối trong Docker** — đặt `webrtc.candidates` trong
  `go2rtc.yaml` thành IP mà host truy cập được (xem file).

## Cấu trúc

```
main.go             bridge: login HCNetSDK -> RealPlay -> TCP :5000
Dockerfile          clone go-hikvision-sdk, vendor sdk/, build cgo
docker-compose.yml  bridge + go2rtc trên một network
go2rtc.yaml         exec/ffmpeg source -> rtsp/webrtc/hls
.env.example        mẫu cấu hình (copy thành .env)
scan_channels.sh    quét dải kênh tìm kênh stream được
sdk/                BẠN giải nén SDK Linux64 vào đây (gitignored)
```

## License

MIT (code trong repo). File `.so`/header HCNetSDK là tài sản của Hikvision —
tự tải về dùng, repo không phân phối lại.
