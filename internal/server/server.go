package server

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// shutdownTimeout bounds how long Run waits for in-flight sessions to
// finish after ctx is canceled before giving up.
const shutdownTimeout = 5 * time.Second

type Server struct {
	listen    string
	sshConfig *ssh.ServerConfig
}

func New(listen string, sshConfig *ssh.ServerConfig) *Server {
	return &Server{listen: listen, sshConfig: sshConfig}
}

// Run listens for SSH connections and serves them until ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.listen, err)
	}
	slog.Info("sshole: listening", "addr", ln.Addr().String())

	shuttingDown := make(chan struct{})
	go func() {
		<-ctx.Done()
		slog.Error("TRIGGER: server ctx canceled (context.WithCancel cancel() called) — closing listener")
		close(shuttingDown)
		ln.Close()
	}()

	var wg sync.WaitGroup
	for {
		nc, err := ln.Accept()
		if err != nil {
			select {
			case <-shuttingDown:
				slog.Info("accept loop exiting due to shutdown (ctx canceled), waiting for in-flight sessions")
				waitWithTimeout(&wg, shutdownTimeout)
				return nil
			default:
				slog.Error("TRIGGER: accept loop exiting due to unexpected error", "err", err)
				return fmt.Errorf("accept: %w", err)
			}
		}
		wg.Go(func() {
			handleConn(nc, s.sshConfig)
		})
	}
}

func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("sshole: shutdown timed out waiting for active sessions")
	}
}

func handleConn(nc net.Conn, cfg *ssh.ServerConfig) {
	defer nc.Close()

	conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		log.Printf("sshole: handshake failed from %s: %v", nc.RemoteAddr(), err)
		return
	}
	defer conn.Close()
	log.Printf("sshole: connection from %s (user=%q)", conn.RemoteAddr(), conn.User())

	go ssh.DiscardRequests(reqs)

	var wg sync.WaitGroup
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		ch, requests, err := newCh.Accept()
		if err != nil {
			log.Printf("sshole: accept channel: %v", err)
			continue
		}
		wg.Go(func() {
			handleSession(ch, requests)
		})
	}
	wg.Wait()
	log.Printf("sshole: connection from %s closed", conn.RemoteAddr())
}
