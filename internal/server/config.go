package server

import (
	"github.com/trolleyman/ottoman/internal/config"
)

// Config is an alias for config.ServerConfig
type Config = config.ServerConfig

// WakeTarget is an alias for config.WakeTarget
type WakeTarget = config.WakeTarget

// LoadConfig loads server configuration using Viper
func LoadConfig(path string) (*Config, error) {
	config.Init(path)
	return config.GetServer()
}
