# Build stage
FROM golang:1.25-alpine AS builder

# Set up build arguments
ARG VERSION=dev
ARG COMMIT=unknown

# Set working directory
WORKDIR /app

# Install necessary packages for building
RUN apk --no-cache add git ca-certificates wget gcompat

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code including .git for submodule access
COPY . .

# Download third_party dependencies directly instead of using git submodules
# This is more reliable in cloud build environments
RUN mkdir -p third_party/iam-dataset/aws third_party/iam-dataset/azure third_party/iam-dataset/gcp && \
    wget -O third_party/iam-dataset/aws/docs.json https://raw.githubusercontent.com/iann0036/iam-dataset/main/aws/docs.json && \
    wget -O third_party/iam-dataset/aws/managed_policies.json https://raw.githubusercontent.com/iann0036/iam-dataset/main/aws/managed_policies.json && \
    wget -O third_party/iam-dataset/azure/built-in-roles.json https://raw.githubusercontent.com/iann0036/iam-dataset/main/azure/built-in-roles.json && \
    wget -O third_party/iam-dataset/azure/provider-operations.json https://raw.githubusercontent.com/iann0036/iam-dataset/main/azure/provider-operations.json && \
    wget -O third_party/iam-dataset/gcp/role_permissions.json https://raw.githubusercontent.com/iann0036/iam-dataset/main/gcp/role_permissions.json && \
    wget -O third_party/iam-dataset/gcp/predefined_roles.json https://raw.githubusercontent.com/iann0036/iam-dataset/main/gcp/predefined_roles.json

# Verify the required files are present
RUN ls -la third_party/iam-dataset/aws/ && ls -la third_party/iam-dataset/azure/ && ls -la third_party/iam-dataset/gcp/

# Build the application
RUN GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X github.com/thand-io/agent/internal/common.Version=${VERSION} -X github.com/thand-io/agent/internal/common.GitCommit=${COMMIT}" \
    -o bin/agent .

# Final stage
FROM alpine:latest

LABEL org.opencontainers.image.source=https://github.com/thand-io/agent
LABEL org.opencontainers.image.description="Thand Agent - Open-source agent for AI-ready privilege access management (PAM) and just-in-time access (JIT) to cloud infrastructure, SaaS applications and local systems."
LABEL org.opencontainers.image.licenses=BSL-1.1

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates gcompat

# Create a non-root user
RUN addgroup -S agent && adduser -S agent -G agent

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/bin/agent ./agent

# Copy configuration example file if it exists
COPY config.example.yaml ./

# Change ownership to non-root user
RUN chown -R agent:agent /app

# Switch to non-root user
USER agent

# Expose the default port (adjust if your server uses a different port)
EXPOSE 8080

# Health check (adjust the endpoint as needed)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENV THAND_SERVER_PORT=8080

# Default command is to run the server
CMD ["./agent", "server"]
