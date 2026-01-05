---
layout: default
title: GCP IAP
description: Google Cloud Platform Identity-Aware Proxy authentication provider
parent: Providers
grand_parent: Configuration
---

# GCP IAP Provider

The GCP IAP (Identity-Aware Proxy) provider enables authentication when your application is deployed behind Google Cloud's Identity-Aware Proxy.

## Setup Guide

### Step 1: Create OIDC Credentials

To allow programmatic access (e.g., for CLI tools), you need an OAuth 2.0 client ID.

1. Go to [APIs & Services > Credentials](https://console.cloud.google.com/apis/credentials).
2. Click **Create Credentials** → **OAuth 2.0 Client ID**.
3. Choose **Desktop app** (for CLI tools) or **Web application**.
4. Copy the **Client ID** (e.g., `XXXXX.apps.googleusercontent.com`) and **Client Secret**.

### Step 2: Choose App Platform & Enable IAP

Choose your deployment platform to enable IAP.

#### Cloud Run

1.  **Deploy your Cloud Run service**:
    Follow the [Cloud Run deployment documentation](https://cloud.google.com/run/docs/deploying) to deploy your service.

2.  **Enable IAP**:
    Enable Identity-Aware Proxy for your Cloud Run service via the Google Cloud Console:
    1. Go to the cloud run application. Click on the **Security** tab. Under authentication, click on **Require authentication** and check both boxes to allow only authenticated invocations (**Identity and Access Management (IAM)** and **Identity-Aware Proxy (IAP)**).
    2. Then you need to manage the users who can access the application. Click on the **Edit policy**. Then add the users or groups who should have access to the application. No need to add a role.
    3. Save your changes.

### Step 3: Enable OIDC as Auth Method

To allow your OIDC client (created in Step 1) to authenticate with IAP, you must add it to the IAP settings.

#### Cloud Run Example (Without Load Balancer)

Use the following commands to apply the OIDC settings.

1.  **List your services**:
    ```bash
    gcloud run services list --project=thand
    ```

2.  **Apply IAP Settings**:
    Create a file named `iap_settings.yaml` with your OAuth client ID:

    ```yaml
    access_settings:
      oauth_settings:
        programmatic_clients: ["PROJECTID-XXXX.apps.googleusercontent.com"]
    ```

    Then apply the settings:

    ```bash
    # Set IAP settings on the Cloud Run service:
    gcloud beta iap settings set iap_settings.yaml \
      --project=thand  \
      --resource-type=cloud-run \
      --service=agent \
      --region=europe-west1
    ```

## Agent Configuration

Configure the provider in your `config.yaml`:

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      # REQUIRED: JWT audience claim for validating incoming IAP JWTs
      audience: "/projects/PROJECT_NUMBER/locations/REGION/services/SERVICE_NAME"
      
      # REQUIRED for programmatic access: OAuth 2.0 client ID
      oauth_client_id: "XXXXX.apps.googleusercontent.com"
      oauth_client_secret: "YOUR_CLIENT_SECRET"
```

After configuring the provider, restart your application to apply the changes.
You will now be able to authenticate using GCP IAP and use the CLI tools with programmatic access.
