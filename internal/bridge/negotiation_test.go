package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDualProtocolBackendUsesLegacySession(t *testing.T) {
	// Unlike the original fixture, this server supports modern discovery too.
	s := mcp.NewServer(&mcp.Implementation{Name: "dual-protocol", Version: "1"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "safe"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})
	h := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil))
	defer h.Close()
	cs, err := dial(context.Background(), config.Definition{Type: "http", URL: h.URL}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if cs.InitializeResult().ProtocolVersion != "2025-11-25" || cs.ID() == "" {
		t.Fatal("legacy backend session was not established")
	}
	for i := 0; i < 2; i++ {
		if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "safe"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRelayHeartbeatSurvivesDaemonIdleTimeout(t *testing.T) {
	h := setupPolicy(t, 8, config.Server{BackendScope: "shared", Stateless: true})
	// The fixture daemon expires idle frontends after two seconds. A live stdio
	// owner must keep its HTTP frontend alive without replaying any tool call.
	cs, err := dial(context.Background(), config.Definition{Type: "http",
		URL: h.http.URL + "/mcp/test/fixture", KeepAliveMillis: 300,
		Headers: map[string]string{"Authorization": "Bearer alice"}}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if counter(t, cs) != "1" {
		t.Fatal("unexpected initial state")
	}
	time.Sleep(3 * time.Second)
	if counter(t, cs) != "2" {
		t.Fatal("idle timeout lost the relay's backend state")
	}
	_ = cs.Close()
	deadline := time.Now().Add(3 * time.Second)
	for h.g.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.g.Active() != 0 {
		t.Fatal("closed owner retained its heartbeat/session")
	}
}
