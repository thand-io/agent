---
layout: default
title: Container Apps
parent: Azure
grand_parent: Environments
nav_order: 1
description: Setup guide for using Azure Container to deploy the Thand Agent.
---

# App Runner Setup
{: .no_toc }

Complete guide to deploying Thand Agent on Azure Container Apps with IAM integration.
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

## Prerequisites

- An [Azure account](https://azure.microsoft.com/en-us/free/) with sufficient permissions to create resources.
- An Azure subscription. You can check your subscription ID in the Azure Portal.


### Enabling Vault (Azure Key Vault)

Many of the providers supported by Thand that require API keys or secrets can be configured to use Azure Key Vault to store and retrieve these secrets securely.

This can either be done by configuring the provider to use Key Vault directly, or by configuring Thand to use Key Vault as its secret backend.

In this example, we will configure Thand to use Key Vault as its secret backend. We will create three secrets for our roles, providers, and workflows.

A default provider for Azure using the managed identity attached to the Container App would look something like this:

```yaml
providers:
  azure:
    name: Azure Default
    description: Default Azure provider using managed identity
    provider: azure
    enabled: true
    config:
      subscription_id: your-subscription-id
```

Create a Key Vault:

- Navigate to the [Azure Portal](https://portal.azure.com/) and search for "Key vaults".
- Click on "Create".
- Select your subscription and resource group.
- Enter a unique name for your Key Vault.
- Select your region.
- Choose your pricing tier (Standard is sufficient for most use cases).
- Click "Review + create", then "Create".

Configure access for your Container App:

- In your Key Vault, go to "Access configuration" under Settings.
- Ensure "Azure role-based access control" is selected (recommended) or use "Vault access policy".
- If using RBAC, assign the "Key Vault Secrets User" role to your Container App's managed identity.
- If using access policies, add an access policy granting "Get" and "List" permissions for secrets to your Container App's managed identity.

Create the secrets:

- In your Key Vault, go to "Secrets" under Objects.
- Click on "Generate/Import".
- Select "Manual" as the upload option.
- Name: `thand-providers`
- Value: Provide your entire [provider](../../configuration/providers/) configuration in YAML or JSON format.
- Click "Create".

Repeat the above steps to create two more secrets:
- `thand-roles` - containing your [roles configuration](../../configuration/roles/)
- `thand-workflows` - containing your [workflows configuration](../../configuration/workflows/)

Documentation for configuring providers, roles and workflows can be found in the [Configuration](../../configuration/) section.

![Azure Key Vault](step04.png)

You might also need to store other secrets depending on your provider configurations. Or other environment specific secrets you want to manage via Key Vault. Unfortunately, you will need to create a secret per environment variable.

Otherwise, you can provide your configuration via a mounted volume or other methods as described in the [Configuration](../../configuration/) section.

### Deploying Thand Agent on Azure Container Apps

