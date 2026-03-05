package kubernetes

func (b *kubernetesProvider) RegisterActivities() any {
	return &kubernetesProviderActivities{provider: b}
}
