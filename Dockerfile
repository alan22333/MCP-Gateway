# MCP Gateway — production Docker image
# Multi-stage build with CGO for SQLite support

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git gcc musl-dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /gateway ./cmd/server/

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates
COPY --from=builder /gateway /gateway
COPY config.yaml /config.yaml
COPY web/ /web/

EXPOSE 8080
ENTRYPOINT ["/gateway"]
