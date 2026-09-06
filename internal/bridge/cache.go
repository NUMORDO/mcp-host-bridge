package bridge

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Filtered results are principal/policy specific. Use a valid private cache scope
// even when an older upstream omitted cache hints. The SDK's zero-value empty
// scope is rejected by strict clients such as Codex's Rust MCP client.
func privateResult(result mcp.Result) mcp.Result {
	var cache *mcp.Cacheable
	switch value := result.(type) {
	case *mcp.ListToolsResult:
		cache = &value.Cacheable
	case *mcp.ListPromptsResult:
		cache = &value.Cacheable
	case *mcp.ListResourcesResult:
		cache = &value.Cacheable
	case *mcp.ListResourceTemplatesResult:
		cache = &value.Cacheable
	case *mcp.ReadResourceResult:
		cache = &value.Cacheable
	}
	if cache != nil {
		cache.CacheScope = "private"
		cache.TTLMs = 0
	}
	return result
}
