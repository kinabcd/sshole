//go:build windows

package svc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows"
	winsvc "golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// consoleControlHandler is the callback passed to SetConsoleCtrlHandler. It
// intercepts console control events (Ctrl+C, Ctrl+Break, Ctrl+Close, and the
// "close console window" event) so we can log which one terminates the
// process. It returns 0 (FALSE) so the default handling still proceeds.
func consoleControlHandler(event uintptr) uintptr {
	switch event {
	case windows.CTRL_C_EVENT:
		slog.Error("TRIGGER: console control event = CTRL_C_EVENT (Ctrl+C pressed in console)")
	case windows.CTRL_BREAK_EVENT:
		slog.Error("TRIGGER: console control event = CTRL_BREAK_EVENT (Ctrl+Break pressed in console)")
	case windows.CTRL_CLOSE_EVENT:
		slog.Error("TRIGGER: console control event = CTRL_CLOSE_EVENT (console window closed / Ctrl+Z+Ctrl+C)")
	case windows.CTRL_LOGOFF_EVENT:
		slog.Error("TRIGGER: console control event = CTRL_LOGOFF_EVENT (user logging off)")
	case windows.CTRL_SHUTDOWN_EVENT:
		slog.Error("TRIGGER: console control event = CTRL_SHUTDOWN_EVENT (system shutting down)")
	default:
		slog.Error("TRIGGER: console control event = unknown", "event", fmt.Sprintf("0x%x", event))
	}
	return 0
}

// ctrlHandlerCallback holds the C callback pointer for the console control
// handler. It must live for the entire process lifetime: if it were garbage
// collected, Windows would call freed memory the next time a control event
// fires.
var ctrlHandlerCallback uintptr

// installConsoleControlHandler registers consoleControlHandler with the
// console via kernel32!SetConsoleCtrlHandler.
func installConsoleControlHandler() error {
	ctrlHandlerCallback = syscall.NewCallback(consoleControlHandler)
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	// SetConsoleCtrlHandler returns BOOL (TRUE on success). The syscall
	// wrapper's error value is unreliable for BOOL-returning functions, so we
	// check the raw return value directly.
	r1, _, _ := proc.Call(ctrlHandlerCallback, 1)
	if r1 == 0 {
		return fmt.Errorf("SetConsoleCtrlHandler: failed (returned FALSE)")
	}
	return nil
}

// Install registers the current executable as an auto-start service. args
// are appended to the service's command line and show up in os.Args when the
// service starts.
func Install(name, displayName, description string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	if existing, err := m.OpenService(name); err == nil {
		existing.Close()
		return fmt.Errorf("service %q already exists", name)
	}

	s, err := m.CreateService(name, exe, mgr.Config{
		DisplayName: displayName,
		Description: description,
		StartType:   mgr.StartAutomatic,
	}, args...)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()
	return nil
}

func Uninstall(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service %q: %w", name, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

func Start(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service %q: %w", name, err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

func Stop(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service %q: %w", name, err)
	}
	defer s.Close()

	if _, err := s.Control(winsvc.Stop); err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	return nil
}

// RunOrService runs fn directly when launched interactively, or hands
// control to the Windows Service Control Manager when launched as a
// service, translating Stop/Shutdown control requests into ctx cancellation.
func RunOrService(name string, fn ServerFunc) error {
	isService, err := winsvc.IsWindowsService()
	if err != nil {
		isService = false
	}
	if !isService {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Install a console control handler first so we can log exactly which
		// console event (Ctrl+C, window close, logoff, ...) terminates the
		// process. This is the most likely trigger when the server is run from
		// a console window.
		if err := installConsoleControlHandler(); err != nil {
			slog.Warn("installConsoleControlHandler failed", "err", err)
		} else {
			slog.Info("console control handler installed (interactive mode)")
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		defer signal.Stop(sigCh)

		go func() {
			sig := <-sigCh
			slog.Error("TRIGGER: Go signal received (os.Interrupt via signal.Notify)", "signal", sig)
			cancel()
		}()

		return fn(ctx)
	}
	slog.Info("running as Windows service", "name", name)
	return winsvc.Run(name, &handler{fn: fn})
}

type handler struct {
	fn ServerFunc
}

func (h *handler) Execute(args []string, r <-chan winsvc.ChangeRequest, s chan<- winsvc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.fn(ctx)
	}()

	s <- winsvc.Status{State: winsvc.StartPending}
	s <- winsvc.Status{State: winsvc.Running, Accepts: winsvc.AcceptStop | winsvc.AcceptShutdown}
	slog.Info("service handler running, waiting for control requests")

	for {
		select {
		case err := <-errCh:
			slog.Info("service handler fn returned", "err", err)
			s <- winsvc.Status{State: winsvc.Stopped}
			if err != nil {
				return false, 1
			}
			return false, 0

		case cr := <-r:
			switch cr.Cmd {
			case winsvc.Interrogate:
				slog.Info("service control request = Interrogate (status query, not a termination trigger)")
				s <- cr.CurrentStatus
			case winsvc.Stop, winsvc.Shutdown:
				slog.Error("TRIGGER: service control request (Stop/Shutdown from SCM)", "cmd", cr.Cmd)
				s <- winsvc.Status{State: winsvc.StopPending}
				cancel()
				<-errCh
				s <- winsvc.Status{State: winsvc.Stopped}
				return false, 0
			default:
				slog.Info("service control request = unknown", "cmd", fmt.Sprintf("0x%x", cr.Cmd))
			}
		}
	}
}
