package agent

import (
	"github.com/trolleyman/ottoman/internal/config"
)

// Config is an alias for config.AgentConfig
type Config = config.AgentConfig

// LoadConfig loads agent configuration using Viper
func LoadConfig(path string) (*Config, error) {
	config.Init(path)
	return config.GetAgent()
}
