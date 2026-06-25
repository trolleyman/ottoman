package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestExampleControllerConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "controller.toml"))
	if err != nil {
		t.Fatalf("failed to read controller.toml: %v", err)
	}

	var cfg map[string]any
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse controller.toml: %v", err)
	}

	// Verify controller section exists
	if _, ok := cfg["controller"]; !ok {
		t.Error("controller.toml should have [controller] section")
	}

	// Verify agent section does NOT exist
	if _, ok := cfg["agent"]; ok {
		t.Error("controller.toml should NOT have [agent] section")
	}

	// Check for unknown top-level keys
	validKeys := map[string]bool{"controller": true, "agent": true}
	for key := range cfg {
		if !validKeys[key] {
			t.Errorf("controller.toml has unknown top-level key: %s", key)
		}
	}
}

func TestExampleAgentConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "agent.toml"))
	if err != nil {
		t.Fatalf("failed to read agent.toml: %v", err)
	}

	var cfg map[string]any
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse agent.toml: %v", err)
	}

	// Verify agent section exists
	if _, ok := cfg["agent"]; !ok {
		t.Error("agent.toml should have [agent] section")
	}

	// Verify controller section does NOT exist
	if _, ok := cfg["controller"]; ok {
		t.Error("agent.toml should NOT have [controller] section")
	}

	// Check for unknown top-level keys
	validKeys := map[string]bool{"controller": true, "agent": true}
	for key := range cfg {
		if !validKeys[key] {
			t.Errorf("agent.toml has unknown top-level key: %s", key)
		}
	}
}

func TestExampleConfigsLoadWithViper(t *testing.T) {
	// Test that example configs can be loaded by the actual config system
	examples := []struct {
		name          string
		file          string
		hasController bool
		hasAgent      bool
	}{
		{"controller.toml", filepath.Join("..", "..", "examples", "controller.toml"), true, false},
		{"agent.toml", filepath.Join("..", "..", "examples", "agent.toml"), false, true},
	}

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			Init(ex.file)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("failed to load %s: %v", ex.name, err)
			}

			if ex.hasController && cfg.Controller.ListenAddress == "" {
				t.Error("controller.listen_addr should be set")
			}
			if ex.hasAgent && cfg.Agent.ListenAddress == "" {
				t.Error("agent.listen_addr should be set")
			}
		})
	}
}
