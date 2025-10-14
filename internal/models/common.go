package models

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
)

var ENCODED_WORKFLOW_TASK = "workflow_task"
var ENCODED_AUTH = "auth"
var ENCODED_SESSION = "session"
var ENCODED_SESSION_LOCAL = "session_local"

type EncodingWrapper struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier,omitempty"`
	Data       any    `json:"data"`
}

func (e EncodingWrapper) EncodeAndEncrypt(encryptor EncryptionImpl) string {
	return e.encode(encryptor)
}

func (e EncodingWrapper) Encode() string {
	return e.encode()
}

func (e EncodingWrapper) encode(modifiers ...EncryptionImpl) string {

	// encode workflow to JSON
	data, err := json.Marshal(e)

	if err != nil {
		panic(err)
	}

	// Gzipped and compressed with HPack
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, _ = writer.Write(data)
	_ = writer.Close()

	finalData := compressed.Bytes()

	if len(modifiers) > 0 {

		for _, encryptor := range modifiers {
			// Now encrypt data
			encryptedData, err := encryptor.Encrypt(finalData)

			if err != nil {
				panic(err)
			}

			finalData = encryptedData
		}

	}

	// base64 encode the data
	encoded := base64.StdEncoding.EncodeToString(finalData)

	return encoded
}

func (e EncodingWrapper) DecodeAndDecrypt(input string, decryptor EncryptionImpl) (*EncodingWrapper, error) {
	return e.decode(input, decryptor)
}

func (e EncodingWrapper) Decode(input string) (*EncodingWrapper, error) {
	return e.decode(input)
}

func (e EncodingWrapper) decode(input string, modifiers ...EncryptionImpl) (*EncodingWrapper, error) {

	decoded, err := base64.StdEncoding.DecodeString(input)

	if err != nil {
		return nil, err
	}

	decodedData := decoded

	if len(modifiers) > 0 {

		for _, decryptor := range modifiers {
			// Now decrypt data
			decryptedData, err := decryptor.Decrypt(decodedData)

			if err != nil {
				return nil, err
			}

			decodedData = decryptedData
		}
	}

	// de-compress
	// Gzipped and compressed with HPack
	reader, err := zlib.NewReader(bytes.NewReader(decodedData))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var uncompressed bytes.Buffer
	_, err = io.Copy(&uncompressed, reader)
	if err != nil {
		return nil, err
	}

	// decode JSON
	if err := json.Unmarshal(uncompressed.Bytes(), &e); err != nil {
		return nil, err
	}
	return &e, nil
}

type BasicConfig map[string]any

func (pc *BasicConfig) GetString(key string) (string, bool) {
	if pc == nil {
		return "", false
	}
	if value, ok := (*pc)[key]; ok {
		if strValue, ok := value.(string); ok {
			return strValue, true
		}
	}
	return "", false
}

func (pc *BasicConfig) GetInt(key string) (int, bool) {
	if pc == nil {
		return 0, false
	}
	if value, ok := (*pc)[key]; ok {
		if intValue, ok := value.(int); ok {
			return intValue, true
		}
	}
	return 0, false
}

func (pc *BasicConfig) GetFloat(key string) (float64, bool) {
	if pc == nil {
		return 0, false
	}
	if value, ok := (*pc)[key]; ok {
		if floatValue, ok := value.(float64); ok {
			return floatValue, true
		} else if intValue, ok := value.(int); ok {
			return float64(intValue), true
		} else if stringValue, ok := value.(string); ok {
			if floatValue, err := strconv.ParseFloat(stringValue, 64); err == nil {
				return floatValue, true
			}
		}
	}
	return 0, false
}

func (pc *BasicConfig) GetBool(key string) (bool, bool) {
	if pc == nil {
		return false, false
	}
	if value, ok := (*pc)[key]; ok {
		if boolValue, ok := value.(bool); ok {
			return boolValue, true
		}
	}
	return false, false
}

func (pc *BasicConfig) GetStringWithDefault(key string, defaultValue string) string {
	if pc == nil {
		return defaultValue
	}
	if value, ok := (*pc)[key]; ok {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return defaultValue
}

func (pc *BasicConfig) GetMap(key string) (map[string]any, bool) {
	if pc == nil {
		return nil, false
	}
	if value, ok := (*pc)[key]; ok {
		if mapValue, ok := value.(map[string]any); ok {
			return mapValue, true
		}
	}
	return nil, false
}

func (pc *BasicConfig) GetStringSlice(key string) ([]string, bool) {
	if pc == nil {
		return nil, false
	}
	if value, ok := (*pc)[key]; ok {
		if sliceValue, ok := value.([]string); ok {
			return sliceValue, true
		}
	}
	return nil, false
}
