# Builder stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files FIRST (for better caching)
COPY go.mod go.sum* ./

# Download dependencies (cached layer)
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o convertly .

# Runtime stage - use smaller base image
FROM debian:bookworm-slim

# Install Pandoc and TeX Live in one layer with --no-install-recommends
# This reduces image size and build time
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    pandoc \
    texlive-latex-base \
    texlive-latex-extra \
    texlive-fonts-recommended \
    texlive-xetex \
    ca-certificates && \
    apt-get autoremove -y && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Copy binary from builder
COPY --from=builder /app/convertly .

# Copy static files LAST (changes most frequently)
COPY static/ ./static/

# Set environment variables
ENV PORT=8080

# Expose port
EXPOSE 8080

# Run the application
CMD ["./convertly"]
