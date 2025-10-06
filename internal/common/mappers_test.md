package common

import (
	"testing"
)

func TestConvertMapToInterface(t *testing.T) {
	t.Run("successful conversion to struct", func(t *testing.T) {
		type TestStruct struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		input := map[string]any{
			"name": "John Doe",
			"age":  30,
		}

		var result TestStruct
		err := ConvertMapToInterface(input, &result)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if result.Name != "John Doe" {
			t.Errorf("Expected name 'John Doe', got '%s'", result.Name)
		}

		if result.Age != 30 {
			t.Errorf("Expected age 30, got %d", result.Age)
		}
	})

	t.Run("conversion with nested objects", func(t *testing.T) {
		type Address struct {
			Street string `json:"street"`
			City   string `json:"city"`
		}
		type Person struct {
			Name    string  `json:"name"`
			Address Address `json:"address"`
		}

		input := map[string]any{
			"name": "Jane Doe",
			"address": map[string]any{
				"street": "123 Main St",
				"city":   "New York",
			},
		}

		var result Person
		err := ConvertMapToInterface(input, &result)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if result.Name != "Jane Doe" {
			t.Errorf("Expected name 'Jane Doe', got '%s'", result.Name)
		}

		if result.Address.Street != "123 Main St" {
			t.Errorf("Expected street '123 Main St', got '%s'", result.Address.Street)
		}

		if result.Address.City != "New York" {
			t.Errorf("Expected city 'New York', got '%s'", result.Address.City)
		}
	})

	t.Run("conversion with type mismatch", func(t *testing.T) {
		type TestStruct struct {
			Age int `json:"age"`
		}

		input := map[string]any{
			"age": "not a number", // string instead of int
		}

		var result TestStruct
		err := ConvertMapToInterface(input, &result)

		if err == nil {
			t.Error("Expected an error due to type mismatch, got nil")
		}
	})

	t.Run("conversion with invalid JSON data", func(t *testing.T) {
		// Create a map with a value that cannot be marshaled to JSON
		input := map[string]any{
			"invalid": make(chan int), // channels cannot be marshaled to JSON
		}

		var result map[string]any
		err := ConvertMapToInterface(input, &result)

		if err == nil {
			t.Error("Expected an error due to invalid JSON data, got nil")
		}
	})
}

func TestConvertInterfaceToInterface(t *testing.T) {
	t.Run("successful conversion between structs", func(t *testing.T) {
		type SourceStruct struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		type TargetStruct struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		source := SourceStruct{
			Name: "John Doe",
			Age:  30,
		}

		var target TargetStruct
		err := ConvertInterfaceToInterface(source, &target)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if target.Name != "John Doe" {
			t.Errorf("Expected name 'John Doe', got '%s'", target.Name)
		}

		if target.Age != 30 {
			t.Errorf("Expected age 30, got %d", target.Age)
		}
	})

	t.Run("conversion with different field names", func(t *testing.T) {
		type SourceStruct struct {
			Name string `json:"full_name"`
			Age  int    `json:"age"`
		}

		type TargetStruct struct {
			FullName string `json:"full_name"`
			Age      int    `json:"age"`
		}

		source := SourceStruct{
			Name: "Jane Doe",
			Age:  25,
		}

		var target TargetStruct
		err := ConvertInterfaceToInterface(source, &target)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if target.FullName != "Jane Doe" {
			t.Errorf("Expected full name 'Jane Doe', got '%s'", target.FullName)
		}

		if target.Age != 25 {
			t.Errorf("Expected age 25, got %d", target.Age)
		}
	})

	t.Run("conversion from map to struct", func(t *testing.T) {
		type TargetStruct struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		source := map[string]any{
			"name": "Bob Smith",
			"age":  35,
		}

		var target TargetStruct
		err := ConvertInterfaceToInterface(source, &target)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if target.Name != "Bob Smith" {
			t.Errorf("Expected name 'Bob Smith', got '%s'", target.Name)
		}

		if target.Age != 35 {
			t.Errorf("Expected age 35, got %d", target.Age)
		}
	})

	t.Run("conversion with invalid source data", func(t *testing.T) {
		// Create source with data that cannot be marshaled to JSON
		source := map[string]any{
			"invalid": make(chan int), // channels cannot be marshaled to JSON
		}

		var target map[string]any
		err := ConvertInterfaceToInterface(source, &target)

		if err == nil {
			t.Error("Expected an error due to invalid JSON data, got nil")
		}
	})
}

func TestConvertToSnakeCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple camelCase",
			input:    "camelCase",
			expected: "camelcase",
		},
		{
			name:     "PascalCase",
			input:    "PascalCase",
			expected: "pascalcase",
		},
		{
			name:     "already snake_case",
			input:    "already_snake_case",
			expected: "already_snake_case",
		},
		{
			name:     "with spaces",
			input:    "hello world test",
			expected: "hello_world_test",
		},
		{
			name:     "multiple spaces",
			input:    "hello    world    test",
			expected: "hello_world_test",
		},
		{
			name:     "with allowed special characters",
			input:    "test_name+=value,.@domain-com",
			expected: "test_name+=value,.@domain-com",
		},
		{
			name:     "with numbers",
			input:    "test123Name456",
			expected: "test123name456",
		},
		{
			name:     "with invalid special characters",
			input:    "test#name$value%",
			expected: "testnamevalue",
		},
		{
			name:     "mixed case with spaces and special chars",
			input:    "My Test Name@domain.com",
			expected: "my_test_name@domain.com",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "starting with space",
			input:    " hello world",
			expected: "hello_world",
		},
		{
			name:     "ending with space",
			input:    "hello world ",
			expected: "hello_world_",
		},
		{
			name:     "only special characters (allowed)",
			input:    "_+=,.@-",
			expected: "_+=,.@-",
		},
		{
			name:     "only special characters (not allowed)",
			input:    "#$%^&*",
			expected: "",
		},
		{
			name:     "unicode characters",
			input:    "héllo wörld",
			expected: "héllo_wörld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToSnakeCase(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertToSnakeCase(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance evaluation
func BenchmarkConvertMapToInterface(b *testing.B) {
	type TestStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	input := map[string]any{
		"name": "John Doe",
		"age":  30,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result TestStruct
		ConvertMapToInterface(input, &result)
	}
}

func BenchmarkConvertInterfaceToInterface(b *testing.B) {
	type SourceStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	type TargetStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	source := SourceStruct{
		Name: "John Doe",
		Age:  30,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var target TargetStruct
		ConvertInterfaceToInterface(source, &target)
	}
}

func BenchmarkConvertToSnakeCase(b *testing.B) {
	input := "MyTestNameWithSeveralWords"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertToSnakeCase(input)
	}
}
