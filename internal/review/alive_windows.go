package review

import "os"

// alive reports whether a process is running: on Windows, finding a
// process opens it, which fails once it has exited. It is a variable so
// tests can stand in for a hook that has exited.
var alive = func(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}
