package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStatelessHTTPRequiresEndpointAttestation(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "stateless-docs", Version: "1"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "safe"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "read-only"}}}, nil, nil
	})
	upstream := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer upstream.Close()
	for _, attested := range []bool{false, true} {
		policy := config.Server{Command: "unused", Tools: []string{"safe"}}
		if attested {
			policy.BackendScope, policy.Stateless = "shared", true
		}
		c := &config.Config{Host: "test", Auth: config.Auth{TokenEnv: "unused"}, Servers: map[string]config.Server{"fixture": policy}}
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		g := New(c.Limits)
		protect := auth.RequireBearerToken(func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
			return &auth.TokenInfo{UserID: token, Expiration: time.Now().Add(time.Hour)}, nil
		}, nil)
		// Even a Definition supplied with true cannot override the endpoint policy.
		defs := map[string]config.Definition{"fixture": {Type: "http", URL: upstream.URL, AllowStatelessHTTP: true}}
		hs := httptest.NewUnstartedServer(g.HTTP(c, defs, protect, nil))
		c.AllowedHosts = []string{hs.Listener.Addr().String()}
		hs.Start()
		h := &harness{g, c, hs}
		cs := h.connect(t, "alice")
		result, err := cs.ListTools(context.Background(), nil)
		if !attested {
			if err == nil {
				t.Fatal("stateless upstream admitted without endpoint attestation")
			}
		} else {
			if err != nil || len(result.Tools) != 1 {
				t.Fatalf("attested list failed: %v", err)
			}
			if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "safe"}); err != nil {
				t.Fatal(err)
			}
		}
		_ = cs.Close()
		g.Close()
		hs.Close()
	}
}

func TestRegistryCannotAttestStatelessHTTP(t *testing.T) {
	var definition config.Definition
	if err := json.Unmarshal([]byte(`{"AllowStatelessHTTP":true,"allow_stateless_http":true}`), &definition); err != nil {
		t.Fatal(err)
	}
	if definition.AllowStatelessHTTP {
		t.Fatal("registry was allowed to set an internal policy field")
	}
}
