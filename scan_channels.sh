#!/usr/bin/env bash
# Scans a channel range on the Hikvision device and reports which ones the
# configured account can actually stream from.
#
# Usage: ./scan_channels.sh [FROM] [TO]
#   default: 1..16  (analog/TurboHD channels)
#   NVR IP channels usually start at 33: ./scan_channels.sh 33 50
#
# Reads credentials from .env. Uses the already-built bridge image; no
# containers/ports are published, scans run on the default docker network.

set -euo pipefail
cd "$(dirname "$0")"

FROM="${1:-1}"
TO="${2:-16}"
IMAGE="hikkconnect-go-bridge:latest"
STREAM="${HIK_STREAM:-main}"

set -a; source .env; set +a
: "${HIK_HOST:?HIK_HOST not set - copy .env.example to .env}"
: "${HIK_USER:?}" "${HIK_PASS:?}"

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
	echo "[scan] image $IMAGE not found, building..."
	docker compose build bridge
fi

printf "[scan] %s@%s channels %d-%d (%s stream)\n\n" \
	"$HIK_USER" "$HIK_HOST" "$FROM" "$TO" "$STREAM"
printf "%-8s %-10s %s\n" "channel" "result" "detail"

for ch in $(seq "$FROM" "$TO"); do
	name="chscan-$$"
	docker rm -f "$name" >/dev/null 2>&1 || true
	docker run -d --name "$name" --platform linux/amd64 \
		-e HIK_HOST="$HIK_HOST" -e HIK_PORT="${HIK_PORT:-8000}" \
		-e HIK_USER="$HIK_USER" -e HIK_PASS="$HIK_PASS" \
		-e HIK_CHANNEL="$ch" -e HIK_STREAM="$STREAM" -e HIK_TLS="${HIK_TLS:-}" \
		"$IMAGE" >/dev/null

	# login (~2-4s under amd64 emulation) + realplay attempt
	sleep 6
	err=$(docker logs "$name" 2>&1 | grep -oE "realplay: .*" | tail -1 || true)

	if [ -n "$err" ]; then
		printf "%-8s %-10s %s\n" "$ch" "denied" "$err"
	else
		# session opened - probe whether frames actually flow (some
		# channels open but carry no video, e.g. empty slots)
		bytes=$(docker run --rm --platform linux/amd64 --network "container:$name" busybox \
			sh -c 'timeout 5 nc 127.0.0.1 5000 | wc -c' 2>/dev/null || true)
		bytes=${bytes//[^0-9]/}
		if [ "${bytes:-0}" -gt 1000 ]; then
			printf "%-8s %-10s %s\n" "$ch" "OK" "${bytes}B received - video flowing"
		else
			printf "%-8s %-10s %s\n" "$ch" "empty" "session opened but no frames"
		fi
	fi
	docker rm -f "$name" >/dev/null 2>&1 || true
done

echo
echo "[scan] done. Set HIK_CHANNEL=<an OK channel> in .env"
