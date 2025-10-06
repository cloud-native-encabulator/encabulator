# Build stage
FROM golang:1.24-alpine AS builder

# Install git for go mod download with direct proxy
RUN apk add --no-cache git

WORKDIR /app

# Use direct downloads to avoid proxy issues
ENV GOPROXY=direct

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o hydrocoptic-datastore ./cmd/server

# Final stage - using scratch for minimal image size
FROM scratch

# Copy CA certificates from builder for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/hydrocoptic-datastore /hydrocoptic-datastore

# Expose port
EXPOSE 8080

# Run the application
ENTRYPOINT ["/hydrocoptic-datastore"]
