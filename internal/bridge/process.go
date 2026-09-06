package bridge

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type processTransport struct {
	definition config.Definition
	limit      int64
}
type processConnection struct {
	mcp.Connection
	cmd   *exec.Cmd
	stdin io.Closer
	done  chan struct{}
	once  sync.Once
}

func (t *processTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	d := t.definition
	cmd := exec.Command(d.Command, d.Args...)
	cmd.Dir = d.Cwd
	cmd.Env = childEnv(d.Env)
	cmd.Stderr = io.Discard
	if e := configureProcess(cmd); e != nil {
		return nil, e
	}
	stdout, childOut, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	childIn, stdin, e := os.Pipe()
	if e != nil {
		stdout.Close()
		childOut.Close()
		return nil, e
	}
	cmd.Stdout = childOut
	cmd.Stdin = childIn
	if e = cmd.Start(); e != nil {
		stdout.Close()
		childOut.Close()
		childIn.Close()
		stdin.Close()
		return nil, e
	}
	// Parent owns read/write endpoints independently of cmd.Wait's descriptor lifecycle.
	_ = childOut.Close()
	_ = childIn.Close()
	c, e := (&mcp.IOTransport{Reader: &lineBound{ReadCloser: stdout, limit: t.limit}, Writer: stdin}).Connect(ctx)
	p := &processConnection{Connection: c, cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	if e != nil {
		p.Close()
		return nil, errors.New("cannot connect child IO")
	}
	return p, nil
}
func (p *processConnection) Close() error {
	p.once.Do(func() {
		_ = p.stdin.Close()
		select {
		case <-p.done:
		case <-time.After(200 * time.Millisecond):
		}
		terminateProcessGroup(p.cmd, false)
		select {
		case <-p.done:
		case <-time.After(200 * time.Millisecond):
		}
		// Also kill descendants when the launcher exits before they do.
		terminateProcessGroup(p.cmd, true)
		if p.Connection != nil {
			_ = p.Connection.Close()
		}
		select {
		case <-p.done:
		case <-time.After(time.Second):
		}
	})
	return nil
}
