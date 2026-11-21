//go:build windows

package server

import (
	"strings"
	"syscall"
)

// verbatimCmdLine returns a SysProcAttr that makes the OS pass the command
// line to the child verbatim, bypassing Go's argument escaping.
//
// On Windows, exec.Command builds the CreateProcess command line by
// backslash-escaping every double quote in each argument (and wrapping args
// that contain spaces in quotes). That corrupts commands meant for cmd.exe,
// e.g. `cmd.exe /C powershell -Command "Write-Output hello"` becomes
// `cmd.exe /C "powershell -Command \"Write-Output hello\""`, and cmd.exe —
// which does not understand backslash escaping — mis-parses the mangled
// quotes, so the inner command is never executed as intended.
//
// Setting SysProcAttr.CmdLine makes the runtime use the string as-is for
// lpCommandLine (argv0 still supplies lpApplicationName), so the user's
// command reaches cmd.exe exactly as typed.
func verbatimCmdLine(name string, args []string) *syscall.SysProcAttr {
	var b strings.Builder
	if strings.ContainsAny(name, " \t") {
		b.WriteByte('"')
		b.WriteString(name)
		b.WriteByte('"')
	} else {
		b.WriteString(name)
	}
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(a)
	}
	return &syscall.SysProcAttr{CmdLine: b.String()}
}
