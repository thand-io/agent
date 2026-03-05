package cloudflare

func (b *cloudflareProvider) RegisterActivities() any {
	return &cloudflareProviderActivities{provider: b}
}
