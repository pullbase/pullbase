FROM node:20-alpine AS web-builder

WORKDIR /app/web

# Copy web package files
COPY web/package*.json ./
RUN npm ci

# Copy web source and build
COPY web/ ./
RUN npm run build

# Go builder stage
FROM golang:1.24-alpine AS go-builder

WORKDIR /app

# Copy server go.mod and download dependencies
COPY server/go.mod server/go.sum server/
WORKDIR /app/server
RUN go mod download

# Copy server source
COPY server/ ./

# Copy built web UI files to embed directory
COPY --from=web-builder /app/web/dist /app/server/pkg/server/ui

# Build server with embedded UI
WORKDIR /app/server
RUN go build -o /app/pullbase-server ./cmd/server
RUN go build -o /app/pullbase-seed ./cmd/seed
RUN go build -o /app/pullbasectl ./cmd/pullbasectl

# Final runtime stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

# Copy binaries from builder
COPY --from=go-builder /app/pullbase-server .
COPY --from=go-builder /app/pullbase-seed .
COPY --from=go-builder /app/pullbasectl .

# Create necessary directories
RUN mkdir -p /app/certs /app/config /app/migrations /app/git-repos

# Copy configuration and migrations
COPY server/config/docker-config.json /app/config/config.json
COPY server/migrations/ /app/migrations/

EXPOSE 8080

CMD ["./pullbase-server"]
