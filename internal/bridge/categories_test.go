package bridge

import (
	"context"
	"os"
	"testing"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPromptAndResourcePolicy(t *testing.T) {
	h := setup(t, 2)
	exe, _ := os.Executable()
	p := config.Server{Prompts: []string{"greeting"}, Resources: []string{"fixture://safe"}}
	s := h.g.Server("categories", p, config.Definition{Command: exe, Args: []string{"-test.run=^TestBackendProcess$"}, Env: map[string]string{"BRIDGE_TEST_BACKEND": "1"}})
	st, ct := mcp.NewInMemoryTransports()
	ss, e := s.Connect(context.Background(), st, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	cs, e := mcp.NewClient(&mcp.Implementation{Name: "categories", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	prompts, e := cs.ListPrompts(context.Background(), nil)
	if e != nil || len(prompts.Prompts) != 1 {
		t.Fatalf("prompt listing %v", e)
	}
	prompt, e := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "greeting"})
	if e != nil || len(prompt.Messages) != 1 {
		t.Fatalf("prompt get %v", e)
	}
	if _, e = cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "hidden"}); e == nil {
		t.Fatal("hidden prompt allowed")
	}
	resources, e := cs.ListResources(context.Background(), nil)
	if e != nil || len(resources.Resources) != 1 {
		t.Fatalf("resource listing %v", e)
	}
	r, e := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "fixture://safe"})
	if e != nil || r.Contents[0].Text != "safe" {
		t.Fatalf("resource read %v", e)
	}
	if _, e = cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "fixture://other"}); e == nil {
		t.Fatal("hidden resource allowed")
	}
	if _, e = cs.ListTools(context.Background(), nil); e == nil {
		t.Fatal("disabled tools category exposed")
	}
}
