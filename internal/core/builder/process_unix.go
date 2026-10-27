//go:build !windows

package builder

import (
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
}
func containProcess(cmd *exec.Cmd) (func(), error) {
	var once sync.Once
	return func() { once.Do(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }) }, nil
}
