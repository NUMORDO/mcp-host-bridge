// mcp-bridge-demo is a harmless stateful stdio backend for isolated validation.
package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	s := mcp.NewServer(&mcp.Implementation{Name: "bridge-demo", Version: "0.1.0"}, nil)
	var count atomic.Int64
	mcp.AddTool(s, &mcp.Tool{Name: "counter", Description: "Increment this connection's isolated counter."}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprint(count.Add(1))}}}, nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "hidden", Description: "Used to verify filtering; do not expose."}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, r mcp.Request) (mcp.Result, error) {
			if method == "server/discover" {
				return nil, &jsonrpc.Error{Code: -32601, Message: "legacy fixture"}
			}
			return next(ctx, method, r)
		}
	})
	if e := s.Run(context.Background(), &mcp.StdioTransport{}); e != nil {
		fmt.Fprintln(os.Stderr, "demo transport ended")
		os.Exit(1)
	}
}
