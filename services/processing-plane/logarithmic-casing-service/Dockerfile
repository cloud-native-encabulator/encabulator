# Build stage
FROM public.ecr.aws/docker/library/golang:1.25-alpine AS builder

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
    -o logarithmic-casing-service ./cmd/server

# Final stage - using alpine for minimal image with shell/utilities
FROM public.ecr.aws/docker/library/alpine:3.21

RUN apk --no-cache add ca-certificates

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/logarithmic-casing-service .

USER appuser

# Expose port
EXPOSE 8080

# Run the application
CMD ["./logarithmic-casing-service"]
