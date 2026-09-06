package bridge

import (
	"context"
	"errors"
	"sync"
)

// A pool belongs to exactly one configured endpoint. Credentials and exposure
// policy are never merged across endpoints, even if registry names are equal.
type backendPool struct {
	mu      sync.Mutex
	ctx     context.Context
	limit   int
	entries map[string]*pooledBackend
}

type pooledBackend struct {
	state *session
	refs  int
}

// attach retains one reference per frontend, not per request. The frontend lock
// serializes attachment with EOF/DELETE/idle cleanup. An empty, failed or closed
// frontend cannot resurrect a backend after its cleanup has run.
func (s *session) attach(p *backendPool, principal string) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return nil, errors.New("frontend closed")
	}
	if s.owner != nil {
		return s.owner, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.entries[principal]
	if entry == nil {
		ctx, cancel := context.WithCancel(p.ctx)
		entry = &pooledBackend{state: &session{ctx: ctx, cancel: cancel, calls: make(chan struct{}, p.limit)}}
		p.entries[principal] = entry
	}
	entry.refs++
	s.owner = entry.state
	s.release = func() { p.release(principal, entry) }
	return s.owner, nil
}

func (p *backendPool) release(principal string, entry *pooledBackend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry.refs--
	if entry.refs != 0 {
		return
	}
	entry.state.cancel()
	entry.state.mu.Lock()
	if entry.state.backend != nil {
		_ = entry.state.backend.Close()
	}
	entry.state.mu.Unlock()
	// Keep the pool locked through reap: a new frontend cannot overlap a dying
	// child for the same principal. Failed starts remain sticky until this point.
	delete(p.entries, principal)
}
