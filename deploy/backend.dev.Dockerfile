FROM golang:1.26-alpine
RUN apk add --no-cache gcc g++ musl-dev ffmpeg ca-certificates
WORKDIR /src
ENV CGO_ENABLED=1 GOFLAGS=-tags=nosqlite DATA_DIR=/app/data CACHE_DIR=/app/cache
# Run the compiled server directly so it receives Compose's shutdown signal.
CMD ["sh", "-c", "go build -o /tmp/gazes-server ./cmd/server && exec /tmp/gazes-server"]
