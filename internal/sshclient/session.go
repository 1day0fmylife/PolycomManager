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
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	commandTimeout          = 6 * time.Second
	responseIdleTimeout     = 50 * time.Millisecond
	optionalIdleTimeout     = 20 * time.Millisecond
	optionalResponseTimeout = 80 * time.Millisecond
)

type NotificationHandler func(line string)

type Session struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	lines   chan string
	done    chan struct{}

	execMu             sync.Mutex
	interactiveWaiters atomic.Int32
	onNotify           NotificationHandler
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
		if strings.HasPrefix(lower, "notification:") || strings.HasPrefix(lower, "control event:") {
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

func validateCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("empty command")
	}
	if strings.ContainsAny(command, "\r\n\x00") {
		return "", errors.New("command contains control characters")
	}
	return command, nil
}

func isLatencySensitive(command string) bool {
	c := strings.ToLower(strings.TrimSpace(command))
	for _, prefix := range []string{
		"camera ", "preset ", "gendial ", "volume set ", "mute near ",
		"dial ", "hangup ", "vcbutton play", "vcbutton stop",
	} {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (s *Session) lockCommand(ctx context.Context, command string) error {
	interactive := isLatencySensitive(command)
	if interactive {
		s.interactiveWaiters.Add(1)
		defer s.interactiveWaiters.Add(-1)
		s.execMu.Lock()
		return nil
	}
	for s.interactiveWaiters.Load() > 0 {
		t := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	s.execMu.Lock()
	return nil
}

func (s *Session) Execute(ctx context.Context, command string) ([]string, error) {
	return s.execute(ctx, command, commandTimeout, responseIdleTimeout, false)
}

func (s *Session) ExecuteOptional(ctx context.Context, command string, wait time.Duration) ([]string, error) {
	if wait <= 0 {
		wait = optionalResponseTimeout
	}
	return s.execute(ctx, command, wait, optionalIdleTimeout, true)
}

func (s *Session) ExecuteNoWait(ctx context.Context, command string) error {
	command, err := validateCommand(command)
	if err != nil {
		return err
	}
	if err := s.lockCommand(ctx, command); err != nil {
		return err
	}
	defer s.execMu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.drain()
	_, err = io.WriteString(s.stdin, command+"\r")
	return err
}

func (s *Session) execute(ctx context.Context, command string, responseTimeout, idleTimeout time.Duration, allowNoResponse bool) ([]string, error) {
	command, err := validateCommand(command)
	if err != nil {
		return nil, err
	}
	if err := s.lockCommand(ctx, command); err != nil {
		return nil, err
	}
	defer s.execMu.Unlock()

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
				idle = time.NewTimer(idleTimeout)
			} else {
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(idleTimeout)
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
