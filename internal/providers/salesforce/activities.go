package salesforce

import (
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func (b *salesForceProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &salesForceProviderActivities{provider: b}
}
