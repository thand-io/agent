package models

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

// ResponseMeta contains pagination metadata
type ResponseMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type EncodingWrapper struct {
	Type string `json:"type"`
	// The Identifier field is temporarily disabled for debugging purposes.
	// Restore this field if identifier tracking is required in future updates.
	//Identifier string `json:"identifier,omitempty"`
	Data any `json:"data"`
}

func NewEncodingWrapper(typeName string, data any) *EncodingWrapper {
	return &EncodingWrapper{
		Type: typeName,
		Data: data,
	}
}

func NewDecodingWrapper(typeName string) *EncodingWrapper {
	return &EncodingWrapper{
		Type: typeName,
	}
}

func (e EncodingWrapper) EncodeAndEncrypt(encryptor EncryptionImpl) string {
	return e.encodeBase64(encryptor)
}

func (e EncodingWrapper) EncodeBase64() string {
	return e.encodeBase64()
}

func (e EncodingWrapper) EncodeBytes() []byte {
	return e.encodeBytes()
}

func (e EncodingWrapper) encodeBase64(modifiers ...EncryptionImpl) string {
	finalData := e.encodeBytes(modifiers...)
	// base64 URL-safe encode the data (uses - and _ instead of + and / for URL safety)
	return base64.URLEncoding.EncodeToString(finalData)
}

func (e EncodingWrapper) encodeBytes(modifiers ...EncryptionImpl) []byte {

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
	ctx := context.Background()

	// Filter out empty modifiers
	modifiers = common.FilterEmpty(modifiers...)

	if len(modifiers) > 0 {

		for _, encryptor := range modifiers {

			if encryptor == nil {
				logrus.Warningln("No valid encryptor provided")
				continue
			}

			// Now encrypt data
			encryptedData, err := encryptor.Encrypt(ctx, finalData)

			if err != nil {
				panic(err)
			}

			finalData = encryptedData
		}

	}

	return finalData
}

func (e EncodingWrapper) DecodeAndDecrypt(input string, decryptor EncryptionImpl) (*EncodingWrapper, error) {
	return e.decode(input, decryptor)
}

func (e EncodingWrapper) Decode(input string) (*EncodingWrapper, error) {
	return e.decode(input)
}

func (e EncodingWrapper) DecodeBytes(input []byte) (*EncodingWrapper, error) {
	return e.decodeBytes(input)
}

func (e EncodingWrapper) decode(input string, modifiers ...EncryptionImpl) (*EncodingWrapper, error) {
	// Use URL-safe base64 decoding (uses - and _ instead of + and / for URL safety)
	decoded, err := base64.URLEncoding.DecodeString(input)

	// For backward compatibility: if URL-safe decoding fails, try standard base64 decoding
	// This ensures existing tokens encoded with standard base64 (containing + or /) still work
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(input)
		if err != nil {
			return nil, err
		}
	}

	return e.decodeBytes(decoded, modifiers...)
}

func (e EncodingWrapper) decodeBytes(input []byte, modifiers ...EncryptionImpl) (*EncodingWrapper, error) {
	decodedData := input
	ctx := context.Background()

	// Filter out empty modifiers
	modifiers = common.FilterEmpty(modifiers...)

	if len(modifiers) > 0 {

		for _, decryptor := range modifiers {

			if decryptor == nil {
				logrus.Warningln("No valid decryptor provided")
				continue
			}

			// Now decrypt data
			decryptedData, err := decryptor.Decrypt(ctx, decodedData)

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

	// decode from JSON
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

func (pc *BasicConfig) HasString(key string) bool {
	if pc == nil {
		return false
	}
	_, ok := (*pc)[key]
	return ok
}

func (pc *BasicConfig) GetInt(key string) (int, bool) {
	if pc == nil {
		return 0, false
	}
	if value, ok := (*pc)[key]; ok {
		if intValue, ok := value.(int); ok {
			return intValue, true
		} else if floatValue, ok := value.(float64); ok {
			return int(floatValue), true
		} else if stringValue, ok := value.(string); ok {
			if intValue, err := strconv.Atoi(stringValue); err == nil {
				return intValue, true
			}
		}
	}
	return 0, false
}

func (pc *BasicConfig) GetIntWithDefault(key string, defaultValue int) int {
	if pc == nil {
		return defaultValue
	}
	if value, ok := pc.GetInt(key); ok {
		return value
	}
	return defaultValue
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
		} else if stringValue, ok := value.(string); ok {
			if parsed, err := strconv.ParseBool(stringValue); err == nil {
				return parsed, true
			}
		} else if intValue, ok := value.(int); ok {
			return intValue != 0, true
		} else if floatValue, ok := value.(float64); ok {
			return floatValue != 0, true
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

func (pc *BasicConfig) AsMap() map[string]any {
	if pc == nil {
		return map[string]any{}
	}
	return map[string]any(*pc)
}

func (pc *BasicConfig) SetKeyWithValue(key string, value any) {
	if pc == nil {
		return
	}
	if *pc == nil {
		*pc = BasicConfig{}
	}
	(*pc)[key] = value
}

func (pc *BasicConfig) Update(updateMap map[string]any) {
	if pc == nil {
		return
	}
	if *pc == nil {
		*pc = BasicConfig{}
	}
	maps.Copy((*pc), updateMap)
}

// ConvertVersionToString converts a version field from any type to a string.
// This is a helper function for UnmarshalJSON and UnmarshalYAML methods
// to handle version fields that may be parsed as different types (string, int, float64, etc.)
func ConvertVersionToString(versionAny any) string {
	switch v := versionAny.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%g", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		if v != nil {
			return fmt.Sprintf("%v", v)
		}
		return "0.0.0"
	}
}
