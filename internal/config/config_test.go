package config

import "testing"

func TestServerConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *ServerConfig
		wantErr bool
	}{
		{
			name:    "rejects non-loopback bind without auth",
			cfg:     &ServerConfig{ListenAddr: "0.0.0.0:1080"},
			wantErr: true,
		},
		{
			name: "allows loopback bind without auth",
			cfg:  &ServerConfig{ListenAddr: "127.0.0.1:1080"},
		},
		{
			name: "allows non-loopback bind with auth",
			cfg: &ServerConfig{
				ListenAddr: "0.0.0.0:1080",
				Auth:       true,
				User:       "bethrou",
				Pass:       "secret",
			},
		},
		{
			name: "allows explicit insecure bind override",
			cfg: &ServerConfig{
				ListenAddr:        "0.0.0.0:1080",
				AllowInsecureBind: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got none")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

// TargetNodes is optional: an empty list means the client discovers every
// exit node on its account from the control plane at connect time, rather
// than requiring local configuration. APIURL, on the other hand, has no
// such fallback and is still required.
func TestClientConfigRequiresAPIURLButNotTargetNodes(t *testing.T) {
	cfg := &ClientConfig{
		Key:         "network.key",
		Server:      &ServerConfig{ListenAddr: "127.0.0.1:1080"},
		Routing:     &RoutingConfig{Strategy: "random", Timeout: "10s"},
		APIURL:      "http://localhost:8080",
		TargetNodes: []string{"12D3KooWExample"},
		Log:         &LogConfig{Level: "info", Format: "text"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected fully-specified config to validate, got %v", err)
	}

	cfg.TargetNodes = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected config without target nodes to still validate (auto-discovered at connect time), got %v", err)
	}

	cfg.APIURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected config without api_url to be rejected")
	}
}
