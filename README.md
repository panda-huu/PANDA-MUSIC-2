# GOMUSICV2

Voice chats use **gogram + ntgcalls**. Start / help / player / queue panels use blockquote captions and coloured buttons. Queues stay in memory — no database.

| thing | version |
| --- | --- |
| go | 1.26.0+ |
| telegram library | gogram |
| voice calls | ntgcalls |
| player | ffmpeg + yt-dlp |

## One command setup

On a fresh Ubuntu/Debian VPS as root:

```bash
cd /root/GOMUSICV2
cp sample.env .env
nano .env
bash setup.sh
```

`setup.sh` installs packages, Go, yt-dlp, ntgcalls, builds `gomusic`, stops any old process, and starts the bot.

Fill `.env` first:

- `API_ID`
- `API_HASH`
- `BOT_TOKEN`
- `STRING_SESSION`
- `OWNER_ID`

Logs:

```bash
tail -f /root/gomusic.log
```

## Manual commands (if you do not use setup.sh)

```bash
apt-get update
apt-get install -y build-essential ffmpeg fonts-dejavu curl git python3

curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp
chmod +x /usr/local/bin/yt-dlp

cd /tmp
curl -fsSLO https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
export PATH=/usr/local/go/bin:$PATH

cd /root/GOMUSICV2
cp -n sample.env .env
export CGO_ENABLED=1
go run setup_ntgcalls.go
go build -o gomusic ./GOMUSIC
pkill -f './gomusic' || true
nohup ./gomusic > /root/gomusic.log 2>&1 &
```

## Update later

```bash
cd /root/GOMUSICV2
git pull
bash setup.sh
```

