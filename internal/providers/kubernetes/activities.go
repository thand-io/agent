package kubernetes

import (
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func (b *kubernetesProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &kubernetesProviderActivities{provider: b}
}
