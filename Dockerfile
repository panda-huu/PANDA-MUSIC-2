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

# Downloads the matching Linux/amd64 ntgcalls 2.2.5 static archive and header.
RUN go run setup_ntgcalls.go

RUN CGO_ENABLED=1 go build -trimpath -ldflags="-w -s" -o /out/gomusic ./GOMUSIC

# Runtime image: keep FFmpeg available for audio/video streams.
FROM debian:bookworm-slim AS runtime
ENV PORT=10000
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates ffmpeg fonts-dejavu-core libstdc++6 zlib1g \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/gomusic /app/gomusic

EXPOSE 10000
CMD ["./gomusic"]
