package bridge

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWireCacheScopeIsValidAndPrivate(t *testing.T) {
	for _, result := range []mcp.Result{
		&mcp.ListToolsResult{Tools: []*mcp.Tool{}},
		&mcp.ListPromptsResult{}, &mcp.ListResourcesResult{},
		&mcp.ListResourceTemplatesResult{}, &mcp.ReadResourceResult{},
		&mcp.ListToolsResult{Cacheable: mcp.Cacheable{CacheScope: "public", TTLMs: 3600000}},
	} {
		data, err := json.Marshal(privateResult(result))
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if wire["cacheScope"] != "private" || wire["ttlMs"] != float64(0) {
			t.Fatalf("invalid or cross-principal cache hints: %s", data)
		}
	}
}
