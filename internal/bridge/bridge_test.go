package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBackendProcess(t *testing.T) {
	if os.Getenv("BRIDGE_TEST_BACKEND") != "1" {
		return
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	var count atomic.Int64
	mcp.AddTool(s, &mcp.Tool{Name: "pid"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprint(os.Getpid())}}}, nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "counter"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprint(count.Add(1))}}}, nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "hidden"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})
	s.AddPrompt(&mcp.Prompt{Name: "greeting"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "hello"}}}}, nil
	})
	s.AddResource(&mcp.Resource{URI: "fixture://safe", Name: "safe"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "fixture://safe", Text: "safe"}}}, nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "slow"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &mcp.CallToolResult{}, nil, nil
		}
	})
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, m string, r mcp.Request) (mcp.Result, error) {
			if m == "server/discover" {
				return nil, &jsonrpc.Error{Code: -32601, Message: "legacy"}
			}
			return next(ctx, m, r)
		}
	})
	e := s.Run(context.Background(), &mcp.StdioTransport{})
	if e != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

type harness struct {
	g    *Gateway
	c    *config.Config
	http *httptest.Server
}

func setup(t *testing.T, max int) *harness {
	return setupPolicy(t, max, config.Server{})
}

func setupPolicy(t *testing.T, max int, policy config.Server) *harness {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	policy.Command = exe
	if policy.Tools == nil {
		policy.Tools = []string{"counter", "slow"}
	}
	c := &config.Config{Host: "test", Auth: config.Auth{TokenEnv: "unused"}, Servers: map[string]config.Server{"fixture": policy}, Limits: config.Limits{Sessions: max, IdleSeconds: 2, TimeoutSeconds: 1}}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	g := New(c.Limits)
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token != "alice" && token != "bob" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: token, Expiration: time.Now().Add(time.Hour)}, nil
	}
	protect := auth.RequireBearerToken(verifier, nil)
	defs := map[string]config.Definition{"fixture": {Type: "stdio", Command: exe, Args: []string{"-test.run=^TestBackendProcess$"}, Env: map[string]string{"BRIDGE_TEST_BACKEND": "1"}}}
	hs := httptest.NewUnstartedServer(g.HTTP(c, defs, protect, nil))
	c.AllowedHosts = []string{hs.Listener.Addr().String()}
	hs.Start()
	t.Cleanup(func() { g.Close(); hs.Close() })
	return &harness{g, c, hs}
}
func (h *harness) connect(t *testing.T, who string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, e := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: h.http.URL + "/mcp/test/fixture", HTTPClient: &http.Client{Transport: headersTransport{http.DefaultTransport, map[string]string{"Authorization": "Bearer " + who}}}, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}
func counter(t *testing.T, cs *mcp.ClientSession) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	r, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "counter", Arguments: map[string]any{}})
	if e != nil {
		t.Fatal(e)
	}
	if r.IsError {
		t.Fatalf("backend tool error: %#v", r)
	}
	return r.Content[0].(*mcp.TextContent).Text
}
func TestHTTPIsolationPolicyAndCleanup(t *testing.T) {
	h := setup(t, 4)
	a := h.connect(t, "alice")
	b := h.connect(t, "alice")
	if a.InitializeResult().ProtocolVersion != "2025-11-25" {
		t.Fatal("must negotiate stateful legacy protocol")
	}
	if counter(t, a) != "1" || counter(t, a) != "2" || counter(t, b) != "1" {
		t.Fatal("session state mixed or reset between requests")
	}
	list, e := a.ListTools(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, tool := range list.Tools {
		if tool.Name == "hidden" {
			t.Fatal("hidden tool leaked")
		}
	}
	if _, e = a.CallTool(context.Background(), &mcp.CallToolParams{Name: "hidden"}); e == nil {
		t.Fatal("direct hidden call permitted")
	}
	req, _ := http.NewRequest("POST", h.http.URL+"/mcp/test/fixture", strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list","params":{}}`))
	req.Header.Set("Authorization", "Bearer bob")
	req.Header.Set("Mcp-Session-Id", a.ID())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatal("cross-principal session accepted")
	}
	_ = a.Close()
	_ = b.Close()
	deadline := time.Now().Add(5 * time.Second)
	for h.g.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.g.Active() != 0 {
		t.Fatal("backend sessions leaked")
	}
}
func TestHTTPGuards(t *testing.T) {
	h := setup(t, 2)
	for _, tc := range []struct {
		name, auth, origin, host string
		body                     []byte
		want                     int
	}{
		{name: "no auth", want: 401},
		{name: "invalid origin", auth: "Bearer alice", origin: "https://evil.invalid", want: 403},
		{name: "invalid host", auth: "Bearer alice", host: "evil.invalid", want: 403},
		{name: "oversized body", auth: "Bearer alice", body: bytes.Repeat([]byte("x"), int(h.c.Limits.BodyBytes)+1), want: 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("POST", h.http.URL+"/mcp/test/fixture", bytes.NewReader(tc.body))
			r.Header.Set("Authorization", tc.auth)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.host != "" {
				r.Host = tc.host
			}
			resp, e := http.DefaultClient.Do(r)
			if e != nil {
				t.Fatal(e)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("got %d want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
func TestTimeoutAndIdleReaping(t *testing.T) {
	h := setup(t, 2)
	a := h.connect(t, "alice")
	counter(t, a)
	start := time.Now()
	_, e := a.CallTool(context.Background(), &mcp.CallToolParams{Name: "slow"})
	if e == nil || time.Since(start) > 3*time.Second {
		t.Fatal("call deadline not enforced")
	}
	deadline := time.Now().Add(6 * time.Second)
	for h.g.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.g.Active() != 0 {
		t.Fatal("idle backend leaked")
	}
}
func TestSessionCapacity(t *testing.T) {
	h := setup(t, 1)
	a := h.connect(t, "alice")
	counter(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := mcp.NewClient(&mcp.Implementation{Name: "extra", Version: "1"}, nil)
	b, e := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: h.http.URL + "/mcp/test/fixture", HTTPClient: &http.Client{Transport: headersTransport{http.DefaultTransport, map[string]string{"Authorization": "Bearer alice"}}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if e == nil {
		b.Close()
		t.Fatal("excess session accepted")
	}
	if h.g.Active() != 1 {
		t.Fatal("capacity bookkeeping corrupted")
	}
}
func TestStdioAndHTTPConcurrent(t *testing.T) {
	h := setup(t, 3)
	httpSession := h.connect(t, "alice")
	exe, _ := os.Executable()
	server := h.g.Server("local", h.c.Servers["fixture"], config.Definition{Command: exe, Args: []string{"-test.run=^TestBackendProcess$"}, Env: map[string]string{"BRIDGE_TEST_BACKEND": "1"}})
	st, ct := mcp.NewInMemoryTransports()
	ss, e := server.Connect(context.Background(), st, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	cs, e := mcp.NewClient(&mcp.Implementation{Name: "local", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	if counter(t, cs) != "1" || counter(t, httpSession) != "1" || counter(t, cs) != "2" {
		t.Fatal("local/HTTP state contamination")
	}
}
func TestChildEnvironmentDoesNotInheritSecrets(t *testing.T) {
	t.Setenv("UNRELATED_API_SECRET", "not-for-child")
	env := childEnv(map[string]string{"EXPLICIT": "present"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "UNRELATED_API_SECRET") {
		t.Fatal("secret inherited")
	}
	if !strings.Contains(joined, "EXPLICIT=present") {
		t.Fatal("explicit env lost")
	}
}
func TestHTTPUpstream(t *testing.T) {
	upstream := setup(t, 2)
	g := New(upstream.c.Limits)
	defer g.Close()
	s := g.Server("remote", config.Server{Tools: []string{"counter"}}, config.Definition{Type: "http", URL: upstream.http.URL + "/mcp/test/fixture", Headers: map[string]string{"Authorization": "Bearer alice"}})
	st, ct := mcp.NewInMemoryTransports()
	ss, e := s.Connect(context.Background(), st, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	c, e := mcp.NewClient(&mcp.Implementation{Name: "remote-client", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if counter(t, c) != "1" || counter(t, c) != "2" {
		t.Fatal("remote session lost")
	}
}
func TestMissingBackendIsNotEmptySuccess(t *testing.T) {
	h := setup(t, 2)
	s := h.g.Server("missing", config.Server{Tools: []string{"counter"}}, config.Definition{Command: "/does-not-exist"})
	st, ct := mcp.NewInMemoryTransports()
	ss, _ := s.Connect(context.Background(), st, nil)
	defer ss.Close()
	c, e := mcp.NewClient(&mcp.Implementation{Name: "missing", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.ListTools(context.Background(), nil)
	if e == nil {
		t.Fatal("unavailable reported as success")
	}
	b, _ := json.Marshal(e.Error())
	if bytes.Contains(b, []byte("does-not-exist")) {
		t.Fatal("upstream executable leaked")
	}
}
