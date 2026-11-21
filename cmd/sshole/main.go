package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kinabcd/sshole/internal/config"
	"github.com/kinabcd/sshole/internal/keys"
	"github.com/kinabcd/sshole/internal/server"
	"github.com/kinabcd/sshole/internal/svc"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const (
	serviceName        = "sshole"
	serviceDisplayName = "sshole"
	serviceDescription = "sshole SSH Server"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("TRIGGER: panic in main — process is exiting", "panic", r)
		}
	}()

	fs := flag.NewFlagSet("sshole", flag.ContinueOnError)
	fs.Usage = usage
	dirOverride := fs.String("dir", "", "config directory (overrides SSHOLE_CONFIG_DIR and the OS default)")
	listenOverride := fs.String("listen", "", "override the configured listen address (e.g. :22007)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println("sshole", version)
		return
	}
	args := fs.Args()

	if len(args) > 0 {
		var err error
		switch args[0] {
		case "svc":
			if len(args) != 2 {
				usage()
				os.Exit(2)
			}
			err = runSvcCommand(args[1], *dirOverride, *listenOverride)
		case "password":
			if len(args) != 2 {
				usage()
				os.Exit(2)
			}
			err = runPasswordCommand(*dirOverride, args[1])
		default:
			fmt.Fprintf(os.Stderr, "sshole: unknown command %q\n", args[0])
			usage()
			os.Exit(2)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "sshole:", err)
			os.Exit(1)
		}
		return
	}

	slog.Info("sshole starting", "version", version, "pid", os.Getpid())
	if err := svc.RunOrService(serviceName, func(ctx context.Context) error {
		return runServer(ctx, *dirOverride, *listenOverride)
	}); err != nil {
		slog.Error("TRIGGER: RunOrService returned error — process is exiting", "err", err)
		log.Fatalf("sshole: %v", err)
	}
	slog.Info("RunOrService returned nil — process exiting cleanly")
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  sshole [-dir path] [-listen host:port]                         run the server
  sshole [-dir path] [-listen host:port] svc install             install as a service (flags are kept)
  sshole svc uninstall|start|stop                                manage the service
  sshole [-dir path] password <password>                         set the login password
  sshole -version                                                print the version
`)
}

func runPasswordCommand(dirOverride, password string) error {
	dir := config.Dir(dirOverride)
	if err := config.SetPassword(dir, password); err != nil {
		return err
	}
	fmt.Printf("sshole: password updated in %s; restart sshole for it to take effect\n", dir)
	return nil
}

func runSvcCommand(cmd, dirOverride, listenOverride string) error {
	switch cmd {
	case "install":
		// The service starts with its own working directory, so a relative
		// -dir must be resolved now.
		var args []string
		if dirOverride != "" {
			abs, err := filepath.Abs(dirOverride)
			if err != nil {
				return fmt.Errorf("resolve -dir: %w", err)
			}
			args = append(args, "-dir", abs)
		}
		if listenOverride != "" {
			args = append(args, "-listen", listenOverride)
		}
		return svc.Install(serviceName, serviceDisplayName, serviceDescription, args...)
	case "uninstall":
		return svc.Uninstall(serviceName)
	case "start":
		return svc.Start(serviceName)
	case "stop":
		return svc.Stop(serviceName)
	default:
		usage()
		return fmt.Errorf("unknown svc command %q", cmd)
	}
}

func runServer(ctx context.Context, dirOverride, listenOverride string) error {
	dir := config.Dir(dirOverride)

	cfg, generatedPassword, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if listenOverride != "" {
		cfg.Listen = listenOverride
	}

	hostKey, err := keys.LoadOrGenerateHostKey(dir)
	if err != nil {
		return fmt.Errorf("load host key: %w", err)
	}

	authorizedKeys, err := keys.LoadAuthorizedKeys(dir)
	if err != nil {
		return fmt.Errorf("load authorized_keys: %w", err)
	}

	log.Printf("sshole: config directory: %s", dir)
	if generatedPassword != "" {
		log.Printf("sshole: generated initial password (won't be shown again): %s", generatedPassword)
	}
	log.Printf("sshole: %d authorized public key(s) loaded", len(authorizedKeys))

	sshConfig := server.BuildSSHConfig(hostKey, authorizedKeys, cfg)
	return server.New(cfg.Listen, sshConfig).Run(ctx)
}
