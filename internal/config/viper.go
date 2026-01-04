package config

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
)

// stringToEndpointHookFunc returns a mapstructure decode hook that converts strings to model.Endpoint
func stringToEndpointHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		// Only process if target type is model.Endpoint
		if t != reflect.TypeOf(model.Endpoint{}) && t != reflect.TypeOf(&model.Endpoint{}) {
			return data, nil
		}

		// If data is a string, convert it to model.Endpoint using JSON unmarshaling
		if f.Kind() == reflect.String {
			str, ok := data.(string)
			if !ok {
				return data, nil
			}

			var endpoint model.Endpoint
			// Use JSON unmarshal to leverage the custom UnmarshalJSON in model.Endpoint
			if err := json.Unmarshal([]byte(fmt.Sprintf("%q", str)), &endpoint); err != nil {
				return nil, fmt.Errorf("failed to convert string to Endpoint: %w", err)
			}

			if t == reflect.TypeOf(&model.Endpoint{}) {
				return &endpoint, nil
			}
			return endpoint, nil
		}

		return data, nil
	}
}
