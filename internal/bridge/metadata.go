package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func shorten(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	suffix := "… [Full guidance: bridge_describe_tool]"
	return string(runes[:limit-len([]rune(suffix))]) + suffix
}

func compactSchema(schema any, limit int) (any, error) {
	if schema == nil {
		return nil, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var copy any
	if err := decoder.Decode(&copy); err != nil {
		return nil, err
	}
	shortenSchema(copy, limit)
	return copy, nil
}

// Traverse schema-valued keywords only. A literal object in const/default/enum
// may itself contain a "description" key; changing it would change validation.
func shortenSchema(value any, limit int) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	if description, ok := object["description"].(string); ok {
		object["description"] = shorten(description, limit)
	}
	for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas", "dependencies"} {
		if children, ok := object[key].(map[string]any); ok {
			for _, child := range children {
				shortenSchema(child, limit)
			}
		}
	}
	for _, key := range []string{"items", "additionalProperties", "unevaluatedProperties", "unevaluatedItems", "contains", "propertyNames", "if", "then", "else", "not", "contentSchema", "allOf", "anyOf", "oneOf", "prefixItems"} {
		if children, ok := object[key].([]any); ok {
			for _, child := range children {
				shortenSchema(child, limit)
			}
		} else {
			shortenSchema(object[key], limit)
		}
	}
}

func compactTool(tool *mcp.Tool, limit int) (*mcp.Tool, error) {
	copy := *tool
	copy.Description = shorten(tool.Description, limit)
	var err error
	copy.InputSchema, err = compactSchema(tool.InputSchema, limit)
	if err != nil {
		return nil, err
	}
	copy.OutputSchema, err = compactSchema(tool.OutputSchema, limit)
	return &copy, err
}

func descriptorTool() *mcp.Tool {
	closed := false
	return &mcp.Tool{Name: config.DescribeToolName,
		Description: "Read the complete original guidance and schema for one allowed tool. This only reads metadata; it never executes that tool.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"tool_name": map[string]any{"type": "string"}}, "required": []string{"tool_name"}, "additionalProperties": false},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &closed, OpenWorldHint: &closed}}
}

func descriptionTarget(raw json.RawMessage, policy config.Server) (string, error) {
	var input struct {
		ToolName string `json:"tool_name"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		return "", err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return "", errors.New("unexpected trailing input")
	}
	if !allowed(policy.Tools, input.ToolName) {
		return "", errors.New("tool not allowed")
	}
	return input.ToolName, nil
}

func describeTool(ctx context.Context, backend *mcp.ClientSession, raw json.RawMessage, policy config.Server) (*mcp.CallToolResult, error) {
	name, err := descriptionTarget(raw, policy)
	if err != nil {
		return nil, err
	}
	cursor := ""
	seen := map[[32]byte]bool{}
	for range 64 {
		key := sha256.Sum256([]byte(cursor))
		if seen[key] {
			return nil, errors.New("repeated metadata cursor")
		}
		seen[key] = true
		page, err := backend.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, tool := range page.Tools {
			if tool.Name == name {
				data, err := json.Marshal(tool)
				if err != nil {
					return nil, err
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
			}
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	return nil, errors.New("allowed tool metadata unavailable")
}
