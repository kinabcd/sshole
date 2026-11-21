//go:build windows

package server

// defaultShellPath returns the interactive shell to launch for a plain
// "sshole" session on Windows.
func defaultShellPath() string {
	return "cmd.exe"
}

// oneShotCommand builds the argv used to run a one-shot exec command on
// Windows: `cmd.exe /C <command>`. To run PowerShell instead, the caller
// simply includes it in command, e.g. `powershell -Command "..."`.
func oneShotCommand(command string) (string, []string) {
	return "cmd.exe", []string{"/C", command}
}
