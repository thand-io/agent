package examples

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelloWorldTemporalWorkflow(t *testing.T) {
	t.Run("HelloWorldTemporal function returns Hello, World!", func(t *testing.T) {
		// Call the public HelloWorldTemporal function
		output := HelloWorldTemporal()

		// Validate the output
		require.NotNil(t, output, "Output should not be nil")

		outputMap, ok := output.(map[string]any)
		require.True(t, ok, "Output should be a map")

		// Verify greeting message
		greeting, exists := outputMap["greeting"]
		require.True(t, exists, "Output should contain 'greeting' field")
		assert.Equal(t, "Hello, World!", greeting, "Greeting should be 'Hello, World!'")
	})
}
