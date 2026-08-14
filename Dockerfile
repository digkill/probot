# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/crawler ./cmd/crawler \
 && GOBIN=/out go install github.com/pressly/goose/v3/cmd/goose@v3.24.3

FROM alpine:3.21 AS runtime
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -H -u 65532 app
WORKDIR /app
COPY --from=builder /out/api /out/worker /out/crawler /app/
COPY --from=builder /out/goose /usr/local/bin/goose
COPY migrations /migrations
USER app
EXPOSE 8080

FROM runtime AS api
ENTRYPOINT ["/app/api"]

FROM runtime AS worker
ENTRYPOINT ["/app/worker"]

FROM runtime AS crawler
ENTRYPOINT ["/app/crawler"]

FROM alpine:3.21 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/goose /usr/local/bin/goose
COPY migrations /migrations
ENTRYPOINT ["sh", "-c", "goose -dir /migrations postgres \"$DATABASE_URL\" up"]
