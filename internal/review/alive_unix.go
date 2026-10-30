//go:build !windows

package review

import (
	"errors"
	"os"
	"syscall"
)

// alive reports whether a process is running. It is a variable so tests
// can stand in for a hook that has exited.
var alive = func(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
