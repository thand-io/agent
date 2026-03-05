package salesforce

func (b *salesForceProvider) RegisterActivities() any {
	return &salesForceProviderActivities{provider: b}
}
