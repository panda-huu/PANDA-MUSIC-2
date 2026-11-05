#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

export PATH=/usr/local/go/bin:$PATH
export CGO_ENABLED=1

echo "==> GOMUSICV2 setup"

if [ "$(id -u)" -eq 0 ]; then
  SUDO=""
else
  SUDO="sudo"
fi

echo "==> packages"
$SUDO apt update -y
$SUDO apt install -y build-essential gcc g++ make ffmpeg fonts-dejavu-core fonts-dejavu curl git python3 wget unzip ca-certificates

if ! command -v go >/dev/null 2>&1; then
  echo "==> installing Go 1.26.0"
  cd /tmp
  curl -fsSLO https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
  $SUDO rm -rf /usr/local/go
  $SUDO tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
  cd "$ROOT"
fi
export PATH=/usr/local/go/bin:$PATH
go version

echo "==> yt-dlp (YouTube search)"
if ! command -v yt-dlp >/dev/null 2>&1; then
  python3 -m pip install --break-system-packages --upgrade yt-dlp
fi
yt-dlp --version

echo "==> stop old bot (same session cannot run on two IPs)"
pkill -f './gomusic' 2>/dev/null || true
pkill -f '/gomusic' 2>/dev/null || true
sleep 1

echo "==> ntgcalls"
go run setup_ntgcalls.go

echo "==> modules (TypeTube Go client)"
go get github.com/playeon/typetube-go@latest
go mod tidy

echo "==> build"
go build -o gomusic ./GOMUSIC

echo "==> start (foreground)"
exec ./gomusic
