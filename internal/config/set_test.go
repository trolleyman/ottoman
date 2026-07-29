package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempConfig drops content at a temp path and returns it.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

// loadFrom reads path through the normal config machinery.
func loadFrom(t *testing.T, path string) *Config {
	t.Helper()
	Init(path)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("loading %s: %v", path, err)
	}
	return cfg
}

const sampleConfig = `[agent]
listen_address = ":17294"
auth_token = "0123456789abcdef0123"

[agent.trackpad]
sensitivity = 1.5
friction = 0.92

[controller]
listen_address = ":17293"
auth_token = "0123456789abcdef0123"

[controller.agent]
mac_address = "aa:bb:cc:dd:ee:ff"
ip_address = "192.168.1.50"
port = 17294
`

func TestSetValueLeavesEverythingElseAlone(t *testing.T) {
	path := writeTempConfig(t, sampleConfig)

	if _, _, err := SetValue(path, "agent.listen_address", "127.0.0.1:17294"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	cfg := loadFrom(t, path)
	if cfg.Agent.ListenAddress != "127.0.0.1:17294" {
		t.Errorf("agent.listen_address = %q, want 127.0.0.1:17294", cfg.Agent.ListenAddress)
	}
	// The point of `config set`: one key changes, the rest of the file survives.
	if cfg.Agent.AuthToken != "0123456789abcdef0123" {
		t.Errorf("agent.auth_token = %q, want it untouched", cfg.Agent.AuthToken)
	}
	if cfg.Agent.Trackpad.Sensitivity != 1.5 || cfg.Agent.Trackpad.Friction != 0.92 {
		t.Errorf("trackpad tuning = %+v, want it untouched", cfg.Agent.Trackpad)
	}
	if cfg.Controller.ListenAddress != ":17293" || cfg.Controller.Agent.MACAddress != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("controller section = %+v, want it untouched", cfg.Controller)
	}
}

func TestSetValueTypedValues(t *testing.T) {
	path := writeTempConfig(t, sampleConfig)

	if _, _, err := SetValue(path, "agent.require_local_auth", "false"); err != nil {
		t.Fatalf("SetValue(require_local_auth): %v", err)
	}

	// A boolean has to land as a TOML boolean, not a quoted string that
	// unmarshals back to nil - which would silently take the default.
	cfg := loadFrom(t, path)
	if cfg.Agent.RequireLocalAuth == nil || *cfg.Agent.RequireLocalAuth {
		t.Errorf("agent.require_local_auth = %v, want an explicit false", cfg.Agent.RequireLocalAuth)
	}
	if cfg.Agent.LocalAuthRequired() {
		t.Error("LocalAuthRequired() = true, want the configured false")
	}
}

func TestSetValueRejectsBadInput(t *testing.T) {
	path := writeTempConfig(t, sampleConfig)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"unknown key", "agent.token", "0123456789abcdef0123"},
		{"port only", "agent.listen_address", "17294"},
		{"hostname", "agent.listen_address", "hades:17294"},
		{"port out of range", "agent.listen_address", ":99999"},
		{"short token", "agent.auth_token", "hunter2"},
		{"non-boolean", "agent.require_local_auth", "yes please"},
		{"schemeless url", "controller.agent.url", "hades.tail1234.ts.net"},
		{"unsupported scheme", "controller.agent.url", "ftp://hades"},
		{"bad mac", "controller.agent.mac_address", "not-a-mac"},
		{"key nothing reads", "agent.trackpad.friction", "0.8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := SetValue(path, tc.key, tc.value); err == nil {
				t.Fatalf("SetValue(%q, %q) succeeded, want a validation error", tc.key, tc.value)
			}
			// A refused value must not have been half-written.
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading config: %v", err)
			}
			if string(after) != string(before) {
				t.Error("the config file changed despite the error")
			}
		})
	}
}

func TestSetAgentURLRetiresLegacyKeys(t *testing.T) {
	path := writeTempConfig(t, sampleConfig)

	if _, _, err := SetValue(path, "controller.agent.url", "https://hades.tail1234.ts.net"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	// The legacy pair would still parse, and a stale ip_address left next to a
	// new url is exactly the kind of disagreement this avoids.
	if strings.Contains(string(content), "ip_address") || strings.Contains(string(content), "192.168.1.50") {
		t.Errorf("legacy ip_address survived:\n%s", content)
	}

	cfg := loadFrom(t, path)
	if got := cfg.Controller.Agent.resolvedURL(); got != "https://hades.tail1234.ts.net" {
		t.Errorf("agent URL = %q, want the one just set", got)
	}
	if cfg.Controller.Agent.MACAddress != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("mac_address = %q, want it untouched", cfg.Controller.Agent.MACAddress)
	}
}

func TestSetValuePreservesFileMode(t *testing.T) {
	path := writeTempConfig(t, sampleConfig)
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if _, _, err := SetValue(path, "agent.listen_address", "127.0.0.1:17294"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	// The file holds the auth token; a rewrite must not widen it.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func TestSetValueCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")

	if _, _, err := SetValue(path, "agent.auth_token", "0123456789abcdef0123"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if cfg := loadFrom(t, path); cfg.Agent.AuthToken != "0123456789abcdef0123" {
		t.Errorf("agent.auth_token = %q, want the one just set", cfg.Agent.AuthToken)
	}
}

func TestRotateTokenUpdatesEveryPresentSection(t *testing.T) {
	path := writeTempConfig(t, sampleConfig)

	rotation, err := RotateToken(path, "")
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if rotation.Token == "0123456789abcdef0123" || len(rotation.Token) < 32 {
		t.Errorf("generated token %q looks wrong", rotation.Token)
	}
	if len(rotation.Keys) != 2 {
		t.Errorf("rotated %v, want both agent and controller tokens", rotation.Keys)
	}

	// Both components must end up presenting the same token, or the controller
	// can no longer talk to the agent.
	cfg := loadFrom(t, path)
	if cfg.Agent.AuthToken != rotation.Token || cfg.Controller.AuthToken != rotation.Token {
		t.Errorf("agent %q / controller %q, want both = %q", cfg.Agent.AuthToken, cfg.Controller.AuthToken, rotation.Token)
	}
	if cfg.Agent.ListenAddress != ":17294" {
		t.Errorf("agent.listen_address = %q, want it untouched", cfg.Agent.ListenAddress)
	}
}

func TestRotateTokenOnlyTouchesConfiguredSections(t *testing.T) {
	path := writeTempConfig(t, `[agent]
listen_address = ":17294"
auth_token = "0123456789abcdef0123"
`)

	rotation, err := RotateToken(path, "")
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if len(rotation.Keys) != 1 || rotation.Keys[0] != "agent.auth_token" {
		t.Errorf("rotated %v, want only agent.auth_token", rotation.Keys)
	}

	// An agent-only config must not sprout half a controller.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if strings.Contains(string(content), "controller") {
		t.Errorf("a controller section appeared:\n%s", content)
	}
}

func TestRotateTokenAcceptsAGivenToken(t *testing.T) {
	path := writeTempConfig(t, sampleConfig)

	if _, err := RotateToken(path, "short"); err == nil {
		t.Error("RotateToken accepted a 5-character token")
	}

	rotation, err := RotateToken(path, "supplied-token-0123456789")
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if cfg := loadFrom(t, path); cfg.Agent.AuthToken != "supplied-token-0123456789" || rotation.Token != cfg.Agent.AuthToken {
		t.Errorf("agent.auth_token = %q, want the supplied token", cfg.Agent.AuthToken)
	}
}

func TestRotateTokenNeedsAConfiguredComponent(t *testing.T) {
	path := writeTempConfig(t, "# nothing configured yet\n")

	if _, err := RotateToken(path, ""); err == nil {
		t.Error("RotateToken succeeded on a config with no agent or controller section")
	}
}

func TestNewConfigFileIsNotWorldReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	if _, _, err := SetValue(path, "agent.auth_token", "0123456789abcdef0123"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	// The file holds the shared token; viper's default 0644 would hand it to
	// every other local user.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func TestSaveAgentIsNotWorldReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	if err := SaveAgent(&AgentConfig{ListenAddress: ":17294", AuthToken: "0123456789abcdef0123"}, path); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("mode = %o, want 600", got)
	}
}
