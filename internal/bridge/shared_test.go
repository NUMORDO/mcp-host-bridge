package bridge

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The deliberately stateful counter is a probe of process ownership, not a
// recommendation to share stateful production tools.
func TestSharedBackendOwnership(t *testing.T) {
	h := setupPolicy(t, 16, config.Server{BackendScope: "shared", Stateless: true, Tools: []string{"counter"}})
	a := h.connect(t, "alice")
	b := h.connect(t, "alice")
	other := h.connect(t, "bob")
	if counter(t, a) != "1" || counter(t, b) != "2" || counter(t, other) != "1" {
		t.Fatal("expected one backend per principal")
	}
	listed, err := b.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 1 || listed.Tools[0].Name != "counter" {
		t.Fatalf("narrow tool surface: %v %v", listed, err)
	}
	if _, err := b.CallTool(context.Background(), &mcp.CallToolParams{Name: "hidden"}); err == nil {
		t.Fatal("hidden tool was callable")
	}
	_ = a.Close()
	if counter(t, b) != "3" {
		t.Fatal("one disconnect closed another frontend's backend")
	}
	_ = b.Close()
	_ = other.Close()
	deadline := time.Now().Add(5 * time.Second)
	for h.g.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.g.Active() != 0 {
		t.Fatal("frontends were not reaped")
	}
	if counter(t, h.connect(t, "alice")) != "1" {
		t.Fatal("last disconnect retained backend")
	}
}

func TestSharedConcurrentFirstUse(t *testing.T) {
	h := setupPolicy(t, 16, config.Server{BackendScope: "shared", Stateless: true})
	clients := make([]*mcp.ClientSession, 4)
	for i := range clients {
		clients[i] = h.connect(t, "alice")
	}
	var wg sync.WaitGroup
	results := make(chan int, len(clients))
	for _, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: "counter"})
			if err != nil {
				t.Error(err)
				return
			}
			n, err := strconv.Atoi(r.Content[0].(*mcp.TextContent).Text)
			if err != nil {
				t.Error(err)
				return
			}
			results <- n
		}()
	}
	wg.Wait()
	close(results)
	seen := map[int]bool{}
	for n := range results {
		seen[n] = true
	}
	for n := 1; n <= len(clients); n++ {
		if !seen[n] {
			t.Fatalf("concurrent calls spawned duplicate backends: %v", seen)
		}
	}
}

func TestSharedCallCancellationDoesNotClosePeer(t *testing.T) {
	h := setupPolicy(t, 8, config.Server{BackendScope: "shared", Stateless: true})
	a, b := h.connect(t, "alice"), h.connect(t, "alice")
	if counter(t, a) != "1" || counter(t, b) != "2" {
		t.Fatal("not shared")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := a.CallTool(ctx, &mcp.CallToolParams{Name: "slow"}); err == nil {
		t.Fatal("slow call did not cancel")
	}
	if counter(t, b) != "3" {
		t.Fatal("cancellation destroyed shared process")
	}
}

func TestSharedRequiresAuthenticatedFrontend(t *testing.T) {
	h := setup(t, 4)
	s := h.g.Server("shared", config.Server{BackendScope: "shared", Stateless: true, Tools: []string{"counter"}}, config.Definition{})
	a, b := mcp.NewInMemoryTransports()
	ss, err := s.Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := c.Connect(context.Background(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if _, err := cs.ListTools(context.Background(), nil); err == nil {
		t.Fatal("shared backend admitted unauthenticated stdio")
	}
}

func TestFiveClientsProcessCount(t *testing.T) {
	for _, shared := range []bool{false, true} {
		policy := config.Server{Tools: []string{"pid"}}
		if shared {
			policy.BackendScope, policy.Stateless = "shared", true
		}
		h := setupPolicy(t, 8, policy)
		pids := map[string]bool{}
		for i := 0; i < 5; i++ {
			c := h.connect(t, "alice")
			r, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: "pid"})
			if err != nil {
				t.Fatal(err)
			}
			pids[r.Content[0].(*mcp.TextContent).Text] = true
		}
		want := 5
		if shared {
			want = 1
		}
		if len(pids) != want {
			t.Fatalf("shared=%v: %d subprocesses, want %d", shared, len(pids), want)
		}
		t.Logf("shared=%v clients=5 backend_processes=%d", shared, len(pids))
	}
}

func TestSharedCallLimitAcrossFrontends(t *testing.T) {
	h := setupPolicy(t, 8, config.Server{BackendScope: "shared", Stateless: true})
	a, b := h.connect(t, "alice"), h.connect(t, "alice")
	_ = counter(t, a)
	_ = counter(t, b)
	var owner *session
	h.g.mu.Lock()
	for _, state := range h.g.sessions {
		state.mu.Lock()
		owner = state.owner
		state.mu.Unlock()
		if owner != nil {
			break
		}
	}
	h.g.mu.Unlock()
	if owner == nil {
		t.Fatal("shared owner absent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < h.c.Limits.ConcurrentCalls; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = a.CallTool(ctx, &mcp.CallToolParams{Name: "slow"}) }()
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for len(owner.calls) != cap(owner.calls) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(owner.calls) != cap(owner.calls) {
		cancel()
		wg.Wait()
		t.Fatal("slow calls never filled backend capacity")
	}
	_, err := b.CallTool(context.Background(), &mcp.CallToolParams{Name: "counter"})
	cancel()
	wg.Wait()
	if err == nil {
		t.Fatal("second frontend bypassed shared call limit")
	}
	deadline = time.Now().Add(time.Second)
	for len(owner.calls) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if counter(t, b) != "3" {
		t.Fatal("cancelled work did not release shared capacity")
	}
}

func TestSharedIdleCleanup(t *testing.T) {
	h := setupPolicy(t, 8, config.Server{BackendScope: "shared", Stateless: true})
	a, b := h.connect(t, "alice"), h.connect(t, "alice")
	if counter(t, a) != "1" || counter(t, b) != "2" {
		t.Fatal("not shared")
	}
	deadline := time.Now().Add(5 * time.Second)
	for h.g.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.g.Active() != 0 {
		t.Fatal("idle frontends retained shared backend")
	}
	if counter(t, h.connect(t, "alice")) != "1" {
		t.Fatal("idle backend survived")
	}
}
