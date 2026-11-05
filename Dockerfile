# Build GOMUSICV2 with the native ntgcalls static library.
FROM golang:1.26-bookworm AS builder

ENV CGO_ENABLED=1
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential gcc g++ make binutils ca-certificates curl unzip \
    zlib1g-dev libssl-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Install the official Linux/amd64 yt-dlp executable for YouTube search.
RUN mkdir -p /out && \
    curl -fL --retry 3 https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux -o /out/yt-dlp && \
    chmod 0755 /out/yt-dlp

# Downloads the matching Linux/amd64 ntgcalls 2.2.5 static archive and header.
RUN go run setup_ntgcalls.go

# Refresh checksums for all imported modules before compiling.
RUN go mod tidy
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-w -s" -o /out/gomusic ./GOMUSIC

FROM debian:bookworm-slim AS runtime
ENV PORT=10000
ENV PATH="/usr/local/bin:/usr/bin:/bin"
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates ffmpeg fonts-dejavu-core libstdc++6 zlib1g \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/gomusic /app/gomusic
COPY --from=builder /out/yt-dlp /usr/local/bin/yt-dlp
RUN chmod 0755 /usr/local/bin/yt-dlp && yt-dlp --version

EXPOSE 10000
CMD ["./gomusic"]
