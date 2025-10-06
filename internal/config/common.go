package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	yamlSig "sigs.k8s.io/yaml"
)

// loadDataFromSource loads data from either a file path or URL
func loadDataFromSource[
	T WorkflowDefinitions | RoleDefinitions | ProviderDefinitions,
](path string, uriEndpoint *model.Endpoint, data string, definition T) ([]*T, error) {

	// Prioritize path over URL if both are provided
	if len(data) > 0 {

		logrus.Debugln("Loading definitions from data")

		item, err := readData([]byte(data), definition)

		if err != nil {
			logrus.WithFields(logrus.Fields{
				"data": data,
			}).WithError(err).Errorln("Error reading data from string")
			return nil, fmt.Errorf("failed to read data from string: %w", err)
		}

		return []*T{item}, nil

	} else if uriEndpoint != nil {

		uri := model.HTTPArguments{
			Method:   http.MethodGet,
			Endpoint: uriEndpoint,
		}

		logrus.WithFields(logrus.Fields{
			"url": uri.Endpoint.String(),
		}).Debugln("Loading definitions from Url")

		// Load from URL using Resty
		resp, err := common.InvokeHttpRequest(&uri)

		if err != nil {
			logrus.WithFields(logrus.Fields{
				"url": uri.Endpoint.String(),
			}).WithError(err).Errorln("Failed to fetch from URL")
			return nil, fmt.Errorf("failed to fetch from URL %s: %w", uri.Endpoint.String(), err)
		}

		if resp.StatusCode() != http.StatusOK {
			logrus.WithFields(logrus.Fields{
				"url":    uri.Endpoint.String(),
				"status": resp.StatusCode(),
			}).Errorln("Failed to fetch from URL")
			return nil, fmt.Errorf("failed to fetch from URL %s: status %d", uri.Endpoint.String(), resp.StatusCode())
		}

		data := resp.Body()

		item, err := readData(data, definition)

		if err != nil {
			logrus.WithFields(logrus.Fields{
				"url":  uri.Endpoint.String(),
				"data": string(data),
			}).WithError(err).Errorln("Error reading data from URL")
			return nil, fmt.Errorf("failed to read data from URL %s: %w", uri.Endpoint.String(), err)
		}

		return []*T{item}, nil

	} else if len(path) > 0 {

		logrus.WithFields(logrus.Fields{
			"path": path,
		}).Debugln("Loading definitions from file path")

		// Check if path is a directory
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			// Load all YAML and JSON files from directory
			return loadFromDirectory(path, definition)
		}

		// Convert to single array item
		item, err := readFile(path, definition)

		if err != nil {
			logrus.WithError(err).Errorln("Failed to read file")
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}

		return []*T{item}, nil

	}

	return nil, fmt.Errorf("either path or url must be provided")
}

// loadFromDirectory loads and merges all YAML and JSON files from a directory
func loadFromDirectory[T WorkflowDefinitions | RoleDefinitions | ProviderDefinitions](
	dirPath string, definition T,
) ([]*T, error) {

	results := make([]*T, 0)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(info.Name()), ".yaml") && !strings.HasSuffix(strings.ToLower(info.Name()), ".yml") && !strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			return nil // Skip non-YAML/JSON files
		}

		item, err := readFile(path, definition)

		if err != nil {
			return err
		}

		results = append(results, item)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil

}

func readFile[T WorkflowDefinitions | RoleDefinitions | ProviderDefinitions](
	path string, definition T,
) (*T, error) {

	ext := strings.ToLower(filepath.Ext(path))

	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return nil, fmt.Errorf("unsupported file extension (yaml, json): %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return readData(data, definition)
}

func readData[T WorkflowDefinitions | RoleDefinitions | ProviderDefinitions](
	data []byte, _ T) (*T, error) {

	var item T

	// remove all starting whitespace including newlines to figure out
	// what the first character is
	data = bytes.TrimLeftFunc(data, unicode.IsSpace)

	if len(data) == 0 {
		return nil, fmt.Errorf("no data provided")
	} else if data[0] == '{' || data[1] == '[' {
		// If JSON we can unmarshal directly
		logrus.Debugln("Data format detected: JSON")
	} else {
		// If YAML we need to convert to JSON after
		if yamlData, err := yamlSig.YAMLToJSON(data); err != nil {
			logrus.WithError(err).Errorln("Failed to convert YAML to JSON")
			return nil, err
		} else {
			data = yamlData
		}
	}

	if err := json.Unmarshal(data, &item); err != nil {
		logrus.WithError(err).Errorln("Failed to unmarshal JSON data")
		return nil, fmt.Errorf("failed to unmarshal JSON file: %w", err)
	}

	return &item, nil
}
