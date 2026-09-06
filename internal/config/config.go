// Package config loads an explicit host-local exposure policy without exporting secrets.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

type Config struct {
	Host           string            `json:"host"`
	Listen         string            `json:"listen"`
	Registry       string            `json:"registry"`
	PublicURL      string            `json:"public_url"`
	AllowedHosts   []string          `json:"allowed_hosts"`
	AllowedOrigins []string          `json:"allowed_origins"`
	Auth           Auth              `json:"auth"`
	Limits         Limits            `json:"limits"`
	Servers        map[string]Server `json:"servers"`
}
type Auth struct {
	TokenEnv      string `json:"token_env"`
	Issuer        string `json:"issuer"`
	Audience      string `json:"audience"`
	PublicKeyFile string `json:"public_key_file"`
	Scope         string `json:"scope"`
}
type Limits struct {
	Sessions        int   `json:"sessions"`
	ConcurrentCalls int   `json:"concurrent_calls"`
	TimeoutSeconds  int   `json:"timeout_seconds"`
	IdleSeconds     int   `json:"idle_seconds"`
	BodyBytes       int64 `json:"body_bytes"`
	OutputBytes     int64 `json:"output_bytes"`
}
type Server struct {
	BackendScope string   `json:"backend_scope,omitempty"`
	Stateless    bool     `json:"stateless,omitempty"`
	RegistryName string   `json:"registry_name"`
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	Cwd          string   `json:"cwd"`
	EnvKeys      []string `json:"env_keys"`
	Tools        []string `json:"tools"`
	Prompts      []string `json:"prompts"`
	Resources    []string `json:"resources"`
}

// Definition is resolved in memory on the owning host; it must never be logged.
type Definition struct {
	AllowStatelessHTTP bool              `json:"-"` // Derived from endpoint policy, never from registry data.
	KeepAliveMillis    int               `json:"-"` // Internal relay lifecycle; never imported from a registry.
	Type               string            `json:"type"`
	Command            string            `json:"command"`
	Args               []string          `json:"args"`
	Env                map[string]string `json:"env"`
	Headers            map[string]string `json:"headers"`
	URL                string            `json:"url"`
	Cwd                string            `json:"cwd"`
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open policy")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	var c Config
	if err = d.Decode(&c); err != nil {
		return nil, errors.New("invalid policy JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing policy JSON")
	}
	if err = c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
func (c *Config) Validate() error {
	if !namePattern.MatchString(c.Host) {
		return errors.New("host must be an explicit short identifier")
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8787"
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return errors.New("listen must be IP:port")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("listen must use a literal IP")
	}
	if c.PublicURL != "" {
		u, e := url.Parse(c.PublicURL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return errors.New("public_url must be an HTTPS origin without trailing slash")
		}
	}
	if !ip.IsLoopback() && c.PublicURL == "" {
		return errors.New("non-loopback listen requires public_url and TLS reverse proxy")
	}
	if len(c.AllowedHosts) == 0 {
		c.AllowedHosts = []string{c.Listen}
	}
	if c.Auth.TokenEnv != "" && (c.Auth.Issuer != "" || c.Auth.PublicKeyFile != "") {
		return errors.New("choose bearer or OAuth, not both")
	}
	if c.Auth.TokenEnv == "" && (c.Auth.Issuer == "" || c.Auth.PublicKeyFile == "" || c.Auth.Audience == "" || c.Auth.Scope == "" || c.PublicURL == "") {
		return errors.New("authentication is required: token_env or complete OAuth resource-server settings")
	}
	if c.Auth.Issuer != "" {
		u, e := url.Parse(c.Auth.Issuer)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("issuer must be HTTPS")
		}
	}
	if c.Limits.Sessions == 0 {
		c.Limits.Sessions = 16
	}
	if c.Limits.ConcurrentCalls == 0 {
		c.Limits.ConcurrentCalls = 4
	}
	if c.Limits.TimeoutSeconds == 0 {
		c.Limits.TimeoutSeconds = 60
	}
	if c.Limits.IdleSeconds == 0 {
		c.Limits.IdleSeconds = 300
	}
	if c.Limits.BodyBytes == 0 {
		c.Limits.BodyBytes = 1 << 20
	}
	if c.Limits.OutputBytes == 0 {
		c.Limits.OutputBytes = 4 << 20
	}
	if c.Limits.OutputBytes < 1024 || c.Limits.OutputBytes > 16<<20 {
		return errors.New("output_bytes out of range")
	}
	if c.Limits.Sessions < 1 || c.Limits.Sessions > 256 || c.Limits.ConcurrentCalls < 1 || c.Limits.ConcurrentCalls > 64 || c.Limits.TimeoutSeconds < 1 || c.Limits.TimeoutSeconds > 600 || c.Limits.IdleSeconds < 1 || c.Limits.IdleSeconds > 3600 || c.Limits.BodyBytes < 1024 || c.Limits.BodyBytes > 16<<20 {
		return errors.New("limits out of range")
	}
	if len(c.Servers) == 0 || len(c.Servers) > 64 {
		return errors.New("configure between 1 and 64 servers")
	}
	for n, s := range c.Servers {
		if s.BackendScope != "" && s.BackendScope != "session" && s.BackendScope != "shared" {
			return fmt.Errorf("server %s invalid backend_scope", n)
		}
		if (s.BackendScope == "shared") != s.Stateless {
			return fmt.Errorf("server %s shared scope requires explicit stateless attestation (and vice versa)", n)
		}
		if !namePattern.MatchString(n) {
			return errors.New("invalid server identifier")
		}
		if (s.RegistryName == "") == (s.Command == "") {
			return fmt.Errorf("server %s requires exactly one registry_name or command", n)
		}
		if s.RegistryName != "" && (len(s.Args) > 0 || len(s.EnvKeys) > 0 || s.Cwd != "") {
			return fmt.Errorf("server %s registry definition cannot be overridden", n)
		}
		if len(s.Tools)+len(s.Prompts)+len(s.Resources) == 0 {
			return fmt.Errorf("server %s requires an explicit exposure allowlist", n)
		}
		for _, list := range [][]string{s.Tools, s.Prompts, s.Resources} {
			seen := map[string]bool{}
			for _, v := range list {
				if v == "" || v == "*" || seen[v] {
					return errors.New("empty, wildcard or duplicate allowlist entry")
				}
				seen[v] = true
			}
		}
	}
	return nil
}
func Registry(path string) (map[string]Definition, error) {
	if path == "" {
		h, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		path = filepath.Join(h, ".claude.json")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, errors.New("cannot read host-local registry")
	}
	var r struct {
		Servers map[string]Definition `json:"mcpServers"`
	}
	if json.Unmarshal(b, &r) != nil || r.Servers == nil {
		return nil, errors.New("invalid host-local registry")
	}
	return r.Servers, nil
}
func (c *Config) Resolve() (map[string]Definition, error) {
	out := map[string]Definition{}
	var reg map[string]Definition
	for n, s := range c.Servers {
		d := Definition{Type: "stdio", Command: s.Command, Args: s.Args, Cwd: s.Cwd, Env: map[string]string{}}
		if s.RegistryName != "" {
			if reg == nil {
				var e error
				reg, e = Registry(c.Registry)
				if e != nil {
					return nil, e
				}
			}
			var ok bool
			d, ok = reg[s.RegistryName]
			if !ok {
				return nil, fmt.Errorf("registry entry absent for %s", n)
			}
		} else {
			for _, k := range s.EnvKeys {
				v, ok := os.LookupEnv(k)
				if !ok {
					return nil, fmt.Errorf("required environment missing for %s", n)
				}
				d.Env[k] = v
			}
		}
		if d.Type == "" {
			d.Type = "stdio"
		}
		if d.Type == "stdio" {
			if d.Command == "" || d.URL != "" {
				return nil, errors.New("invalid stdio definition")
			}
		} else if d.Type == "http" {
			u, e := url.Parse(d.URL)
			if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
				return nil, errors.New("invalid HTTP upstream")
			}
		} else {
			return nil, errors.New("only stdio and Streamable HTTP upstreams are supported")
		}
		out[n] = d
	}
	return out, nil
}
