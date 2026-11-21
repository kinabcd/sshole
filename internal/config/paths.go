package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the OS-appropriate directory for sshole server state: the host
// key, authorized_keys and config.json.
//
// Windows: %ProgramData%\sshole
// Linux:   /etc/sshole
//
// override (from the -dir flag) takes precedence when non-empty, followed by
// the SSHOLE_CONFIG_DIR environment variable. Both are mainly useful for
// local testing without root/Administrator privileges.
func Dir(override string) string {
	if override != "" {
		return override
	}
	if dir := os.Getenv("SSHOLE_CONFIG_DIR"); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "sshole")
	}
	return "/etc/sshole"
}
