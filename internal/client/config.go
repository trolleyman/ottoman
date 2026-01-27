package client

import (
	"github.com/trolleyman/ottoman/internal/config"
)

// Config is an alias for config.ClientConfig
type Config = config.ClientConfig

// LoadConfig loads client configuration using Viper
func LoadConfig(path string) (*Config, error) {
	config.Init(path)
	return config.GetClient()
}
