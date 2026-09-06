//go:build linux || darwin

package bridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
)

func TestProcessTreeFixture(t *testing.T) {
	mode := os.Getenv("BRIDGE_TREE_TEST")
	if mode == "" {
		return
	}
	if mode == "last-response" {
		fmt.Fprintf(os.Stdout, "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":\"%s\"}\n", strings.Repeat("x", 32768))
		os.Exit(0)
	}
	signal.Ignore(syscall.SIGTERM)
	if mode == "parent" {
		exe, _ := os.Executable()
		cmd := exec.Command(exe, "-test.run=^TestProcessTreeFixture$")
		cmd.Env = append(childEnv(nil), "BRIDGE_TREE_TEST=child")
		if cmd.Start() != nil {
			os.Exit(2)
		}
		_ = os.WriteFile(os.Getenv("BRIDGE_PID_FILE"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600)
		go cmd.Wait()
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestExitedChildLastResponsePreserved(t *testing.T) {
	exe, _ := os.Executable()
	for i := 0; i < 20; i++ {
		transport := &processTransport{limit: 65536, definition: config.Definition{Command: exe, Args: []string{"-test.run=^TestProcessTreeFixture$"}, Env: map[string]string{"BRIDGE_TREE_TEST": "last-response"}}}
		c, e := transport.Connect(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		_, e = c.Read(context.Background())
		c.Close()
		if e != nil {
			t.Fatalf("last response lost on iteration %d: %v", i, e)
		}
	}
}
func processAlive(pid int) bool {
	return processAliveWith(pid, runtime.GOOS, os.ReadFile, syscall.Kill)
}

func processAliveWith(pid int, goos string, readStat func(string) ([]byte, error), signalProcess func(int, syscall.Signal) error) bool {
	if goos == "linux" {
		// One /proc observation avoids kill(0) succeeding just before a zombie
		// is reaped and its stat file disappears. Missing/ESRCH means exited.
		b, err := readStat("/proc/" + strconv.Itoa(pid) + "/stat")
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			return false
		}
		if err == nil {
			if i := strings.LastIndex(string(b), ") "); i >= 0 && i+2 < len(b) {
				return b[i+2] != 'Z' && b[i+2] != 'X'
			}
		}
	}
	// macOS has no /proc. Permission/unknown failures do not prove exit.
	return !errors.Is(signalProcess(pid, 0), syscall.ESRCH)
}
func TestProcessGroupCleanup(t *testing.T) {
	exe, _ := os.Executable()
	path := filepath.Join(t.TempDir(), "child.pid")
	transport := &processTransport{limit: 1024, definition: config.Definition{Command: exe, Args: []string{"-test.run=^TestProcessTreeFixture$"}, Env: map[string]string{"BRIDGE_TREE_TEST": "parent", "BRIDGE_PID_FILE": path}}}
	c, e := transport.Connect(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b, e := os.ReadFile(path); e == nil {
			pid, _ = strconv.Atoi(string(b))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child did not start")
	}
	if !processAlive(pid) {
		t.Fatal("fixture child absent before close")
	}
	c.Close()
	deadline = time.Now().Add(3 * time.Second)
	for processAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processAlive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("descendant survived close")
	}
}
