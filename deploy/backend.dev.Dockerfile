FROM golang:1.26-alpine
RUN apk add --no-cache gcc g++ musl-dev ffmpeg ca-certificates
WORKDIR /src
ENV CGO_ENABLED=1 GOFLAGS=-tags=nosqlite DATA_DIR=/app/data CACHE_DIR=/app/cache
# Run the compiled server directly so it receives Compose's shutdown signal.
# gazes-library (admin CLI) is built alongside so `docker compose exec backend gazes-library ...` works in dev.
CMD ["sh", "-c", "go build -o /usr/local/bin/gazes-library ./cmd/library && go build -o /tmp/gazes-server ./cmd/server && exec /tmp/gazes-server"]
