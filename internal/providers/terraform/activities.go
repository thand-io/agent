package terraform

func (b *terraformProvider) RegisterActivities() any {
	return &terraformProviderActivities{provider: b}
}
