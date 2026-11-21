package svc

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

const unsupportedMsg = "sshole: svc %s is not supported on %s; run `sshole` directly, or manage it with your OS's own service manager (e.g. systemd).\n"

func Install(name, displayName, description string, args ...string) error {
	fmt.Printf(unsupportedMsg, "install", runtime.GOOS)
	return nil
}

func Uninstall(name string) error {
	fmt.Printf(unsupportedMsg, "uninstall", runtime.GOOS)
	return nil
}

func Start(name string) error {
	fmt.Printf(unsupportedMsg, "start", runtime.GOOS)
	return nil
}

func Stop(name string) error {
	fmt.Printf(unsupportedMsg, "stop", runtime.GOOS)
	return nil
}

// RunOrService runs fn directly in the foreground until an interrupt or
// termination signal is received.
func RunOrService(name string, fn ServerFunc) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		cancel()
	}()

	return fn(ctx)
}
