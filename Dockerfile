# ---- Build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build the server binary (static, no CGO)
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ---- Run stage ----
FROM alpine:3.19

WORKDIR /app

# CA certs for outbound TLS (e.g. managed Postgres/Redis with SSL)
RUN apk add --no-cache ca-certificates

COPY --from=builder /app/server .

EXPOSE 8000

CMD ["./server"]
