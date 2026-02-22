# Builder stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o convertly .

# Runtime stage
FROM debian:bookworm-slim

# Install Pandoc and TeX Live for PDF generation
RUN apt-get update && \
    apt-get install -y \
    pandoc \
    texlive-latex-base \
    texlive-latex-extra \
    texlive-fonts-recommended \
    texlive-xetex \
    ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy binary from builder
COPY --from=builder /app/convertly .

# Copy static files
COPY static/ ./static/

# Set environment variables
ENV PORT=8080

# Expose port
EXPOSE 8080

# Run the application
CMD ["./convertly"]
