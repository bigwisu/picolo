# Dockerfile

# --- Build Stage ---
# Use an official Go image as the build environment.
# Specify the Go version matching your go.mod file.
FROM golang:1.23-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./

# Download dependencies. This leverages Docker layer caching.
# Dependencies are downloaded only if go.mod or go.sum changes.
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the Go application
# -ldflags="-w -s" reduces the binary size by removing debug information.
# CGO_ENABLED=0 produces a statically linked binary (important for minimal base images like distroless or alpine)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /dialogflow-proxy .

# --- Runtime Stage ---
# Use a minimal base image for the runtime environment.
# distroless/static is very small and secure as it contains only the app and its dependencies.
# Alternatively, use alpine for a small image with a shell (useful for debugging).
# FROM gcr.io/distroless/static-debian11
FROM alpine:3.19

# Alpine needs libc, so ensure the build stage used CGO_ENABLED=0 for static linking if using distroless/static
# If using Alpine as the runtime, you might need some base packages depending on your app needs (e.g., ca-certificates)
RUN apk --no-cache add ca-certificates

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S appuser

# Set the working directory
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /dialogflow-proxy /app/dialogflow-proxy
RUN chown appuser:appgroup /app/dialogflow-proxy

# Switch to the non-root user
USER appuser

# Expose the port the application will listen on.
# This should match the PORT environment variable (default 8080 for Cloud Run).
EXPOSE 8080

# Set the entrypoint for the container.
# This command will run when the container starts.
ENTRYPOINT ["/app/dialogflow-proxy"]