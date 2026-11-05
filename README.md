# GOMUSICV2

Telegram voice-chat music bot built with **GoGram + ntgcalls**. Queue and player state stay in memory; no MongoDB required.

| Component | Technology |
| --- | --- |
| Runtime | Go 1.26+ |
| Telegram | gogram |
| Voice chat | ntgcalls |
| Search / stream resolution / downloads | TypeTube Go client |
| Media processing | FFmpeg |

## Setup

On Ubuntu/Debian VPS:

```bash
cd /root/GOMUSICV2
cp sample.env .env
nano .env
bash setup.sh
```

The setup script installs system packages, Go, ntgcalls and the TypeTube Go module, builds `gomusic`, then starts the bot. **yt-dlp is used only for YouTube search; TypeTube resolves and downloads media using the selected YouTube video ID.**

Required `.env` values:

- `API_ID`
- `API_HASH`
- `BOT_TOKEN`
- `STRING_SESSION`
- `OWNER_ID`

TypeTube's public Singapore cluster is used by default without an API key. Some tracks may be region-restricted; the TypeTube client reports those download errors.

Logs:

```bash
tail -f /root/gomusic.log
```

## Manual build

```bash
apt-get update
apt-get install -y build-essential ffmpeg fonts-dejavu curl git ca-certificates
cd /root/GOMUSICV2
export PATH=/usr/local/go/bin:$PATH
export CGO_ENABLED=1
go run setup_ntgcalls.go
go get github.com/playeon/typetube-go@latest
go mod tidy
go build -o gomusic ./GOMUSIC
pkill -f './gomusic' || true
nohup ./gomusic > /root/gomusic.log 2>&1 &
```

## Update

```bash
cd /root/GOMUSICV2
git pull
bash setup.sh
```
