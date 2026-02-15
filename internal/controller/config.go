package controller

import (
	"github.com/trolleyman/ottoman/internal/config"
)

// Config is an alias for config.Controlleronfig
type Config = config.ControllerConfig

// LoadConfig loads controller configuration using Viper
func LoadConfig(path string) (*Config, error) {
	config.Init(path)
	return config.GetController()
}
