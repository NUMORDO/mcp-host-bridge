package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func valid() *Config {
	return &Config{Host: "test", Auth: Auth{TokenEnv: "TOKEN"}, Servers: map[string]Server{"demo": {Command: "demo", Tools: []string{"counter"}}}}
}
func TestPolicyFailsClosed(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"no auth":         func(c *Config) { c.Auth = Auth{} },
		"public no tls":   func(c *Config) { c.Listen = "0.0.0.0:8787" },
		"empty allowlist": func(c *Config) { c.Servers["demo"] = Server{Command: "demo"} },
		"wildcard":        func(c *Config) { c.Servers["demo"] = Server{Command: "demo", Tools: []string{"*"}} },
		"path traversal":  func(c *Config) { c.Host = "../../other" },
		"negative limit":  func(c *Config) { c.Limits.Sessions = -1 },
		"registry override": func(c *Config) {
			c.Servers["demo"] = Server{RegistryName: "demo", Args: []string{"override"}, Tools: []string{"counter"}}
		},
		"duplicate": func(c *Config) { c.Servers["demo"] = Server{Command: "demo", Tools: []string{"counter", "counter"}} },
	} {
		t.Run(name, func(t *testing.T) {
			c := valid()
			change(c)
			if c.Validate() == nil {
				t.Fatal("unsafe policy accepted")
			}
		})
	}
}
func TestRegistryExactAndNoMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	original := []byte(`{"mcpServers":{"db":{"command":"uvx","args":["--with","mcp[cli]<2","postgres-mcp","--access-mode=restricted"],"env":{"DATABASE_URI":"secret-value"},"cwd":"/example"}}}`)
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	c := valid()
	c.Registry = path
	c.Servers = map[string]Server{"db": {RegistryName: "db", Tools: []string{"list_schemas"}}}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	d, e := c.Resolve()
	if e != nil {
		t.Fatal(e)
	}
	want := []string{"--with", "mcp[cli]<2", "postgres-mcp", "--access-mode=restricted"}
	if !reflect.DeepEqual(d["db"].Args, want) || d["db"].Env["DATABASE_URI"] != "secret-value" || d["db"].Cwd != "/example" {
		t.Fatal("registry definition changed")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("registry mutated")
	}
}
func TestInvalidAndTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"unknown":true}`, `{} {}`, `{"auth":{"token_env":"SENSITIVE_VALUE"},"unknown":true}`} {
		p := filepath.Join(t.TempDir(), "policy.json")
		os.WriteFile(p, []byte(body), 0600)
		if _, e := Load(p); e == nil {
			t.Fatal("invalid accepted")
		}
	}
}
