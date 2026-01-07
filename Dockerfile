# --- Stage 1: Build the Go binary ---
FROM golang:1.25-alpine AS builder

# Install git and CA certs (if needed for Go modules)
RUN apk add --no-cache git ca-certificates

# Set working directory inside the container
WORKDIR /app

# Copy Go source code into the container
COPY . .

# Enable Go Modules (optional if using go.mod)
ENV GO111MODULE=on

# Download deps
RUN go mod download

# Build the Go app for Linux
RUN go build -o omada-exporter

# --- Stage 2: Create a lightweight image with the binary only ---
FROM alpine:3.14

# Add CA certs for HTTPS support
RUN apk add --no-cache ca-certificates

# Copy binary from builder stage
COPY --from=builder /app/omada-exporter /usr/bin/omada-exporter

# Set executable permissions (just in case)
RUN chmod +x /usr/bin/omada-exporter

# Command to run
ENTRYPOINT ["/usr/bin/omada-exporter"]