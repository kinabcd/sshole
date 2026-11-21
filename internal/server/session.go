package server

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	pty "github.com/aymanbagabas/go-pty"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// See RFC 4254 6.2, 6.5 and 6.7 for the wire format of these request
// payloads.
type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	Modelist string
}

type windowChangeMsg struct {
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

type execMsg struct {
	Command string
}

type exitStatusMsg struct {
	Status uint32
}

// handleSession services all out-of-band requests for a single "session"
// channel: pty-req, window-change, shell/exec (RFC 4254 6.2/6.5/6.7), and
// replies to anything else it doesn't understand.
func handleSession(ch ssh.Channel, in <-chan *ssh.Request) {
	defer ch.Close()

	var (
		mu        sync.Mutex
		activePty pty.Pty
		havePty   bool
		ptyReq    ptyRequestMsg
	)
	done := make(chan struct{})

	for {
		select {
		case req, ok := <-in:
			if !ok {
				return
			}

			switch req.Type {
			case "pty-req":
				if err := ssh.Unmarshal(req.Payload, &ptyReq); err != nil {
					reply(req, false)
					continue
				}
				havePty = true
				reply(req, true)

			case "window-change":
				var wc windowChangeMsg
				if err := ssh.Unmarshal(req.Payload, &wc); err == nil {
					mu.Lock()
					if activePty != nil {
						_ = activePty.Resize(int(wc.Columns), int(wc.Rows))
					}
					mu.Unlock()
				}
				// window-change never carries a reply.

			case "shell", "exec":
				var command string
				isExec := req.Type == "exec"
				if isExec {
					var m execMsg
					if err := ssh.Unmarshal(req.Payload, &m); err != nil {
						reply(req, false)
						continue
					}
					command = m.Command
				}
				reply(req, true)
				slog.Info("session started", "type", req.Type, "pty", havePty, "command", command)

				go func() {
					defer close(done)
					code := run(ch, command, isExec, havePty, ptyReq, &mu, &activePty)
					slog.Info("session command finished (normal session end, NOT a server termination trigger)", "type", req.Type, "exit", code)
					sendExitStatus(ch, code)
				}()

			case "subsystem":
				var m struct{ Name string }
				if err := ssh.Unmarshal(req.Payload, &m); err != nil || m.Name != "sftp" {
					reply(req, false)
					continue
				}
				reply(req, true)
				slog.Info("session started", "type", "subsystem", "name", m.Name)
				go func() {
					defer close(done)
					// Full filesystem access (no RootFS chroot).
					srv, err := sftp.NewServer(ch)
					if err != nil {
						slog.Error("sftp server init failed", "err", err)
						sendExitStatus(ch, 1)
						return
					}
					srv.Serve()
					sendExitStatus(ch, 0)
				}()

			default:
				// Includes "env" and anything else we don't act on.
				reply(req, false)
			}

		case <-done:
			return
		}
	}
}

func reply(req *ssh.Request, ok bool) {
	if req.WantReply {
		_ = req.Reply(ok, nil)
	}
}

func run(ch ssh.Channel, command string, isExec, havePty bool, ptyReq ptyRequestMsg, mu *sync.Mutex, activePty *pty.Pty) int {
	var (
		name string
		args []string
	)
	if isExec {
		name, args = oneShotCommand(command)
	} else {
		name = defaultShellPath()
	}

	if havePty {
		return runWithPty(ch, name, args, ptyReq, mu, activePty)
	}
	return runPlain(ch, name, args)
}

func runWithPty(ch ssh.Channel, name string, args []string, ptyReq ptyRequestMsg, mu *sync.Mutex, activePty *pty.Pty) int {
	p, err := pty.New()
	if err != nil {
		fmt.Fprintf(ch.Stderr(), "sshole: failed to allocate pty: %v\n", err)
		return 1
	}

	cols, rows := int(ptyReq.Columns), int(ptyReq.Rows)
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	if err := p.Resize(cols, rows); err != nil {
		log.Printf("sshole: resize pty: %v", err)
	}

	mu.Lock()
	*activePty = p
	mu.Unlock()
	defer func() {
		mu.Lock()
		*activePty = nil
		mu.Unlock()
	}()

	cmd := p.Command(name, args...)
	// Pass the command line verbatim (no quote escaping) so cmd.exe sees
	// the user's command exactly as typed.
	cmd.SysProcAttr = verbatimCmdLine(name, args)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(ch.Stderr(), "sshole: failed to start %s: %v\n", name, err)
		p.Close()
		return 1
	}

	var (
		outMu       sync.Mutex
		outW        io.Writer = ch
		outWg       sync.WaitGroup
		childExited bool
	)
	outWg.Go(func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := p.Read(buf)
			if n > 0 {
				outMu.Lock()
				if childExited {
					slog.Info("pty POST-EXIT bytes (DIAGNOSTIC)", "n", n, "data", fmt.Sprintf("%q", buf[:n]))
				}
				_, _ = outW.Write(buf[:n])
				outMu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	})
	// stdin direction is intentionally not waited on: after the child exits
	// it blocks in ch.Read() waiting for client data/EOF that never comes for
	// exec (the client waits for our exit-status + channel close instead).
	// It exits on its own when handleSession closes the channel.
	go func() { io.Copy(p, ch) }()

	err = cmd.Wait()
	slog.Info("pty child process exited (pty session ending, server should keep running)", "name", name, "err", err)

	// Mark the child as exited immediately so the diagnostic catches ALL
	// bytes read after the child exits (including the conpty final flush,
	// wherever it arrives).
	outMu.Lock()
	childExited = true
	outMu.Unlock()

	// Give the child's final output a moment to reach the pty, then discard
	// everything else: on Windows, ClosePseudoConsole flushes the final
	// screen state (cursor position, scroll region, ...) into the pty
	// output, and forwarding that garbage to the client corrupts the
	// client's terminal.
	time.Sleep(100 * time.Millisecond)
	outMu.Lock()
	outW = io.Discard
	outMu.Unlock()

	slog.Info("pty teardown: about to call p.Close()")
	p.Close()
	slog.Info("pty teardown: p.Close() returned, waiting for stdout copy")

	outWg.Wait()

	// Conpty sends ESC[?9001h (enable synchronized output) and ESC[?1004h
	// (enable mouse tracking) at initialization, and the corresponding
	// disable sequences (ESC[?9001l, ESC[?1004l) at teardown. We discard
	// the teardown sequences (along with the screen state dump), so we
	// must send them ourselves to prevent the client's terminal from
	// getting stuck in synchronized output mode.
	_, _ = ch.Write([]byte("\x1b[?9001l\x1b[?1004l"))

	slog.Info("pty teardown: stdout copy done, returning from runWithPty")
	return exitCodeFromError(err)
}

func runPlain(ch ssh.Channel, name string, args []string) int {
	cmd := exec.Command(name, args...)
	// Pass the command line verbatim (no quote escaping) so cmd.exe sees
	// the user's command exactly as typed.
	cmd.SysProcAttr = verbatimCmdLine(name, args)
	cmd.Stdout = ch
	cmd.Stderr = ch.Stderr()

	// Feed stdin through a pipe instead of setting cmd.Stdin = ch directly.
	// ch is an io.Reader (not *os.File), so exec would spawn its own copy
	// goroutine and cmd.Wait() would block on it until the client sends EOF -
	// which OpenSSH never does for exec (it waits for our exit-status +
	// channel close). The pipe is an *os.File, so exec passes it straight to
	// the child and Wait() only waits for the process. Our copy goroutine is
	// abandoned; it exits when handleSession closes the channel.
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(ch.Stderr(), "sshole: failed to create stdin pipe: %v\n", err)
		return 1
	}
	cmd.Stdin = stdinR
	go func() {
		io.Copy(stdinW, ch)
		stdinW.Close()
	}()

	if err := cmd.Start(); err != nil {
		stdinR.Close()
		stdinW.Close()
		fmt.Fprintf(ch.Stderr(), "sshole: failed to start %s: %v\n", name, err)
		return 1
	}
	return exitCodeFromError(cmd.Wait())
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func sendExitStatus(ch ssh.Channel, code int) {
	_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(&exitStatusMsg{Status: uint32(code)}))
}
