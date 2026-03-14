# Build stage
FROM golang:1.25.1-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/notif-api ./cmd/notificationCreator/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/notif-queueWorker ./cmd/queueWorker/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/notif-outbox-worker ./cmd/outbox-worker/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates postgresql-client

WORKDIR /root/

# Copy binaries from builder
COPY --from=builder /app/bin/notif-api .
COPY --from=builder /app/bin/notif-queueWorker .
COPY --from=builder /app/bin/notif-outbox-worker .

# Copy migrations, docs and entrypoint
COPY migrations /migrations
COPY docs /root/docs
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose API port
EXPOSE 8080

# Run selected application mode
ENTRYPOINT ["/entrypoint.sh"]
