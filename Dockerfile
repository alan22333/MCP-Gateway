# MCP Gateway — production Docker image
# Multi-stage: build in golang, run in distroless

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gateway ./cmd/server/

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /gateway /gateway
COPY config.yaml /config.yaml
COPY web/ /web/

EXPOSE 8080
ENTRYPOINT ["/gateway"]
