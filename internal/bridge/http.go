package bridge

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HTTP routes are host-qualified, so a TLS proxy can federate hosts without copying registries.
func (g *Gateway) HTTP(c *config.Config, defs map[string]config.Definition, authorize func(http.Handler) http.Handler, metadata http.Handler) http.Handler {
	mux := http.NewServeMux()
	for name, policy := range c.Servers {
		server := g.Server(c.Host+"/"+name, policy, defs[name])
		transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
			SessionTimeout: time.Duration(c.Limits.IdleSeconds) * time.Second, JSONResponse: true, MaxRequestBodyBytes: c.Limits.BodyBytes,
			// Exact Host and Origin checks run outside the SDK, including behind TLS proxies.
			DisableLocalhostProtection: true,
		})
		mux.Handle("/mcp/"+c.Host+"/"+name, authorize(transport))
	}
	mux.Handle("/healthz", authorize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ready", "host": c.Host, "active_sessions": g.Active(), "backend_health": "not_probed"})
	})))
	if metadata != nil {
		mux.Handle("/.well-known/oauth-protected-resource", metadata)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !slices.Contains(c.AllowedHosts, r.Host) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		origins := r.Header.Values("Origin")
		if len(origins) > 1 || (len(origins) == 1 && !slices.Contains(c.AllowedOrigins, origins[0])) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
