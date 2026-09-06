package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCompactMetadataPreservesContracts(t *testing.T) {
	long := strings.Repeat("Guidance. ", 100)
	schema := map[string]any{"type": "object", "description": long,
		"properties": map[string]any{"description": map[string]any{"type": "string", "description": long}},
		"required":   []string{"description"}, "additionalProperties": false,
		"default": map[string]any{"description": long}, "const": map[string]any{"description": long},
		"enum": []any{map[string]any{"description": long}}}
	tool := &mcp.Tool{Name: "unchanged", Description: long, InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}
	compact, err := compactTool(tool, 96)
	if err != nil {
		t.Fatal(err)
	}
	if compact.Name != tool.Name || compact.Annotations != tool.Annotations {
		t.Fatal("identity or annotations changed")
	}
	if len([]rune(compact.Description)) > 96 || !strings.Contains(compact.Description, config.DescribeToolName) {
		t.Fatal("missing compact guidance")
	}
	if tool.Description != long || schema["description"] != long {
		t.Fatal("original metadata mutated")
	}
	got := compact.InputSchema.(map[string]any)
	if got["additionalProperties"] != false || got["default"].(map[string]any)["description"] != long || got["const"].(map[string]any)["description"] != long {
		t.Fatal("literal values or validation constraints changed")
	}
	if got["enum"].([]any)[0].(map[string]any)["description"] != long {
		t.Fatal("enum literal changed")
	}
	property := got["properties"].(map[string]any)["description"].(map[string]any)
	if len([]rune(property["description"].(string))) > 96 {
		t.Fatal("schema guidance not compacted")
	}
	large, err := compactSchema(json.RawMessage(`{"type":"integer","minimum":9007199254740993}`), 96)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(large)
	if !strings.Contains(string(encoded), "9007199254740993") {
		t.Fatal("numeric constraint lost precision")
	}
}

func TestFullGuidanceDoesNotExecuteTool(t *testing.T) {
	h := setupPolicy(t, 8, config.Server{Tools: []string{"counter"}, DescriptionLimit: 96})
	cs := h.connect(t, "alice")
	listing, err := cs.ListTools(context.Background(), nil)
	if err != nil || len(listing.Tools) != 2 {
		t.Fatalf("compact list: %v", err)
	}
	if listing.Tools[0].Name != "counter" || len([]rune(listing.Tools[0].Description)) > 96 {
		t.Fatal("native name or compact description wrong")
	}
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: config.DescribeToolName, Arguments: map[string]any{"tool_name": "counter"}})
	if err != nil {
		t.Fatal(err)
	}
	var full mcp.Tool
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &full); err != nil {
		t.Fatal(err)
	}
	if full.Name != "counter" || len(full.Description) < 1000 {
		t.Fatal("original guidance unavailable")
	}
	if counter(t, cs) != "1" {
		t.Fatal("metadata lookup executed the counter")
	}
	for _, args := range []map[string]any{{"tool_name": "hidden"}, {"tool_name": "counter", "extra": true}, {}} {
		if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: config.DescribeToolName, Arguments: args}); err == nil {
			t.Fatal("metadata allowlist or input validation bypass")
		}
	}
}

func TestCompactionIsOptIn(t *testing.T) {
	h := setupPolicy(t, 4, config.Server{Tools: []string{"counter"}})
	cs := h.connect(t, "alice")
	listing, err := cs.ListTools(context.Background(), nil)
	if err != nil || len(listing.Tools) != 1 || len(listing.Tools[0].Description) < 1000 {
		t.Fatal("default metadata changed")
	}
}
