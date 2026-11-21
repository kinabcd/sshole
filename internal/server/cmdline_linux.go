//go:build linux

package server

import "syscall"

// verbatimCmdLine is a no-op on Linux: the kernel passes argv to the child
// as a real array, so there is no command-line string to escape and no
// quoting bug to work around.
func verbatimCmdLine(name string, args []string) *syscall.SysProcAttr {
	return nil
}
