---
layout: default
title: Helm Chart Installation
parent: Kubernetes
grand_parent: Environments
nav_order: 1
description: "Deploy Thand Agent using Helm"
---

# Helm Chart Installation
{: .no_toc }

Deploy Thand Agent to Kubernetes using our official Helm chart for simplified installation and management.
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

The Thand Agent Helm chart provides a production-ready deployment with:

- Automatic RBAC configuration
- ConfigMap and Secret management
- Health and readiness probes
- Configurable resource limits
- Optional ingress support
- Horizontal Pod Autoscaler (HPA) support

## Prerequisites

- **Kubernetes cluster** (1.19+)
- **Helm 3.0+** installed
- **kubectl** configured to access your cluster
- **Cluster admin privileges** for RBAC resources

## Quick Start

### Add Helm Repository

```bash
helm repo add thand https://helm.thand.io
helm repo update
```

### Install Chart

```bash
# Install with default values
helm install thand-agent thand/agent \
  --namespace thand-system \
  --create-namespace

# View installation notes
helm status thand-agent -n thand-system
```

### Verify Installation

```bash
# Check pod status
kubectl get pods -n thand-system

# View logs
kubectl logs -n thand-system -l app.kubernetes.io/name=agent -f

# Test health endpoint
kubectl port-forward -n thand-system svc/thand-agent 8080:8080
curl http://localhost:8080/health
```

## Configuration

### Basic Configuration

Create a `values.yaml` file to customize your deployment:

```yaml
# Image configuration
image:
  repository: ghcr.io/thand-io/agent
  tag: "0.0.63"
  pullPolicy: IfNotPresent

# Replica count
replicaCount: 2

# Resource limits
resources:
  requests:
    memory: "1Gi"
    cpu: "500m"
  limits:
    memory: "4Gi"
    cpu: "1000m"

# Logging configuration
config:
  logging:
    level: "info"
    format: "json"
```

Install with custom values:

```bash
helm install thand-agent thand/agent \
  -f values.yaml \
  -n thand-system \
  --create-namespace
```

### Roles, Providers, and Workflows

You can provide your configuration inline or use existing secrets.

#### Option 1: Inline Configuration

```yaml
roles:
  enabled: true
  files:
    all.yaml: |
      version: "1.0"
        developer:
          name: Developer
          description: Read-only access for developers
          workflows:
            - simple_approval
          permissions:
            allow:
              - ec2:DescribeInstances
              - s3:ListBucket
              - s3:GetObject
          resources:
            allow:
              - "namespace:default"
          providers:
            - kubernetes-cluster
          enabled: true

providers:
  enabled: true
  files:
    all.yaml: |
      version: "1.0"
      providers:
        kubernetes-cluster:
          name: Kubernetes Cluster
          description: Current Kubernetes cluster
          provider: kubernetes
          config: {}
          enabled: true

workflows:
  enabled: true
  files:
    all.yaml: |
      version: "1.0"
      workflows:
        simple_approval:
          description: Simple approval workflow
          authentication: google_oauth2
          enabled: true
          workflow:
            document:
              dsl: "1.0.0-alpha5"
              namespace: "thand"
              name: "simple-approval-workflow"
              version: "1.0.0"
            do:
              - validate:
                  thand: validate
                  with:
                    validator: static
                  then: approvals
              - approvals:
                  thand: approvals
                  on:
                    approved: authorize
                    denied: denied
                  with:
                    approvals: 1
                    selfApprove: false
                    notifiers:
                      - type: slack
                        channel: "#approvals"
              - authorize:
                  thand: authorize
              - denied:
                  thand: deny
```

#### Option 2: Existing Secrets

If you already have secrets in your cluster:

```bash
# Create secrets from files
kubectl create secret generic thand-roles \
  --from-file=all.yaml=./config/roles/all.yaml \
  -n thand-system

kubectl create secret generic thand-providers \
  --from-file=all.yaml=./config/providers/all.yaml \
  -n thand-system

kubectl create secret generic thand-workflows \
  --from-file=all.yaml=./config/workflows/all.yaml \
  -n thand-system
```

Then reference them in your values:

```yaml
roles:
  enabled: true
  existingSecret: thand-roles

providers:
  enabled: true
  existingSecret: thand-providers

workflows:
  enabled: true
  existingSecret: thand-workflows
```

### Ingress Configuration

Enable ingress to expose the agent externally:

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: thand.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: thand-tls
      hosts:
        - thand.example.com
```

### Autoscaling

Enable Horizontal Pod Autoscaler:

```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 80
```

## Advanced Configuration

### RBAC Customization

Customize the ClusterRole permissions:

```yaml
rbac:
  create: true
  rules:
    - apiGroups: ["rbac.authorization.k8s.io"]
      resources: ["roles", "rolebindings", "clusterroles", "clusterrolebindings"]
      verbs: ["get", "list", "create", "update", "patch", "delete"]
    - apiGroups: [""]
      resources: ["namespaces"]
      verbs: ["get", "list"]
    - apiGroups: [""]
      resources: ["serviceaccounts"]
      verbs: ["get", "list", "create"]
    # Add custom rules here
```

### Environment Variables

Add custom environment variables:

```yaml
env:
  - name: THAND_LOGGING_LEVEL
    value: "debug"
  - name: CUSTOM_VAR
    value: "custom-value"

# Or from secrets/configmaps
envFrom:
  - secretRef:
      name: thand-secrets
  - configMapRef:
      name: thand-config
```

### Pod Security

Configure security context:

```yaml
podSecurityContext:
  runAsNonRoot: true
  fsGroup: 1000

securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
  readOnlyRootFilesystem: true
  runAsUser: 1000
```

## Upgrading

### Upgrade Chart

```bash
# Update repository
helm repo update

# Upgrade to latest version
helm upgrade thand-agent thand/agent -n thand-system

# Upgrade with new values
helm upgrade thand-agent thand/agent \
  -f values.yaml \
  -n thand-system
```

### View Upgrade History

```bash
# List releases
helm list -n thand-system

# View revision history
helm history thand-agent -n thand-system
```

### Rollback

```bash
# Rollback to previous version
helm rollback thand-agent -n thand-system

# Rollback to specific revision
helm rollback thand-agent 2 -n thand-system
```

## Uninstallation

```bash
# Uninstall release
helm uninstall thand-agent -n thand-system

# Delete namespace (optional)
kubectl delete namespace thand-system

# Note: ClusterRole and ClusterRoleBinding are automatically removed
```

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -n thand-system
kubectl describe pod -n thand-system <pod-name>
```

### View Logs

```bash
# All pods
kubectl logs -n thand-system -l app.kubernetes.io/name=agent -f

# Specific pod
kubectl logs -n thand-system <pod-name> -f
```

### Common Issues

#### Image Pull Errors

```bash
# Check image pull policy
helm get values thand-agent -n thand-system

# Use specific tag
helm upgrade thand-agent thand/agent \
  --set image.tag=0.0.63 \
  -n thand-system
```

#### RBAC Permissions

```bash
# Verify ClusterRole
kubectl get clusterrole thand-agent -o yaml

# Verify ClusterRoleBinding
kubectl get clusterrolebinding thand-agent -o yaml

# Check ServiceAccount
kubectl get sa -n thand-system thand-agent -o yaml
```

#### Configuration Issues

```bash
# View ConfigMap
kubectl get configmap thand-agent-config -n thand-system -o yaml

# View Secrets
kubectl get secret -n thand-system | grep thand-agent
kubectl get secret thand-agent-roles -n thand-system -o yaml
```

### Debug Mode

Enable debug logging:

```bash
helm upgrade thand-agent thand/agent \
  --set config.logging.level=debug \
  -n thand-system
```

## Testing

Run Helm tests to verify the installation:

```bash
helm test thand-agent -n thand-system
```

## Configuration Reference

For a complete list of configurable parameters, see the [values.yaml](https://github.com/thand-io/helm-charts/blob/main/charts/agent/values.yaml) file or run:

```bash
helm show values thand/agent
```

## Next Steps

- [Configure Kubernetes Provider](../../../configuration/providers/kubernetes.html)
- [Set up Roles](../../../configuration/roles/)
- [Create Workflows](../../../configuration/workflows/)
- [Production Deployment Best Practices](#production-deployment)

## Production Deployment

### Recommended Settings

```yaml
# High availability
replicaCount: 3

# Resource allocation
resources:
  requests:
    memory: "2Gi"
    cpu: "1000m"
  limits:
    memory: "8Gi"
    cpu: "2000m"

# Enable autoscaling
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80

# Pod disruption budget
podDisruptionBudget:
  enabled: true
  minAvailable: 2

# Security
podSecurityContext:
  runAsNonRoot: true
  fsGroup: 1000

securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
  readOnlyRootFilesystem: true

# Monitoring
podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8080"
  prometheus.io/path: "/metrics"

# Node affinity
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: agent
          topologyKey: kubernetes.io/hostname
```

## Support

- [Documentation](https://docs.thand.io)
- [GitHub Issues](https://github.com/thand-io/agent/issues)
- [Helm Chart Repository](https://github.com/thand-io/helm-charts)
