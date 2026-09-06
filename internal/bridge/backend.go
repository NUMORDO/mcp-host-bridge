// Package bridge binds each frontend MCP session to its own host-local backend.
package bridge

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Only execution essentials are inherited. Registry env is applied verbatim locally.
func childEnv(extra map[string]string) []string {
	env := map[string]string{}
	for _, k := range []string{"PATH", "HOME", "USER", "LANG", "LC_ALL", "TMPDIR", "SYSTEMROOT", "SystemRoot", "WINDIR"} {
		if v, ok := os.LookupEnv(k); ok {
			env[k] = v
		}
	}
	for k, v := range extra {
		env[k] = v
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

type headersTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

var backendHTTP = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxResponseHeaderBytes = 64 << 10
	t.MaxIdleConns = 64
	t.MaxIdleConnsPerHost = 4
	t.MaxConnsPerHost = 64
	t.IdleConnTimeout = 30 * time.Second
	return t
}()

func (t headersTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	for k, v := range t.headers {
		clone.Header.Set(k, v)
	}
	return t.base.RoundTrip(clone)
}
func dial(ctx context.Context, d config.Definition, limit int64) (*mcp.ClientSession, error) {
	var t mcp.Transport
	if d.Type == "http" {
		client := &http.Client{Transport: boundedHTTP{base: headersTransport{backendHTTP, d.Headers}, limit: limit}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("upstream redirects disabled") }}
		t = &mcp.StreamableClientTransport{Endpoint: d.URL, HTTPClient: client, MaxRetries: -1, DisableStandaloneSSE: true}
	} else {
		t = &processTransport{definition: d, limit: limit}
	}
	options := &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}}
	if d.KeepAliveMillis > 0 {
		options.KeepAlive = time.Duration(d.KeepAliveMillis) * time.Millisecond
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-host-bridge", Version: "0.2.0-dev"}, options)
	// Select the legacy handshake before contacting an upstream that also supports
	// sessionless discovery. Rejecting only after negotiation stranded compatible
	// real servers on the SDK default (2026) protocol. The SDK owns the fallback.
	client.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "server/discover" {
				return nil, rpcError(-32601, "legacy backend session required")
			}
			return next(ctx, method, req)
		}
	})
	session, err := client.Connect(ctx, t, nil)
	if err != nil {
		return nil, err
	}
	if session.InitializeResult().ProtocolVersion > "2025-11-25" || (d.Type == "http" && session.ID() == "" && !d.AllowStatelessHTTP) {
		_ = session.Close()
		return nil, errors.New("sessionless backend unsupported")
	}
	return session, nil
}
