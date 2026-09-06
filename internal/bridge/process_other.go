//go:build !linux && !darwin

package bridge

import (
	"errors"
	"os/exec"
)

func configureProcess(*exec.Cmd) error {
	return errors.New("stdio process containment requires Linux or macOS; use WSL on Windows")
}
func terminateProcessGroup(*exec.Cmd, bool) {}
