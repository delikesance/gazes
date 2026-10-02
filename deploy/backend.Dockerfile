FROM golang:1.26-alpine AS build
RUN apk add --no-cache gcc g++ musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=1 go build -tags=nosqlite -buildvcs=false -trimpath -ldflags="-s -w" -o /gazes-server ./cmd/server && CGO_ENABLED=1 go build -tags=nosqlite -buildvcs=false -trimpath -ldflags="-s -w" -o /gazes-logs ./cmd/logs

FROM alpine:3.23
RUN apk add --no-cache ca-certificates ffmpeg libstdc++ && addgroup -S -g 10001 gazes && adduser -S -u 10001 -G gazes gazes
WORKDIR /app
RUN mkdir data cache diagnostics && chown -R gazes:gazes /app
COPY --from=build /gazes-server /app/gazes-server
COPY --from=build /gazes-logs /usr/local/bin/gazes-logs
USER gazes
ENV HOST=0.0.0.0 PORT=8090 DATA_DIR=/app/data CACHE_DIR=/app/cache LOG_DIR=/app/diagnostics
EXPOSE 8090
ENTRYPOINT ["/app/gazes-server"]
