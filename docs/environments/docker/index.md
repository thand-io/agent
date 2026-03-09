---
layout: default
title: Docker
parent: Environments
nav_order: 3
description: "Setup guide for deploying Thand Server on Docker"
has_children: true
---

# Docker Deployment
{: .no_toc }

Complete guide to deploying Thand Agent using Docker containers.
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

This guide walks you through deploying Thand Agent using Docker, including:

- Running the official Docker image
- Building custom images from source
- Configuring providers, roles, and workflows via volume mounts
- Production deployment considerations

---

## Quick Start

### 1. Pull and Run Official Image

```bash
# Pull the latest image
docker pull ghcr.io/thand-io/agent:latest

# Run with default configuration
docker run -d \
  --name thand-agent \
  -p 8080:8080 \
  ghcr.io/thand-io/agent:latest
```

### 2. Verify Deployment

```bash
# Check container status
docker ps | grep thand-agent

# Check logs
docker logs thand-agent

# Test health endpoint
curl http://localhost:8080/health
```

## Configuration Setup

### Directory Structure

Create a configuration directory structure for Docker volume mounting:

```bash
mkdir -p ./thand-config/{providers,roles,workflows}
```

### Configuration Files

For detailed configuration examples, see the documentation:

- **[Provider Configuration](../../configuration/providers/)** - Configure cloud providers, authentication, and integrations
- **[Role Configuration](../../configuration/roles/)** - Define access roles and permissions
- **[Workflow Configuration](../../configuration/workflows/)** - Set up approval workflows and automation

### Docker-Specific Configuration

Create the main configuration file optimized for Docker deployment:

```yaml
# ./thand-config/config.yaml
version: "1.0"

# Server Configuration
server:
  port: 8080
  host: "0.0.0.0"  # Bind to all interfaces for container access
  health:
    enabled: true
    path: "/health"
  ready:
    enabled: true
    path: "/ready"

# Logging Configuration
logging:
  level: "info"
  format: "json"
  output: "stdout"  # Container-friendly logging

# Configuration paths for volume mounts
providers:
  path: /app/config/providers/

roles:
  path: /app/config/roles/

workflows:
  path: /app/config/workflows/
```

## Deployment Options

### Option 1: Volume Mounts

Run with configuration mounted from host:

```bash
docker run -d \
  --name thand-agent \
  -p 8080:8080 \
  -v $(pwd)/thand-config:/app/config:ro \
  -e THAND_CONFIG_PATH=/app/config/config.yaml \
  ghcr.io/thand-io/agent:latest \
  ./thand server --config /app/config/config.yaml
```

### Option 2: Environment Variables

Run with environment-based configuration:

```bash
docker run -d \
  --name thand-agent \
  -p 8080:8080 \
  -e THAND_SERVER_PORT=8080 \
  -e THAND_LOGGING_LEVEL=info \
  -e THAND_PROVIDERS_PATH=/app/config/providers/ \
  -e THAND_ROLES_PATH=/app/config/roles/ \
  -e THAND_WORKFLOWS_PATH=/app/config/workflows/ \
  -v $(pwd)/thand-config:/app/config:ro \
  ghcr.io/thand-io/agent:latest
```

## Building from Source

### Build Custom Image

```bash
# Clone the repository
git clone https://github.com/thand-io/agent.git
cd agent

# Build the image
docker build -t thand-agent:custom .

# Run your custom image
docker run -d \
  --name thand-agent-custom \
  -p 8080:8080 \
  -v $(pwd)/config:/app/config:ro \
  thand-agent:custom
```

### Multi-stage Build Options

Build with specific version and commit:

```bash
docker build \
  --build-arg VERSION=v1.0.0 \
  --build-arg COMMIT=$(git rev-parse HEAD) \
  -t thand-agent:v1.0.0 .
```

## Production Deployment

### Production Configuration

Run with production settings and resource limits:

```bash
docker run -d \
  --name thand-agent-prod \
  -p 8080:8080 \
  --restart unless-stopped \
  --memory=2g \
  --cpus=1.0 \
  -v $(pwd)/config:/app/config:ro \
  -v $(pwd)/logs:/app/logs \
  -e THAND_CONFIG_PATH=/app/config/config.yaml \
  -e THAND_LOGGING_LEVEL=info \
  -e THAND_LOG_OUTPUT=/app/logs/agent.log \
  ghcr.io/thand-io/agent:latest
```

### Reverse Proxy Setup

Create an Nginx configuration for load balancing:

```nginx
# nginx.conf
upstream thand_backend {
    server localhost:8080;
}

server {
    listen 80;
    server_name your-domain.com;
    
    location / {
        return 301 https://$server_name$request_uri;
    }
}

server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    location / {
        proxy_pass http://thand_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /health {
        proxy_pass http://thand_backend/health;
        access_log off;
    }
}
```

Run Nginx as a separate container using Docker networks:

```bash
# Create a user-defined bridge network
docker network create thand-net

# Run Thand Agent container attached to the network
docker run -d \
  --name thand-agent-prod \
  --network thand-net \
  -p 8080:8080 \
  --restart unless-stopped \
  --memory=2g \
  --cpus=1.0 \
  -v $(pwd)/config:/app/config:ro \
  -v $(pwd)/logs:/app/logs \
  -e THAND_CONFIG_PATH=/app/config/config.yaml \
  ghcr.io/thand-io/agent:latest

# Run Nginx container attached to the same network
docker run -d \
  --name nginx-proxy \
  --network thand-net \
  -p 80:80 \
  -p 443:443 \
  -v $(pwd)/nginx.conf:/etc/nginx/nginx.conf:ro \
  -v $(pwd)/ssl:/etc/nginx/ssl:ro \
  nginx:alpine
```

## Security Considerations

### Non-root User

The official image runs as a non-root user by default:

```dockerfile
# Already included in the official image
RUN addgroup -S agent && adduser -S agent -G agent
USER agent
```

### Secret Management

Use Docker secrets for sensitive data in Docker Swarm mode:

```bash
# Create secrets (requires Docker Swarm mode)
echo "your-aws-access-key" | docker secret create aws_access_key -
echo "your-aws-secret-key" | docker secret create aws_secret_key -

# For standalone Docker, use environment variables or mounted files
docker run -d \
  --name thand-agent \
  -p 8080:8080 \
  -e AWS_ACCESS_KEY_ID_FILE=/run/secrets/aws_access_key \
  -e AWS_SECRET_ACCESS_KEY_FILE=/run/secrets/aws_secret_key \
  -v $(pwd)/secrets:/run/secrets:ro \
  -v $(pwd)/thand-config:/app/config:ro \
  ghcr.io/thand-io/agent:latest
```

### Network Security

Restrict container network access:

```bash
# Create custom network
docker network create --driver bridge thand-network

# Run with custom network and restricted port binding
docker run -d \
  --name thand-agent \
  --network thand-network \
  -p 127.0.0.1:8080:8080 \
  -v $(pwd)/thand-config:/app/config:ro \
  ghcr.io/thand-io/agent:latest
```

## Monitoring and Logging

### Log Management

Configure structured logging with Docker logging drivers:

```bash
docker run -d \
  --name thand-agent \
  -p 8080:8080 \
  --log-driver json-file \
  --log-opt max-size=100m \
  --log-opt max-file=3 \
  -v $(pwd)/thand-config:/app/config:ro \
  ghcr.io/thand-io/agent:latest
```

### Health Checks

The image includes built-in health checks:

```bash
# Manual health check
docker exec thand-agent wget --no-verbose --tries=1 --spider http://localhost:8080/health

# View health status
docker inspect --format='{% raw %}{{.State.Health.Status}}{% endraw %}' thand-agent
```

### Metrics Collection

Add Prometheus monitoring with separate containers:

```bash
# Create monitoring network
docker network create monitoring

# Run Prometheus
docker run -d \
  --name prometheus \
  --network monitoring \
  -p 9090:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus

# Connect thand-agent to monitoring network
docker network connect monitoring thand-agent
```

## Troubleshooting

### Common Issues

#### Container Won't Start

```bash
# Check logs
docker logs thand-agent

# Check configuration
docker exec -it thand-agent cat /app/config/config.yaml

# Verify permissions
docker exec -it thand-agent ls -la /app/config/
```

#### Configuration Not Loading

```bash
# Verify volume mounts
docker inspect thand-agent | grep -A 10 "Mounts"

# Check file permissions
ls -la ./thand-config/
```

#### Network Connectivity Issues

```bash
# Test container networking
docker exec -it thand-agent ping google.com

# Check port bindings
docker port thand-agent
```

### Debug Mode

Run with debug logging:

```bash
docker run -d \
  --name thand-agent-debug \
  -p 8080:8080 \
  -e THAND_LOGGING_LEVEL=debug \
  -v $(pwd)/thand-config:/app/config:ro \
  ghcr.io/thand-io/agent:latest
```

## Next Steps

- Configure [providers](../../configuration/providers) for your cloud environments
- Define [roles](../../configuration/roles) for your organization
- Set up [approval workflows](../../configuration/workflows)
- Integrate with your existing authentication systems
- Set up monitoring and alerting for production deployments

