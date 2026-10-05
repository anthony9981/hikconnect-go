# syntax=docker/dockerfile:1

# Build needs: go-hikvision-sdk source + official HCNetSDK Linux64 vendored
# into it (headers for cgo compile, .so for link). HCNetSDK is proprietary,
# supply it yourself: put the extracted Linux64 SDK folder (containing
# incEn/ and lib/) at ./sdk/ next to this Dockerfile.

FROM golang:1.25-bookworm AS builder
ARG TARGETPLATFORM=linux/amd64

RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 --branch v1.2.3 \
    https://github.com/elyor04/go-hikvision-sdk /hiksdk

# Vendor HCNetSDK into the module: internal/sdklib/linux_amd64/{include,lib}
COPY sdk/ /sdk/
RUN bash /hiksdk/scripts/vendor-sdk.sh "" /sdk

WORKDIR /app
COPY go.mod main.go ./
RUN go mod tidy \
    && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o hikvision-bridge .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/hikvision-bridge .

# cgo bakes rpath -> /hiksdk/internal/sdklib/linux_amd64/lib (path where the
# module lived at build time). Recreate the same path in the runtime image.
COPY --from=builder /hiksdk/internal/sdklib /hiksdk/internal/sdklib
ENV LD_LIBRARY_PATH=/hiksdk/internal/sdklib/linux_amd64/lib

EXPOSE 5000
CMD ["./hikvision-bridge"]
