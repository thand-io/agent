package environment

import (
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/thand-io/agent/internal/models"
)

/*
type EnvironmentMode string

const (
	Server EnvironmentMode = "server"
	Client EnvironmentMode = "client"
)
*/
// DetectEnvironmentConfig automatically detects and returns an EnvironmentConfig
// based on the current runtime environment, cloud platform, and execution context
func DetectEnvironmentConfig() models.EnvironmentConfig {
	config := models.EnvironmentConfig{
		Name:                   DetectSystemName(),
		OperatingSystem:        DetectOperatingSystem(),
		OperatingSystemVersion: DetectOSVersion(),
		Architecture:           runtime.GOARCH,
		Platform:               DetectPlatform(),
		Ephemeral:              IsEphemeralEnvironment(),
		Config:                 &models.BasicConfig{},
		MetaData: &models.BasicConfig{
			"detection_time": time.Now().UTC().Format(time.RFC3339),
			"go_version":     runtime.Version(),
		},
	}

	return config
}

// DetectOperatingSystem returns the operating system name
func DetectOperatingSystem() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}

// DetectSystemName detects and returns a meaningful system name based on the environment
func DetectSystemName() string {
	// Check for specific cloud function/service names first
	if functionName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); len(functionName) > 0 {
		return functionName
	}

	if functionName := os.Getenv("FUNCTION_NAME"); len(functionName) > 0 {
		return functionName
	}

	if serviceName := os.Getenv("K_SERVICE"); len(serviceName) > 0 {
		return serviceName
	}

	if siteName := os.Getenv("WEBSITE_SITE_NAME"); len(siteName) > 0 {
		return siteName
	}

	// Check for container/pod names
	if hostname, err := os.Hostname(); err == nil && len(hostname) > 0 {
		// In Kubernetes, hostname is often the pod name
		if isKubernetes() {
			return hostname
		}
		// For other environments, use hostname if it's not generic
		if !isGenericHostname(hostname) {
			return hostname
		}
	}

	// Check for environment-specific naming
	if envName := os.Getenv("ENVIRONMENT"); len(envName) > 0 {
		return envName
	}

	if envName := os.Getenv("ENV"); len(envName) > 0 {
		return envName
	}

	if stage := os.Getenv("STAGE"); len(stage) > 0 {
		return stage
	}

	// Generate a name based on platform and OS
	platform := DetectPlatform()
	os := DetectOperatingSystem()

	if platform == "local" {
		return "local-" + os
	}

	return string(platform) + "-" + os
}

// isGenericHostname checks if a hostname is too generic to be useful
func isGenericHostname(hostname string) bool {
	genericNames := []string{
		"localhost",
		"computer",
		"desktop",
		"laptop",
		"workstation",
		"server",
		"host",
		"machine",
	}

	lowerHostname := strings.ToLower(hostname)
	for _, generic := range genericNames {
		if strings.Contains(lowerHostname, generic) {
			return true
		}
	}

	// Check for patterns like "ip-xxx-xxx-xxx-xxx" (AWS EC2 default)
	if strings.HasPrefix(lowerHostname, "ip-") && strings.Count(hostname, "-") >= 4 {
		return true
	}

	return false
}

// DetectOSVersion attempts to detect the OS version
func DetectOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		return detectWindowsVersion()
	case "darwin":
		return detectDarwinVersion()
	case "linux":
		return detectLinuxVersion()
	default:
		return "unknown"
	}
}

// DetectPlatform detects the cloud platform or environment
func DetectPlatform() models.EnvironmentPlatform {
	// Check for AWS
	if isAWS() {
		return models.AWS
	}

	// Check for GCP
	if isGCP() {
		return models.GCP
	}

	// Check for Azure
	if isAzure() {
		return models.Azure
	}

	// Check for Kubernetes
	if isKubernetes() {
		return models.Kubernetes
	}

	// Default to local/unknown
	return models.Local
}

// IsEphemeralEnvironment determines if we're running in an ephemeral environment
func IsEphemeralEnvironment() bool {
	// Check for AWS Lambda
	if len(os.Getenv("AWS_LAMBDA_FUNCTION_NAME")) > 0 || len(os.Getenv("LAMBDA_TASK_ROOT")) > 0 {
		return true
	}

	// Check for Google Cloud Functions
	if len(os.Getenv("FUNCTION_NAME")) > 0 || len(os.Getenv("K_SERVICE")) > 0 {
		return true
	}

	// Check for Azure Functions
	if len(os.Getenv("AZURE_FUNCTIONS_ENVIRONMENT")) > 0 || len(os.Getenv("FUNCTIONS_WORKER_RUNTIME")) > 0 {
		return true
	}

	// Check for Cloud Run
	if len(os.Getenv("K_SERVICE")) > 0 && len(os.Getenv("K_REVISION")) > 0 {
		return true
	}

	return false
}

// isAWS checks if we're running on AWS
func isAWS() bool {
	// Check for AWS environment variables
	if len(os.Getenv("AWS_REGION")) > 0 || len(os.Getenv("AWS_DEFAULT_REGION")) > 0 {
		return true
	}

	if len(os.Getenv("AWS_LAMBDA_FUNCTION_NAME")) > 0 {
		return true
	}

	// Check AWS metadata service
	// return checkAWSMetadata()
	return false
}

// isGCP checks if we're running on Google Cloud Platform
func isGCP() bool {
	// Check for GCP environment variables
	if len(os.Getenv("GOOGLE_CLOUD_PROJECT")) > 0 || len(os.Getenv("GCLOUD_PROJECT")) > 0 {
		return true
	}

	if len(os.Getenv("FUNCTION_NAME")) > 0 || len(os.Getenv("K_SERVICE")) > 0 {
		return true
	}

	// Check GCP metadata service
	// return checkGCPMetadata()
	return false
}

// isAzure checks if we're running on Microsoft Azure
func isAzure() bool {
	// Check for Azure environment variables
	if len(os.Getenv("AZURE_FUNCTIONS_ENVIRONMENT")) > 0 {
		return true
	}

	if len(os.Getenv("WEBSITE_SITE_NAME")) > 0 {
		return true
	}

	// Check Azure metadata service
	// return checkAzureMetadata()
	return false
}

// isKubernetes checks if we're running in a Kubernetes environment
func isKubernetes() bool {
	// Check for Kubernetes service account token
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		return true
	}

	// Check for Kubernetes environment variables
	if len(os.Getenv("KUBERNETES_SERVICE_HOST")) > 0 {
		return true
	}

	return false
}

// checkAWSMetadata attempts to contact AWS metadata service
func checkAWSMetadata() bool {
	client := resty.New().SetTimeout(2 * time.Second)
	resp, err := client.R().Get("http://169.254.169.254/latest/meta-data/")
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}

// checkGCPMetadata attempts to contact GCP metadata service
func checkGCPMetadata() bool {
	client := resty.New().SetTimeout(2 * time.Second)
	resp, err := client.R().
		SetHeader("Metadata-Flavor", "Google").
		Get("http://metadata.google.internal/computeMetadata/v1/")
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}

// checkAzureMetadata attempts to contact Azure metadata service
func checkAzureMetadata() bool {
	client := resty.New().SetTimeout(2 * time.Second)
	resp, err := client.R().
		SetHeader("Metadata", "true").
		Get("http://169.254.169.254/metadata/instance?api-version=2021-02-01")
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}

// detectWindowsVersion detects Windows version
func detectWindowsVersion() string {
	if runtime.GOOS != "windows" {
		return "unknown"
	}

	// Try to get version from environment
	if version := os.Getenv("OS"); len(version) > 0 {
		return version
	}

	// Fallback to generic Windows
	return "windows"
}

// detectLinuxVersion detects Linux distribution and version
func detectLinuxVersion() string {
	if runtime.GOOS != "linux" {
		return "unknown"
	}

	// Try reading /etc/os-release
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "VERSION_ID=") {
				version := strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
				return version
			}
		}
	}

	// Try reading /proc/version
	if data, err := os.ReadFile("/proc/version"); err == nil {
		version := string(data)
		if strings.Contains(version, "Ubuntu") {
			return "ubuntu"
		} else if strings.Contains(version, "Red Hat") {
			return "rhel"
		} else if strings.Contains(version, "CentOS") {
			return "centos"
		} else if strings.Contains(version, "Amazon Linux") {
			return "amazon-linux"
		}
	}

	return "linux"
}
