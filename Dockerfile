############################
# STEP 1: Build executable binary
############################
FROM golang:1.24-alpine3.20 AS builder

# Install build dependencies
RUN apk update && apk add --no-cache gcc musl-dev git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY src/ ./src/

# Build the application
WORKDIR /app/src
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags="-w -s" -o /app/whatsapp .

############################
# STEP 2: Build runtime image
############################
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ffmpeg ca-certificates tzdata

# Create app user for security
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/whatsapp /app/whatsapp
COPY --from=builder /app/src/views /app/views

# Create necessary directories and set permissions
RUN mkdir -p statics/qrcode statics/senditems statics/media storages && \
    chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port (Railway will set this via $PORT)
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT:-3000}/ || exit 1

# Default command - Railway will override this with startCommand
CMD ["./whatsapp", "rest"]