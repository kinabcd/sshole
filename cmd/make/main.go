// Command make builds the sshole binary.
//
// Usage:
//
//	go tool make          # build out/sshole.exe for Windows (default)
//	go tool make linux    # build out/sshole for Linux
//	go tool make clean    # remove build artifacts
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	target := "windows"
	if len(os.Args) > 1 {
		target = os.Args[1]
	}

	switch target {
	case "windows":
		build("windows", "out/sshole.exe")
	case "linux":
		build("linux", "out/sshole")
	case "clean":
		if err := os.RemoveAll("out"); err == nil {
			fmt.Println("removed out/")
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown target %q (use: windows, linux, clean)\n", target)
		os.Exit(2)
	}
}

func build(goos, out string) {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}
	cmd := exec.Command("go", "build", "-o", out, "./cmd/sshole")
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+goos,
		"GOARCH=amd64",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("built", out)
}
