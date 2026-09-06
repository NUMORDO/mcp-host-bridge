package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/NUMORDO/mcp-host-bridge/internal/bridge"
	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// relay keeps stdio-only clients lightweight: no registry resolution or backend
// subprocess occurs here. The existing authenticated loopback daemon owns them.
func relay(c *config.Config, name string) error {
	p, d, err := relayTarget(c, name)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	g := bridge.New(c.Limits)
	defer g.Close()
	return g.Server(c.Host+"/"+name, p, d).Run(ctx, &mcp.StdioTransport{})
}

func relayTarget(c *config.Config, name string) (config.Server, config.Definition, error) {
	p, ok := c.Servers[name]
	if !ok {
		return p, config.Definition{}, errors.New("select a configured --server")
	}
	host, _, err := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || c.Auth.TokenEnv == "" || c.Auth.Issuer != "" {
		return p, config.Definition{}, errors.New("relay requires a loopback daemon with host-local bearer authentication")
	}
	token := os.Getenv(c.Auth.TokenEnv)
	if len(token) < 32 {
		return p, config.Definition{}, errors.New("relay bearer token missing or too short")
	}
	// This frontend has a private HTTP session to the daemon; only the daemon
	// applies the selected sharing policy to its own backend process.
	p.BackendScope, p.Stateless = "session", false
	return p, config.Definition{Type: "http", KeepAliveMillis: max(100, c.Limits.IdleSeconds*1000/3), URL: "http://" + c.Listen + "/mcp/" + c.Host + "/" + name,
		Headers: map[string]string{"Authorization": "Bearer " + token}}, nil
}
