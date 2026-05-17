package terraform

import (
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func (b *terraformProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &terraformProviderActivities{provider: b}
}
