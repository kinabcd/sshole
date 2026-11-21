//go:build linux

package server

import "os/exec"

// defaultShellPath returns the interactive shell to launch for a plain
// "sshole" session on Linux: bash if available, otherwise sh.
func defaultShellPath() string {
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	if p, err := exec.LookPath("sh"); err == nil {
		return p
	}
	return "/bin/sh"
}

// oneShotCommand builds the argv used to run a one-shot exec command on
// Linux: `bash -c <command>` (or `sh -c <command>` if bash is unavailable).
func oneShotCommand(command string) (string, []string) {
	return defaultShellPath(), []string{"-c", command}
}
