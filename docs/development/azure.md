---
layout: default
title: Development - Azure
nav_order: 3
has_children: true
description: Building and Pushing Thand to Azure Container Registry
---

# Building and Pushing Thand to Azure Container Registry

This guide explains how to build and push the Thand agent container image to Azure Container Registry (ACR).

## Prerequisites

- Azure CLI installed and configured
- Docker installed on your machine
- Access to the Azure Container Registry
- Proper permissions to push images to the registry

## Steps

### 1. Authenticate with Azure Container Registry

First, log in to your Azure Container Registry:

```bash
az acr login --name thand
```

This command authenticates your Docker client with the Azure Container Registry named "thand".

### 2. Build the Linux Binary

Build the agent binary for Linux AMD64 architecture:

```bash
GOOS=linux GOARCH=amd64 GOEXPERIMENT=jsonv2 go build -ldflags "-s -w" -o dist/agent-linux-amd64 .
```

**Build flags explained:**
- `GOOS=linux` - Target operating system
- `GOARCH=amd64` - Target architecture
- `GOEXPERIMENT=jsonv2` - Enable JSON v2 experiment
- `-ldflags "-s -w"` - Strip debug information to reduce binary size
- `-o dist/agent-linux-amd64` - Output path for the binary

### 3. Build the Docker Image

Build the Docker image for the Linux AMD64 platform:

```bash
docker build --platform linux/amd64 -f Dockerfile -t thand.azurecr.io/thand-io/agent:latest .
```

**Parameters:**
- `--platform linux/amd64` - Build for Linux AMD64 platform
- `-f Dockerfile` - Use the Dockerfile in the current directory
- `-t thand.azurecr.io/thand-io/agent:latest` - Tag the image with registry path and version

### 4. Push the Image to Azure Container Registry

Push the built image to ACR:

```bash
docker push thand.azurecr.io/thand-io/agent:latest
```

## Complete Build Script

You can run all commands in sequence:

```bash
# Login to ACR
az acr login --name thand

# Build the binary
GOOS=linux GOARCH=amd64 GOEXPERIMENT=jsonv2 go build -ldflags "-s -w" -o dist/agent-linux-amd64 .

# Build the Docker image
docker build --platform linux/amd64 -f Dockerfile -t thand.azurecr.io/thand-io/agent:latest .

# Push to registry
docker push thand.azurecr.io/thand-io/agent:latest
```

## Tagging Different Versions

To push a specific version instead of `latest`:

```bash
# Build with version tag
docker build --platform linux/amd64 -f Dockerfile -t thand.azurecr.io/thand-io/agent:v1.0.0 .

# Push the versioned image
docker push thand.azurecr.io/thand-io/agent:v1.0.0
```

## Troubleshooting

### Authentication Issues

If you encounter authentication errors, ensure you have:
- Valid Azure credentials: `az login`
- Proper ACR permissions
- Correct registry name in the commands

### Build Failures

If the Docker build fails:
- Ensure the binary was built successfully in `dist/agent-linux-amd64`
- Check that the Dockerfile exists in the current directory
- Verify Docker is running: `docker ps`

### Push Failures

If the push fails:
- Confirm you're authenticated: `az acr login --name thand`
- Verify the registry name and image tag are correct
- Check your network connection and ACR status
