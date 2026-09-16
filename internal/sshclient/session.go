package sshclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type NotificationHandler func(line string)

type Session struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	lines   chan string
	done    chan struct{}

	execMu   sync.Mutex
	onNotify NotificationHandler
}

type DialConfig struct {
	Address  string
	Username string
	Password string
	Timeout  time.Duration
	HostKeys *HostKeyStore
	OnNotify NotificationHandler
}

func Dial(ctx context.Context, cfg DialConfig) (*Session, error) {
	sshCfg := &ssh.ClientConfig{
		User:    cfg.Username,
		Auth:    []ssh.AuthMethod{ssh.Password(cfg.Password)},
		Timeout: cfg.Timeout,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			if cfg.HostKeys == nil {
				return errors.New("host key store is required")
			}
			return cfg.HostKeys.Check(hostname, key)
		},
	}
	dialer := net.Dialer{Timeout: cfg.Timeout}
	netConn, err := dialer.DialContext(ctx, "tcp", cfg.Address)
	if err != nil {
		return nil, err
	}
	cc, chans, reqs, err := ssh.NewClientConn(netConn, cfg.Address, sshCfg)
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	client := ssh.NewClient(cc, chans, reqs)
	sess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, err
	}

	_ = sess.RequestPty("vt100", 80, 120, ssh.TerminalModes{ssh.ECHO: 0})
	if err := sess.Shell(); err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, err
	}

	s := &Session{client: client, session: sess, stdin: stdin, lines: make(chan string, 256), done: make(chan struct{}), onNotify: cfg.OnNotify}
	go s.readLoop(io.MultiReader(stdout, stderr))
	// Let banners/prompts arrive, then discard them so the first command result is clean.
	t := time.NewTimer(350 * time.Millisecond)
	select {
	case <-ctx.Done():
		t.Stop()
		s.Close()
		return nil, ctx.Err()
	case <-t.C:
	}
	s.drain()
	return s, nil
}

func (s *Session) readLoop(r io.Reader) {
	defer close(s.done)
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimRight(scanner.Text(), "\r"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "notification:") ||
			strings.HasPrefix(lower, "control event:") {
			if s.onNotify != nil {
				s.onNotify(line)
			}
			continue
		}
		select {
		case s.lines <- line:
		default:
		}
	}
}

func (s *Session) drain() {
	for {
		select {
		case <-s.lines:
			continue
		default:
			return
		}
	}
}

func (s *Session) Execute(ctx context.Context, command string) ([]string, error) {
	return s.execute(ctx, command, 6*time.Second, false)
}

// ExecuteOptional is for documented API commands that may not emit a response.
// It captures a response if one arrives during wait, but treats an empty response
// as success when that short grace period expires.
func (s *Session) ExecuteOptional(ctx context.Context, command string, wait time.Duration) ([]string, error) {
	if wait <= 0 {
		wait = 500 * time.Millisecond
	}
	return s.execute(ctx, command, wait, true)
}

func (s *Session) execute(ctx context.Context, command string, responseTimeout time.Duration, allowNoResponse bool) ([]string, error) {
	s.execMu.Lock()
	defer s.execMu.Unlock()
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errors.New("empty command")
	}
	if strings.ContainsAny(command, "\r\n\x00") {
		return nil, errors.New("command contains control characters")
	}
	s.drain()
	if _, err := io.WriteString(s.stdin, command+"\r"); err != nil {
		return nil, err
	}

	deadline := time.NewTimer(responseTimeout)
	defer deadline.Stop()
	var idle *time.Timer
	var idleC <-chan time.Time
	var lines []string
	for {
		select {
		case <-ctx.Done():
			return lines, ctx.Err()
		case <-s.done:
			return lines, io.EOF
		case <-deadline.C:
			if len(lines) > 0 {
				return clean(lines, command), nil
			}
			if allowNoResponse {
				return nil, nil
			}
			return nil, fmt.Errorf("timeout waiting for response to %q", command)
		case line := <-s.lines:
			lines = append(lines, line)
			if idle == nil {
				idle = time.NewTimer(220 * time.Millisecond)
			} else {
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(220 * time.Millisecond)
			}
			idleC = idle.C
		case <-idleC:
			return clean(lines, command), nil
		}
	}
}

func clean(lines []string, command string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || t == command || t == ">" || t == "#" {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (s *Session) Close() error {
	if s.session != nil {
		_ = s.session.Close()
	}
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unable to authenticate") || strings.Contains(s, "no supported methods remain") || strings.Contains(s, "permission denied")
}
