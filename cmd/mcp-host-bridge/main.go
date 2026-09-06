// mcp-host-bridge serves explicit host-local MCP capabilities over HTTP or stdio.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/access"
	"github.com/NUMORDO/mcp-host-bridge/internal/bridge"
	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mcp-host-bridge serve|stdio|relay|check|inventory|version [options]")
	}
	if args[0] == "version" {
		fmt.Println("mcp-host-bridge 0.2.0-dev")
		return nil
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	path := f.String("config", "bridge.json", "host-local policy file")
	name := f.String("server", "", "server identifier (stdio)")
	registry := f.String("registry", "", "local Claude registry (inventory)")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if args[0] == "inventory" {
		return inventory(*registry)
	}
	if args[0] != "serve" && args[0] != "stdio" && args[0] != "relay" && args[0] != "check" {
		return errors.New("unknown command")
	}
	c, e := config.Load(*path)
	if e != nil {
		return e
	}
	if args[0] == "relay" {
		return relay(c, *name)
	}
	defs, e := c.Resolve()
	if e != nil {
		return e
	}
	if args[0] == "check" {
		fmt.Printf("policy valid: host=%s servers=%d; backends not started\n", c.Host, len(c.Servers))
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	g := bridge.New(c.Limits)
	defer g.Close()
	if args[0] == "stdio" {
		p, ok := c.Servers[*name]
		if !ok {
			return errors.New("select a configured --server")
		}
		if p.BackendScope == "shared" {
			return errors.New("shared endpoints require the HTTP daemon; use relay for stdio clients")
		}
		return g.Server(c.Host+"/"+*name, p, defs[*name]).Run(ctx, &mcp.StdioTransport{})
	}
	protect, e := access.Middleware(c)
	if e != nil {
		return e
	}
	server := &http.Server{Addr: c.Listen, Handler: g.HTTP(c, defs, protect, access.Metadata(c)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case e := <-done:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP listener failed")
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		g.Close()
		if e := server.Shutdown(shutdown); e != nil {
			_ = server.Close()
			return errors.New("HTTP shutdown exceeded deadline")
		}
		return nil
	}
}
func inventory(path string) error {
	reg, e := config.Registry(path)
	if e != nil {
		return e
	}
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)
	type row struct {
		Name       string   `json:"name"`
		Transport  string   `json:"transport"`
		EnvKeys    []string `json:"env_keys"`
		HeaderKeys []string `json:"header_keys"`
	}
	rows := []row{}
	for _, n := range names {
		d := reg[n]
		t := d.Type
		if t == "" {
			t = "stdio"
		}
		env := []string{}
		headers := []string{}
		for k := range d.Env {
			env = append(env, k)
		}
		for k := range d.Headers {
			headers = append(headers, k)
		}
		sort.Strings(env)
		sort.Strings(headers)
		rows = append(rows, row{n, t, env, headers})
	}
	return json.NewEncoder(os.Stdout).Encode(rows)
}
