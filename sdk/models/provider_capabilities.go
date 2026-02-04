package models

import (
	awsInternal "github.com/thand-io/agent/internal/providers/aws"
	azureInternal "github.com/thand-io/agent/internal/providers/azure"
	gcpInternal "github.com/thand-io/agent/internal/providers/gcp"
	emailInternal "github.com/thand-io/agent/internal/providers/email"
	oktaInternal "github.com/thand-io/agent/internal/providers/okta"
	kubernetesInternal "github.com/thand-io/agent/internal/providers/kubernetes"
	cloudflareInternal "github.com/thand-io/agent/internal/providers/cloudflare"
	githubInternal "github.com/thand-io/agent/internal/providers/github"
	salesforceInternal "github.com/thand-io/agent/internal/providers/salesforce"
	internal "github.com/thand-io/agent/internal/models"
)

var AwsCapabilities = awsInternal.AwsCapabilities
var AzureCapabilities = azureInternal.AzureCapabilities
var CloudflareCapabilities = cloudflareInternal.CloudflareCapabilities
var EmailCapabilities = emailInternal.EmailCapabilities
var GcpCapabilities = gcpInternal.GcpCapabilities
var GithubCapabilities = githubInternal.GithubCapabilities
var KubernetesCapabilities = kubernetesInternal.KubernetesCapabilities
var OktaCapabilities = oktaInternal.OktaCapabilities
var SalesforceCapabilities = salesforceInternal.SalesforceCapabilities

var ProviderCapabilitiesMap = map[string]*ProviderCapabilities{
	awsInternal.AwsProviderName:     AwsCapabilities,
	azureInternal.AzureProviderName: AzureCapabilities,
	cloudflareInternal.CloudflareProviderName: CloudflareCapabilities,
	emailInternal.EmailProviderName:    EmailCapabilities,
	gcpInternal.GcpProviderName:     GcpCapabilities,
	githubInternal.GithubProviderName: GithubCapabilities,
	kubernetesInternal.KubernetesProviderName: KubernetesCapabilities,
	oktaInternal.OktaProviderName:   OktaCapabilities,
	salesforceInternal.SalesforceProviderName: SalesforceCapabilities,
}

var GetCapabilityFromString = internal.GetCapabilityFromString

var NewProviderCapabilities = internal.NewProviderCapabilities
