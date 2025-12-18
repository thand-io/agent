package models

type ThandConfig struct {
	Endpoint string `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint" default:"https://app.thand.io/"`
	Base     string `json:"base" yaml:"base" mapstructure:"base" default:"/"`    // Base path for login endpoint e.g. /
	ApiKey   string `json:"api_key" yaml:"api_key" mapstructure:"api_key"`       // The API key for authenticating with Thand.io
	Sync     bool   `json:"sync" yaml:"sync" mapstructure:"sync" default:"true"` // Whether to enable synchronization with Thand.io
}
