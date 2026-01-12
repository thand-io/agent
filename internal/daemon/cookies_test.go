package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCookieDomain(t *testing.T) {
	tests := []struct {
		name           string
		hostname       string
		expectedDomain string
		description    string
	}{
		{
			name:           "localhost should use host-only cookies",
			hostname:       "localhost",
			expectedDomain: "",
			description:    "localhost should not set Domain attribute for security",
		},
		{
			name:           "127.0.0.1 should use host-only cookies",
			hostname:       "127.0.0.1",
			expectedDomain: "",
			description:    "loopback IP should not set Domain attribute",
		},
		{
			name:           "empty hostname should use host-only cookies",
			hostname:       "",
			expectedDomain: "",
			description:    "empty hostname should not set Domain attribute",
		},
		{
			name:           "Azure Container Apps with subdomain",
			hostname:       "thand.livelysand-47c199af.eastus.azurecontainerapps.io",
			expectedDomain: ".azurecontainerapps.io",
			description:    "Azure Container Apps should extract parent domain (azurecontainerapps.io is in public suffix list)",
		},
		{
			name:           "Azure Container Apps different region",
			hostname:       "myapp.subdomain.westus2.azurecontainerapps.io",
			expectedDomain: ".azurecontainerapps.io",
			description:    "Azure Container Apps in different region should work",
		},
		{
			name:           "AWS App Runner with subdomain",
			hostname:       "myapp-abc123.us-east-1.awsapprunner.com",
			expectedDomain: "",
			description:    "AWS App Runner - awsapprunner.com is a public suffix, uses host-only",
		},
		{
			name:           "GCP Cloud Run with subdomain",
			hostname:       "my-service-abc123.europe-west1.run.app",
			expectedDomain: "",
			description:    "GCP Cloud Run - run.app is a public suffix, uses host-only",
		},
		{
			name:           "GCP Cloud Run different region",
			hostname:       "service-xyz.us-central1.run.app",
			expectedDomain: "",
			description:    "GCP Cloud Run in different region - run.app is a public suffix",
		},
		{
			name:           "simple custom domain",
			hostname:       "example.com",
			expectedDomain: "",
			description:    "base domain should use host-only cookies",
		},
		{
			name:           "subdomain on custom domain",
			hostname:       "app.example.com",
			expectedDomain: ".example.com",
			description:    "subdomain should set parent domain",
		},
		{
			name:           "multi-level subdomain on custom domain",
			hostname:       "api.staging.example.com",
			expectedDomain: ".example.com",
			description:    "multi-level subdomain should set base domain",
		},
		{
			name:           "UK domain",
			hostname:       "app.example.co.uk",
			expectedDomain: ".example.co.uk",
			description:    "should handle multi-part TLD correctly",
		},
		{
			name:           "base UK domain",
			hostname:       "example.co.uk",
			expectedDomain: "",
			description:    "base domain with multi-part TLD should use host-only",
		},
		{
			name:           "GitHub Pages",
			hostname:       "username.github.io",
			expectedDomain: "",
			description:    "github.io is a public suffix, should use host-only",
		},
		{
			name:           "Heroku app",
			hostname:       "myapp-staging.herokuapp.com",
			expectedDomain: "",
			description:    "Heroku - herokuapp.com is a public suffix, uses host-only",
		},
		{
			name:           "Vercel deployment",
			hostname:       "myproject.vercel.app",
			expectedDomain: "",
			description:    "vercel.app is a public suffix, should use host-only",
		},
		{
			name:           "Netlify deployment",
			hostname:       "mysite.netlify.app",
			expectedDomain: "",
			description:    "netlify.app is a public suffix, should use host-only",
		},
		{
			name:           "AWS CloudFront",
			hostname:       "d1234567890.cloudfront.net",
			expectedDomain: "",
			description:    "cloudfront.net is a public suffix, should use host-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCookieDomain(tt.hostname)
			assert.Equal(t, tt.expectedDomain, result, tt.description)
		})
	}
}

func TestGetCookieDomain_SecurityCases(t *testing.T) {
	t.Run("should not allow overly broad domain", func(t *testing.T) {
		// Test that we don't accidentally set cookie for entire TLD
		hostname := "example.com"
		result := getCookieDomain(hostname)
		assert.Equal(t, "", result, "base domain should not set Domain attribute to avoid overly broad scope")
	})

	t.Run("should handle malformed hostnames gracefully", func(t *testing.T) {
		// Test various malformed inputs
		malformedHosts := []string{
			"not..valid",
			".invalid",
			"invalid.",
			"some space.com",
		}

		for _, hostname := range malformedHosts {
			result := getCookieDomain(hostname)
			// Should either return empty string or a valid domain, never panic
			assert.NotPanics(t, func() {
				getCookieDomain(hostname)
			}, "should not panic on malformed hostname: %s", hostname)
			t.Logf("Malformed hostname '%s' returned domain: '%s'", hostname, result)
		}
	})
}

func TestGetCookieDomain_PlatformSpecific(t *testing.T) {
	t.Run("Azure Container Apps variations", func(t *testing.T) {
		testCases := []struct {
			hostname string
			expected string
		}{
			{"app.eastus.azurecontainerapps.io", ".azurecontainerapps.io"},
			{"my-app.westus.azurecontainerapps.io", ".azurecontainerapps.io"},
			{"service.northeurope.azurecontainerapps.io", ".azurecontainerapps.io"},
			{"test.sub.eastus2.azurecontainerapps.io", ".azurecontainerapps.io"},
		}

		for _, tc := range testCases {
			result := getCookieDomain(tc.hostname)
			assert.Equal(t, tc.expected, result, "Azure Container Apps hostname: %s", tc.hostname)
		}
	})

	t.Run("AWS App Runner variations", func(t *testing.T) {
		testCases := []struct {
			hostname string
			expected string
		}{
			{"abc123.us-east-1.awsapprunner.com", ""},
			{"xyz789.eu-west-1.awsapprunner.com", ""},
			{"service.us-west-2.awsapprunner.com", ""},
		}

		for _, tc := range testCases {
			result := getCookieDomain(tc.hostname)
			assert.Equal(t, tc.expected, result, "AWS App Runner hostname: %s", tc.hostname)
		}
	})

	t.Run("GCP Cloud Run variations", func(t *testing.T) {
		testCases := []struct {
			hostname string
			expected string
		}{
			{"service-abc.us-central1.run.app", ""},
			{"app-xyz.europe-west1.run.app", ""},
			{"api-123.asia-southeast1.run.app", ""},
		}

		for _, tc := range testCases {
			result := getCookieDomain(tc.hostname)
			assert.Equal(t, tc.expected, result, "GCP Cloud Run hostname: %s", tc.hostname)
		}
	})
}
