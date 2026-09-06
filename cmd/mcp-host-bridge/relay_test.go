package main

import (
	"strings"
	"testing"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
)

func TestRelayTarget(t *testing.T) {
	t.Setenv("RELAY_TEST_TOKEN", strings.Repeat("x", 32))
	c := &config.Config{Host: "local", Listen: "127.0.0.1:8787", Auth: config.Auth{TokenEnv: "RELAY_TEST_TOKEN"}, Servers: map[string]config.Server{
		"safe": {BackendScope: "shared", Stateless: true, RegistryName: "not-read-by-relay", Tools: []string{"safe"}},
	}}
	p, d, err := relayTarget(c, "safe")
	if err != nil || p.BackendScope != "session" || p.Stateless || d.Type != "http" || d.URL != "http://127.0.0.1:8787/mcp/local/safe" || len(p.Tools) != 1 {
		t.Fatalf("invalid relay target (error=%v)", err)
	}
	for _, addr := range []string{"0.0.0.0:8787", "192.0.2.1:8787", "localhost:8787", "invalid"} {
		c.Listen = addr
		if _, _, err := relayTarget(c, "safe"); err == nil {
			t.Errorf("accepted %s", addr)
		}
	}
	c.Listen = "127.0.0.1:8787"
	t.Setenv("RELAY_TEST_TOKEN", "")
	if _, _, err := relayTarget(c, "safe"); err == nil {
		t.Fatal("accepted missing token")
	}
}
