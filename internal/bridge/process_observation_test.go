//go:build linux || darwin

package bridge

import (
	"os"
	"syscall"
	"testing"
)

func TestProcessAliveObservations(t *testing.T) {
	for _, tc := range []struct {
		name, goos, stat   string
		readErr, signalErr error
		want               bool
		wantSignalCalls    int
	}{
		// The old implementation returned true for these reaping races after
		// kill(pid, 0) had succeeded; the process was already gone.
		{"reaped-before-open", "linux", "", os.ErrNotExist, nil, false, 0},
		{"exited-during-read", "linux", "", syscall.ESRCH, nil, false, 0},
		{"zombie", "linux", "123 (fixture) Z 1", nil, nil, false, 0},
		{"dead", "linux", "123 (fixture) X 1", nil, nil, false, 0},
		{"running", "linux", "123 (fixture) R 1", nil, nil, true, 0},
		{"permission-unknown", "linux", "", os.ErrPermission, syscall.EPERM, true, 1},
		{"darwin-running", "darwin", "", nil, nil, true, 1},
		{"darwin-exited", "darwin", "", nil, syscall.ESRCH, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			alive := processAliveWith(123, tc.goos, func(string) ([]byte, error) {
				if tc.goos != "linux" {
					t.Fatal("non-Linux process queried through /proc")
				}
				return []byte(tc.stat), tc.readErr
			}, func(int, syscall.Signal) error { calls++; return tc.signalErr })
			if alive != tc.want || calls != tc.wantSignalCalls {
				t.Fatalf("alive=%v signalCalls=%d; want %v/%d", alive, calls, tc.want, tc.wantSignalCalls)
			}
		})
	}
}
