package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestExampleServerConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "server.toml"))
	if err != nil {
		t.Fatalf("failed to read server.toml: %v", err)
	}

	var cfg map[string]interface{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse server.toml: %v", err)
	}

	// Verify server section exists
	if _, ok := cfg["server"]; !ok {
		t.Error("server.toml should have [server] section")
	}

	// Verify client section does NOT exist
	if _, ok := cfg["client"]; ok {
		t.Error("server.toml should NOT have [client] section")
	}

	// Check for unknown top-level keys
	validKeys := map[string]bool{"server": true, "client": true}
	for key := range cfg {
		if !validKeys[key] {
			t.Errorf("server.toml has unknown top-level key: %s", key)
		}
	}
}

func TestExampleClientConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "client.toml"))
	if err != nil {
		t.Fatalf("failed to read client.toml: %v", err)
	}

	var cfg map[string]interface{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse client.toml: %v", err)
	}

	// Verify client section exists
	if _, ok := cfg["client"]; !ok {
		t.Error("client.toml should have [client] section")
	}

	// Verify server section does NOT exist
	if _, ok := cfg["server"]; ok {
		t.Error("client.toml should NOT have [server] section")
	}

	// Check for unknown top-level keys
	validKeys := map[string]bool{"server": true, "client": true}
	for key := range cfg {
		if !validKeys[key] {
			t.Errorf("client.toml has unknown top-level key: %s", key)
		}
	}
}

func TestExampleConfigsLoadWithViper(t *testing.T) {
	// Test that example configs can be loaded by the actual config system
	examples := []struct {
		name     string
		file     string
		hasServer bool
		hasClient bool
	}{
		{"server.toml", filepath.Join("..", "..", "examples", "server.toml"), true, false},
		{"client.toml", filepath.Join("..", "..", "examples", "client.toml"), false, true},
	}

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			Init(ex.file)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("failed to load %s: %v", ex.name, err)
			}

			if ex.hasServer && cfg.Server.ListenAddr == "" {
				t.Error("server.listen_addr should be set")
			}
			if ex.hasClient && cfg.Client.ListenAddr == "" {
				t.Error("client.listen_addr should be set")
			}
		})
	}
}
