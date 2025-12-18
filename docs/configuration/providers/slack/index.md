---
layout: default
title: Slack
description: Slack provider for team communication and notifications
parent: Providers
grand_parent: Configuration
---

# Slack Provider

The Slack provider enables integration with Slack for notifications and team communication.

## Capabilities

- **Notifications**: Send notifications to Slack channels
- **Team Integration**: Access to Slack workspace and user information
- **Bot Integration**: Support for Slack bot tokens and app integration

## Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `token` | string | Yes | Slack bot token or OAuth token |
| `channel` | string | No | Default channel for notifications |
| `webhook_url` | string | No | Slack webhook URL (alternative to token) |

## Example Configuration

```yaml
version: "1.0"
providers:
  slack:
    name: Slack
    description: Slack notifications
    provider: slack
    enabled: true
    config:
      token: YOUR_SLACK_BOT_TOKEN
      channel: "#general"
```

## Slack App Manifest

To configure the Slack app, you can use the following manifest. This configuration includes the necessary scopes and settings for the provider to function correctly.

```json
{
    "display_information": {
        "name": "Thand",
        "description": "Distributed PAM",
        "background_color": "#361290"
    },
    "features": {
        "bot_user": {
            "display_name": "Thand",
            "always_online": false
        }
    },
    "oauth_config": {
        "scopes": {
            "bot": [
                "channels:join",
                "channels:read",
                "chat:write",
                "users:read.email",
                "users:read"
            ]
        }
    },
    "settings": {
        "interactivity": {
            "is_enabled": true,
            "request_url": "https://slack.external.thand.io/"
        },
        "org_deploy_enabled": false,
        "socket_mode_enabled": false,
        "token_rotation_enabled": false
    }
}
```

The interactive endpoint (`request_url` under `interactivity`) helps provide responsiveness for the Slack blocks, enabling interactive features within the Slack interface.

## Setup Instructions

### Create the App

1. Go to [Your Apps](https://api.slack.com/apps) on the Slack API site.
2. Click **Create New App**.
3. Select **From an app manifest**.
4. Choose the workspace where you want to install the app.
5. Paste the JSON manifest provided above.
6. Review the configuration and click **Create**.

### Get Configuration Values

1. Once the app is created, navigate to **OAuth & Permissions** in the sidebar.
2. Click **Install to Workspace** to install the app to your Slack workspace.
3. After installation, copy the **Bot User OAuth Token** (it usually starts with `xoxb-`).
4. Use this token as the `token` value in your provider configuration.

For more details, refer to the [Slack API documentation](https://api.slack.com/).
