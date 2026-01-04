package config

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// loadDataFromSource loads data from either a file path or URL
func loadDataFromSource[
	T models.WorkflowDefinitions | models.RoleDefinitions | models.ProviderDefinitions,
](path string, uriEndpoint *model.Endpoint, data string, definition T) ([]*T, error) {

	// Prioritize path over URL if both are provided
	if len(data) > 0 {

		logrus.Debugln("Loading definitions from data")

		item, err := common.ReadDataToInterface([]byte(data), definition)

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

		data := resp.Body()

		if resp.StatusCode() != http.StatusOK {
			logrus.WithFields(logrus.Fields{
				"url":    uri.Endpoint.String(),
				"status": resp.StatusCode(),
				"body":   string(data),
			}).Errorln("Failed to fetch from URL")
			return nil, fmt.Errorf("failed to fetch from URL %s: status %d", uri.Endpoint.String(), resp.StatusCode())
		}

		item, err := common.ReadDataToInterface(data, definition)

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

		info, err := os.Stat(path)

		if err != nil {

			if os.IsNotExist(err) {
				logrus.WithFields(logrus.Fields{
					"path": path,
				}).Errorln("File or directory does not exist")
				// Return empty slice if path does not exist
				return []*T{}, nil
			}

			// Handle other errors (permission denied, etc.)
			logrus.WithError(err).WithFields(logrus.Fields{
				"path": path,
			}).Errorln("Failed to stat path")

			return nil, fmt.Errorf("failed to stat path %s: %w", path, err)
		}

		// Check if path is a directory
		if info.Mode().IsDir() {

			// Load all YAML and JSON files from directory
			return loadFromDirectory(path, definition)

		} else if info.Mode().IsRegular() {

			// Convert to single array item
			item, err := readFile(path, definition)

			if err != nil {
				logrus.WithError(err).Errorln("Failed to read file")
				return nil, fmt.Errorf("failed to read file %s: %w", path, err)
			}

			return []*T{item}, nil

		} else {

			logrus.WithFields(logrus.Fields{
				"path": path,
			}).Errorln("Path is neither a file nor a directory")

			return []*T{}, nil
		}

	}

	return nil, fmt.Errorf("either path or url must be provided")
}

// loadFromDirectory loads and merges all YAML and JSON files from a directory
func loadFromDirectory[T models.WorkflowDefinitions | models.RoleDefinitions | models.ProviderDefinitions](
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

			logrus.WithFields(logrus.Fields{
				"path": path,
			}).WithError(err).Errorln("Failed to read file in directory")

			return err
		}

		results = append(results, item)
		return nil
	})

	if err != nil {

		logrus.WithFields(logrus.Fields{
			"path": dirPath,
		}).WithError(err).Errorln("Failed to walk directory")

		return nil, err
	}

	return results, nil

}

func readFile[T models.WorkflowDefinitions | models.RoleDefinitions | models.ProviderDefinitions](
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

	return common.ReadDataToInterface(data, definition)
}
