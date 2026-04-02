# Stage 1: Build the Go binary
FROM golang:1.26.1-alpine AS builder

WORKDIR /app

# Copy go.mod and install dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the source code
COPY *.go ./

# Build the application
# Use -ldflags to strip debug info and reduce memory footprint
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bot .

# Stage 2: Create a minimal runtime environment
FROM alpine:3.23.3

# Install CA certificates (needed for MySQL & HTTPS requests) and create app user
RUN apk --no-cache add ca-certificates tzdata && adduser -D -u 1001 botuser

WORKDIR /app

# Copy the compiled Go binary from the builder stage
COPY --from=builder /app/bot .

COPY *.json ./

USER botuser

# Run the bot
ENTRYPOINT ["/app/bot"]
