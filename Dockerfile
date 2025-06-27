FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o loki-mcp-server ./cmd/server

# Use a smaller image for the final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/loki-mcp-server .

# Expose port for unified MCP server (both SSE and Streamable HTTP)
EXPOSE 8080

# Set the entry point
ENTRYPOINT ["./loki-mcp-server"]
