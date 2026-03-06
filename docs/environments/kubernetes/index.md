---
layout: default
title: Kubernetes
parent: Environments
nav_order: 6
description: "Setup guide for deploying Thand Server on Kubernetes"
has_children: true
---

# Kubernetes Deployment
{: .no_toc }

Complete guide to deploying Thand Agent on Kubernetes clusters with full RBAC integration.
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

This guide walks you through deploying Thand Agent on Kubernetes clusters, including:

- Setting up RBAC permissions for the agent
- Deploying the agent using Kubernetes manifests
- Configuring providers, roles, and workflows
- Integration with Kubernetes authentication systems

## Prerequisites

Before deploying Thand Agent on Kubernetes, ensure you have:

- **Kubernetes cluster** with version 1.20 or later
- **kubectl** configured to access your cluster
- **Docker** (for building custom images)
- **Cluster admin privileges** to create ClusterRoles and ClusterRoleBindings
- **Container registry access** (optional, for custom images)

## Architecture

Thand Agent runs as a Deployment in Kubernetes with the following components:

- **ServiceAccount**: For in-cluster authentication
- **ClusterRole**: RBAC permissions to manage roles and bindings
- **Deployment**: The Thand Agent server
- **Service**: Internal cluster access
- **ConfigMaps & Secrets**: Configuration storage

## Authentication Methods

The Kubernetes provider automatically detects and uses:

1. **In-cluster configuration** when running inside Kubernetes (recommended)
2. **Kubeconfig file** when running externally (development)

## Quick Start

### 1. Clone and Prepare

```bash
git clone https://github.com/thand-io/agent.git
cd agent/internal/providers/kubernetes/kubeadm
```

### 2. Deploy with Script

```bash
# Build and deploy to your current kubectl context
./deploy.sh
```

This script automatically detects your Kubernetes environment:
- **Docker Desktop**: Uses local Docker daemon
- **minikube**: Uses minikube's Docker environment  
- **kind**: Loads image into kind cluster
- **Other**: Manual image loading required

### 3. Verify Deployment

```bash
# Check deployment status
kubectl get pods -n thand-system -l app=thand-agent

# Check logs
kubectl logs -n thand-system deployment/thand-agent

# Port forward for local access
kubectl port-forward -n thand-system svc/thand-agent 8080:8080
```

## Manual Deployment

### 1. Create Namespace and RBAC

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: thand-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: thand-agent
  namespace: thand-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: thand-agent
rules:
# RBAC permissions for managing roles and bindings
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings", "clusterroles", "clusterrolebindings"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
# Namespace access
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list"]
# Service account access (for user validation)
- apiGroups: [""]
  resources: ["serviceaccounts"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: thand-agent
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: thand-agent
subjects:
- kind: ServiceAccount
  name: thand-agent
  namespace: thand-system
```

### 2. Create Configuration

Create the main configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: thand-config
  namespace: thand-system
data:
  config.yaml: |
    # Server Configuration
    server:
      port: 8080
      host: "0.0.0.0"
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
      output: "stdout"

    # Providers Configuration
    providers:
      kubernetes-cluster:
        description: Current Kubernetes cluster
        provider: kubernetes
        config: {}
        enabled: true

    # Configuration paths
    roles:
      path: /app/config/roles/
    workflows:
      path: /app/config/workflows/
```

### 3. Create Configuration Secrets

```bash
# Create secrets for providers, roles, and workflows
kubectl create secret generic thand-providers-config \
  --namespace=thand-system \
  --from-file=all.yaml=../../../../config/providers/all.yaml

kubectl create secret generic thand-roles-config \
  --namespace=thand-system \
  --from-file=all.yaml=../../../../config/roles/all.yaml

kubectl create secret generic thand-workflows-config \
  --namespace=thand-system \
  --from-file=all.yaml=../../../../config/workflows/all.yaml
```

### 4. Deploy the Agent

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thand-agent
  namespace: thand-system
  labels:
    app: thand-agent
spec:
  replicas: 1
  selector:
    matchLabels:
      app: thand-agent
  template:
    metadata:
      labels:
        app: thand-agent
    spec:
      serviceAccountName: thand-agent
      containers:
      - name: thand-agent
        image: ghcr.io/thand-io/agent:latest
        ports:
        - containerPort: 8080
          name: http
        command: ["./agent", "server", "--config", "/app/config/config.yaml"]
        env:
        - name: THAND_LOGGING_LEVEL
          value: "info"
        volumeMounts:
        - name: config
          mountPath: /app/config
          readOnly: true
        - name: roles-config
          mountPath: /app/config/roles
          readOnly: true
        - name: providers-config
          mountPath: /app/config/providers
          readOnly: true
        - name: workflows-config
          mountPath: /app/config/workflows
          readOnly: true
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "4Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: thand-config
      - name: roles-config
        secret:
          secretName: thand-roles-config
      - name: providers-config
        secret:
          secretName: thand-providers-config
      - name: workflows-config
        secret:
          secretName: thand-workflows-config
---
apiVersion: v1
kind: Service
metadata:
  name: thand-agent
  namespace: thand-system
  labels:
    app: thand-agent
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: http
    protocol: TCP
    name: http
  selector:
    app: thand-agent
```

## Configuration

### Provider Configuration

The Kubernetes provider requires minimal configuration:

```yaml
providers:
  kubernetes-prod:
    description: Production Kubernetes cluster
    provider: kubernetes
    config: {}  # No configuration needed
    enabled: true
```

### Role Examples

#### Namespace-Scoped Role

```yaml
roles:
  dev-pod-reader:
    name: Development Pod Reader
    description: Read pods in development namespace
    providers:
      - kubernetes-prod
    resources:
      allow:
        - "namespace:development"
    permissions:
      allow:
        - "k8s:pods:get,list,watch"
        - "k8s:services:get,list"
```

#### Cluster-Wide Role

```yaml
roles:
  cluster-viewer:
    name: Cluster Viewer
    description: View resources across all namespaces
    providers:
      - kubernetes-prod
    permissions:
      allow:
        - "k8s:pods:get,list,watch"
        - "k8s:namespaces:get,list"
        - "k8s:services:get,list"
```

### Built-in Roles

The provider includes several built-in roles:

- **`view`** - Read-only access to most objects in a namespace
- **`edit`** - Read/write access to most objects in a namespace  
- **`admin`** - Full admin access within a namespace
- **`cluster-admin`** - Full admin access across the entire cluster
- **`pod-reader`** - Read-only access to pods
- **`deployment-manager`** - Manage deployments and related resources
- **`secret-manager`** - Manage secrets and config maps
- **`network-admin`** - Manage networking resources
- **`storage-admin`** - Manage storage resources

## Permission Format

Kubernetes permissions follow the format: `k8s:[apiGroup/]resource:verb`

Examples:
- `k8s:pods:get` - Read access to pods
- `k8s:apps/deployments:create` - Create deployments
- `k8s:secrets:list` - List secrets
- `k8s:rbac.authorization.k8s.io/roles:update` - Update RBAC roles
- `k8s:networking.k8s.io/ingresses:delete` - Delete ingresses

Multiple verbs can be specified with commas:
- `k8s:pods:get,list,watch`
- `k8s:deployments:create,update,patch,delete`

## Troubleshooting

### Common Issues

#### Permission Denied

```bash
# Check thand-agent permissions
kubectl auth can-i create clusterrolebindings --as=system:serviceaccount:thand-system:thand-agent

# Check agent logs
kubectl logs -n thand-system deployment/thand-agent
```

#### Role Not Found

- Verify role definition in configuration
- Check role name matches exactly (case-sensitive)
- Ensure configuration is properly mounted

#### Deployment Issues

```bash
# Check pod status
kubectl describe pod -n thand-system -l app=thand-agent

# Check service account
kubectl get serviceaccount -n thand-system thand-agent

# Verify RBAC
kubectl get clusterrole thand-agent
kubectl get clusterrolebinding thand-agent
```

### Health Checks

The agent provides health endpoints:

```bash
# Health check
curl http://localhost:8080/health

# Readiness check  
curl http://localhost:8080/ready
```

## Production Considerations

### High Availability

For production deployments:

```yaml
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
```

### Resource Limits

Adjust based on cluster size:

```yaml
resources:
  requests:
    memory: "2Gi"
    cpu: "1000m"
  limits:
    memory: "8Gi"
    cpu: "2000m"
```

### Network Policies

Restrict network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: thand-agent
  namespace: thand-system
spec:
  podSelector:
    matchLabels:
      app: thand-agent
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to: []
    ports:
    - protocol: TCP
      port: 443  # Kubernetes API
```

### Monitoring

Add monitoring annotations:

```yaml
metadata:
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/path: "/metrics"
```

## Example Usage

Once deployed, users can request access:

```bash
# Grant namespace access
thand elevate --provider kubernetes-prod --role dev-pod-reader --user john@company.com

# Grant cluster access
thand elevate --provider kubernetes-prod --role cluster-viewer --user admin@company.com

# Revoke access
thand revoke --provider kubernetes-prod --role dev-pod-reader --user john@company.com
```

## Next Steps

- Configure [OIDC integration](../../configuration/providers/oauth2) for user authentication
- Set up [workflows](../../configuration/workflows) for approval processes
- Define custom [roles](../../configuration/roles) for your organization
- Integrate with monitoring and alerting systems
