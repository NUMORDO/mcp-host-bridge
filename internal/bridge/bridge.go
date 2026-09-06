package bridge

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Gateway struct {
	ctx      context.Context
	cancel   context.CancelFunc
	limits   config.Limits
	slots    chan struct{}
	mu       sync.Mutex
	sessions map[*mcp.ServerSession]*session
	closed   bool
	wg       sync.WaitGroup
}
type session struct {
	owner   *session
	release func()
	mu      sync.Mutex
	backend *mcp.ClientSession
	failed  bool
	calls   chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
}

func New(limits config.Limits) *Gateway {
	ctx, cancel := context.WithCancel(context.Background())
	return &Gateway{ctx: ctx, cancel: cancel, limits: limits, slots: make(chan struct{}, limits.Sessions), sessions: map[*mcp.ServerSession]*session{}}
}
func (g *Gateway) Close() {
	g.mu.Lock()
	g.closed = true
	g.cancel()
	fronts := make([]*mcp.ServerSession, 0, len(g.sessions))
	for s := range g.sessions {
		fronts = append(fronts, s)
	}
	g.mu.Unlock()
	for _, s := range fronts {
		_ = s.Close()
	}
	g.wg.Wait()
}
func (g *Gateway) Active() int { g.mu.Lock(); defer g.mu.Unlock(); return len(g.sessions) }

func (g *Gateway) get(front *mcp.ServerSession) (*session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, errors.New("gateway closed")
	}
	if s, ok := g.sessions[front]; ok {
		return s, nil
	}
	select {
	case g.slots <- struct{}{}:
	default:
		return nil, errors.New("session capacity reached")
	}
	ctx, cancel := context.WithCancel(g.ctx)
	s := &session{ctx: ctx, cancel: cancel, calls: make(chan struct{}, g.limits.ConcurrentCalls)}
	g.sessions[front] = s
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		_ = front.Wait()
		s.cancel()
		s.mu.Lock()
		if s.release != nil {
			s.release()
		} else if s.backend != nil {
			_ = s.backend.Close()
		}
		s.mu.Unlock()
		g.mu.Lock()
		delete(g.sessions, front)
		<-g.slots
		g.mu.Unlock()
	}()
	return s, nil
}
func (s *session) connect(ctx context.Context, d config.Definition, limit int64) (*mcp.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed {
		return nil, errors.New("backend unavailable; reconnect required")
	}
	if s.backend != nil {
		return s.backend, nil
	}
	b, e := dial(ctx, d, limit)
	if e != nil {
		s.failed = true
		return nil, errors.New("backend unavailable; reconnect required")
	}
	s.backend = b
	return b, nil
}
func rpcError(code int64, message string) error { return &jsonrpc.Error{Code: code, Message: message} }
func allowed(list []string, name string) bool {
	for _, v := range list {
		if v == name {
			return true
		}
	}
	return false
}

func (g *Gateway) Server(name string, policy config.Server, d config.Definition) *mcp.Server {
	d.AllowStatelessHTTP = policy.BackendScope == "shared" && policy.Stateless
	pool := &backendPool{ctx: g.ctx, limit: g.limits.ConcurrentCalls, entries: make(map[string]*pooledBackend)}
	instructions := "Only explicitly configured host-local capabilities are exposed. Each connection owns an isolated backend session."
	if policy.BackendScope == "shared" {
		instructions = "Only explicitly configured host-local capabilities are exposed. This endpoint shares an operator-attested stateless backend within each authenticated principal."
	}
	if policy.DescriptionLimit > 0 {
		instructions += " Tool descriptions may be shortened. Call bridge_describe_tool before using an unfamiliar tool to obtain its full original guidance."
	}
	caps := &mcp.ServerCapabilities{}
	if len(policy.Tools) > 0 {
		caps.Tools = &mcp.ToolCapabilities{}
	}
	if len(policy.Prompts) > 0 {
		caps.Prompts = &mcp.PromptCapabilities{}
	}
	if len(policy.Resources) > 0 {
		caps.Resources = &mcp.ResourceCapabilities{}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: name, Version: "0.2.0-dev"}, &mcp.ServerOptions{Capabilities: caps, Instructions: instructions})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// Stateful legacy sessions are intentional. Modern sessionless clients must negotiate down.
			if method == "server/discover" {
				return nil, rpcError(-32601, "stateful gateway requires protocol 2025-11-25 or earlier")
			}
			front, ok := req.GetSession().(*mcp.ServerSession)
			if !ok {
				return nil, rpcError(-32603, "missing session")
			}
			if versioned, ok := req.(interface{ ProtocolVersion() string }); ok && versioned.ProtocolVersion() > "2025-11-25" {
				return nil, rpcError(-32600, "unsupported sessionless protocol")
			}
			state, e := g.get(front)
			if e != nil {
				return nil, rpcError(-32000, e.Error())
			}
			switch method {
			case "initialize", "notifications/initialized", "notifications/cancelled", "ping":
				return next(ctx, method, req)
			case "tools/list":
				if len(policy.Tools) == 0 {
					return nil, rpcError(-32601, "tools disabled")
				}
			case "tools/call":
				q := req.GetParams().(*mcp.CallToolParamsRaw)
				if q == nil {
					return nil, rpcError(-32602, "tool parameters required")
				}
				if policy.DescriptionLimit > 0 && q.Name == config.DescribeToolName {
					if _, err := descriptionTarget(q.Arguments, policy); err != nil {
						return nil, rpcError(-32602, "an allowed tool_name is required")
					}
				} else if !allowed(policy.Tools, q.Name) {
					return nil, rpcError(-32602, "tool not allowed")
				}
			case "prompts/list":
				if len(policy.Prompts) == 0 {
					return nil, rpcError(-32601, "prompts disabled")
				}
			case "prompts/get":
				params := req.GetParams().(*mcp.GetPromptParams)
				if params == nil || !allowed(policy.Prompts, params.Name) {
					return nil, rpcError(-32602, "prompt not allowed")
				}
			case "resources/list":
				if len(policy.Resources) == 0 {
					return nil, rpcError(-32601, "resources disabled")
				}
			case "resources/read":
				params := req.GetParams().(*mcp.ReadResourceParams)
				if params == nil || !allowed(policy.Resources, params.URI) {
					return nil, rpcError(-32602, "resource not allowed")
				}
			case "resources/templates/list":
				return privateResult(&mcp.ListResourceTemplatesResult{ResourceTemplates: []*mcp.ResourceTemplate{}}), nil
			default:
				return nil, rpcError(-32601, "method not exposed")
			}
			select {
			case state.calls <- struct{}{}:
				defer func() { <-state.calls }()
			default:
				return nil, rpcError(-32000, "concurrent call capacity reached")
			}
			callCtx, cancel := context.WithTimeout(ctx, time.Duration(g.limits.TimeoutSeconds)*time.Second)
			defer cancel()
			stop := context.AfterFunc(state.ctx, cancel)
			defer stop()
			owner := state
			if policy.BackendScope == "shared" {
				if !policy.Stateless {
					return nil, rpcError(-32000, "shared backend requires stateless attestation")
				}
				extra := req.GetExtra()
				if extra == nil || extra.TokenInfo == nil || extra.TokenInfo.UserID == "" {
					return nil, rpcError(-32000, "shared backend requires authenticated HTTP principal")
				}
				owner, e = state.attach(pool, extra.TokenInfo.UserID)
				if e != nil {
					return nil, rpcError(-32000, e.Error())
				}
				select {
				case owner.calls <- struct{}{}:
					defer func() { <-owner.calls }()
				default:
					return nil, rpcError(-32000, "shared backend call capacity reached")
				}
			}
			b, e := owner.connect(callCtx, d, g.limits.OutputBytes)
			if e != nil {
				return nil, rpcError(-32000, e.Error())
			}
			result, e := forward(callCtx, b, method, req, policy)
			if e != nil {
				return nil, rpcError(-32000, "backend request failed; no automatic retry")
			}
			return privateResult(result), nil
		}
	})
	return server
}

func forward(ctx context.Context, b *mcp.ClientSession, method string, req mcp.Request, p config.Server) (mcp.Result, error) {
	switch method {
	case "tools/list":
		r, e := b.ListTools(ctx, req.GetParams().(*mcp.ListToolsParams))
		if e != nil {
			return nil, e
		}
		items := []*mcp.Tool{}
		for _, t := range r.Tools {
			if allowed(p.Tools, t.Name) {
				if p.DescriptionLimit > 0 {
					compact, err := compactTool(t, p.DescriptionLimit)
					if err != nil {
						return nil, err
					}
					items = append(items, compact)
				} else {
					items = append(items, t)
				}
			}
		}
		params := req.GetParams().(*mcp.ListToolsParams)
		if p.DescriptionLimit > 0 && (params == nil || params.Cursor == "") {
			items = append(items, descriptorTool())
		}
		r.Tools = items
		return r, nil
	case "tools/call":
		q := req.GetParams().(*mcp.CallToolParamsRaw)
		if p.DescriptionLimit > 0 && q.Name == config.DescribeToolName {
			return describeTool(ctx, b, q.Arguments, p)
		}
		return b.CallTool(ctx, &mcp.CallToolParams{Meta: q.Meta, Name: q.Name, Arguments: q.Arguments})
	case "prompts/list":
		r, e := b.ListPrompts(ctx, req.GetParams().(*mcp.ListPromptsParams))
		if e != nil {
			return nil, e
		}
		items := []*mcp.Prompt{}
		for _, v := range r.Prompts {
			if allowed(p.Prompts, v.Name) {
				items = append(items, v)
			}
		}
		r.Prompts = items
		return r, nil
	case "prompts/get":
		return b.GetPrompt(ctx, req.GetParams().(*mcp.GetPromptParams))
	case "resources/list":
		r, e := b.ListResources(ctx, req.GetParams().(*mcp.ListResourcesParams))
		if e != nil {
			return nil, e
		}
		items := []*mcp.Resource{}
		for _, v := range r.Resources {
			if allowed(p.Resources, v.URI) {
				items = append(items, v)
			}
		}
		r.Resources = items
		return r, nil
	case "resources/read":
		return b.ReadResource(ctx, req.GetParams().(*mcp.ReadResourceParams))
	}
	return nil, errors.New("unsupported method")
}
