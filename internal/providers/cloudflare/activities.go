package cloudflare

import (
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func (b *cloudflareProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &cloudflareProviderActivities{provider: b}
}
