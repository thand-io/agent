package environment

import (
	"os"
	"runtime"
	"slices"
	"testing"
)

func TestDetectEnvironmentConfig(t *testing.T) {
	config := DetectEnvironmentConfig()

	// Test basic OS detection
	if len(config.OperatingSystem) == 0 {
		t.Error("OperatingSystem should not be empty")
	}

	// Should match runtime.GOOS
	expectedOS := runtime.GOOS
	if expectedOS == "darwin" {
		expectedOS = "darwin"
	}
	if config.OperatingSystem != expectedOS {
		t.Errorf("Expected OS %s, got %s", expectedOS, config.OperatingSystem)
	}

	// Test architecture detection
	if config.Architecture != runtime.GOARCH {
		t.Errorf("Expected architecture %s, got %s", runtime.GOARCH, config.Architecture)
	}

	// Test platform detection
	if len(config.Platform) == 0 {
		t.Error("Platform should not be empty")
	}

}

func TestDetectOperatingSystem(t *testing.T) {
	os := DetectOperatingSystem()

	switch runtime.GOOS {
	case "windows":
		if os != "windows" {
			t.Errorf("Expected windows, got %s", os)
		}
	case "darwin":
		if os != "darwin" {
			t.Errorf("Expected darwin, got %s", os)
		}
	case "linux":
		if os != "linux" {
			t.Errorf("Expected linux, got %s", os)
		}
	default:
		if os != runtime.GOOS {
			t.Errorf("Expected %s, got %s", runtime.GOOS, os)
		}
	}
}

func TestIsEphemeralEnvironment(t *testing.T) {
	// Save original environment
	originalLambda := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	originalFunction := os.Getenv("FUNCTION_NAME")
	originalAzure := os.Getenv("AZURE_FUNCTIONS_ENVIRONMENT")

	// Clean up after test
	defer func() {
		os.Setenv("AWS_LAMBDA_FUNCTION_NAME", originalLambda)
		os.Setenv("FUNCTION_NAME", originalFunction)
		os.Setenv("AZURE_FUNCTIONS_ENVIRONMENT", originalAzure)
	}()

	// Test normal environment
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
	os.Unsetenv("FUNCTION_NAME")
	os.Unsetenv("AZURE_FUNCTIONS_ENVIRONMENT")
	if IsEphemeralEnvironment() {
		t.Error("Should not be ephemeral in normal environment")
	}

	// Test AWS Lambda
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function")
	if !IsEphemeralEnvironment() {
		t.Error("Should be ephemeral in AWS Lambda")
	}
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	// Test Google Cloud Functions
	os.Setenv("FUNCTION_NAME", "test-function")
	if !IsEphemeralEnvironment() {
		t.Error("Should be ephemeral in Google Cloud Functions")
	}
	os.Unsetenv("FUNCTION_NAME")

	// Test Azure Functions
	os.Setenv("AZURE_FUNCTIONS_ENVIRONMENT", "Development")
	if !IsEphemeralEnvironment() {
		t.Error("Should be ephemeral in Azure Functions")
	}
}

func TestDetectPlatform(t *testing.T) {
	platform := DetectPlatform()

	// Platform should be one of the expected values
	validPlatforms := []string{"aws", "gcp", "azure", "kubernetes", "local"}
	found := slices.Contains(validPlatforms, string(platform))

	if !found {
		t.Errorf("Platform %s is not a valid platform", platform)
	}
}

// Benchmark the detection function
func BenchmarkDetectEnvironmentConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DetectEnvironmentConfig()
	}
}
