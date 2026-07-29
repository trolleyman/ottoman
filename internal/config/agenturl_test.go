package config

import "testing"

func TestAgentBaseURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  AgentControllerConfig
		want string
	}{
		{
			name: "nothing configured falls back to loopback",
			cfg:  AgentControllerConfig{},
			want: "http://127.0.0.1:17294",
		},
		{
			name: "legacy ip_address and port fold into an http url",
			cfg:  AgentControllerConfig{IPAddress: "192.168.0.100", Port: 17294},
			want: "http://192.168.0.100:17294",
		},
		{
			name: "legacy ip_address without a port assumes the agent's default",
			cfg:  AgentControllerConfig{IPAddress: "192.168.0.100"},
			want: "http://192.168.0.100:17294",
		},
		{
			name: "legacy ipv6 is bracketed",
			cfg:  AgentControllerConfig{IPAddress: "fd7a:115c:a1e0::139:d036", Port: 17294},
			want: "http://[fd7a:115c:a1e0::139:d036]:17294",
		},
		{
			name: "a magicdns name works as a host",
			cfg:  AgentControllerConfig{URL: "http://hades.tailc6ea62.ts.net:17294"},
			want: "http://hades.tailc6ea62.ts.net:17294",
		},
		{
			name: "https is carried through",
			cfg:  AgentControllerConfig{URL: "https://hades.tailc6ea62.ts.net:17294"},
			want: "https://hades.tailc6ea62.ts.net:17294",
		},
		{
			name: "url wins over the legacy pair",
			cfg: AgentControllerConfig{
				URL:       "https://hades.tailc6ea62.ts.net:17294",
				IPAddress: "192.168.0.100",
				Port:      17294,
			},
			want: "https://hades.tailc6ea62.ts.net:17294",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := tc.cfg.BaseURL()
			if err != nil {
				t.Fatalf("BaseURL() error: %v", err)
			}
			if got := u.String(); got != tc.want {
				t.Errorf("BaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAgentBaseURLRejectsUnusableValues(t *testing.T) {
	cases := []struct {
		name string
		cfg  AgentControllerConfig
	}{
		{"bare host with no scheme", AgentControllerConfig{URL: "hades.tailc6ea62.ts.net:17294"}},
		{"a scheme we cannot dial", AgentControllerConfig{URL: "ftp://hades:17294"}},
		{"websocket scheme belongs to the ws helper", AgentControllerConfig{URL: "ws://hades:17294"}},
		{"no host", AgentControllerConfig{URL: "http://"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.cfg.BaseURL(); err == nil {
				t.Fatalf("BaseURL(%q) succeeded, want an error", tc.cfg.URL)
			}
		})
	}
}

// Normalize is what makes `config init` write a file back out carrying only the
// modern key, so a round-trip can't leave two sources of truth disagreeing.
func TestNormalizeCollapsesLegacyKeys(t *testing.T) {
	cfg := AgentControllerConfig{IPAddress: "192.168.0.100", Port: 17294}
	cfg.Normalize()

	if cfg.URL != "http://192.168.0.100:17294" {
		t.Errorf("URL = %q, want the folded http url", cfg.URL)
	}
	if cfg.IPAddress != "" || cfg.Port != 0 {
		t.Errorf("legacy keys survived Normalize: ip=%q port=%d", cfg.IPAddress, cfg.Port)
	}
}

func TestControllerValidateRejectsABadAgentURL(t *testing.T) {
	cfg := ControllerConfig{
		ListenAddress: ":17293",
		AuthToken:     "token",
		Agent:         AgentControllerConfig{URL: "hades:17294"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a schemeless agent URL")
	}
}
