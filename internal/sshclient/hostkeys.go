package sshclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
)

type HostKeyStore struct {
	mu   sync.Mutex
	path string
	keys map[string]string
}

func NewHostKeyStore(dataDir string) (*HostKeyStore, error) {
	s := &HostKeyStore{path: filepath.Join(dataDir, "hostkeys.json"), keys: map[string]string{}}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.keys); err != nil {
			return nil, fmt.Errorf("parse hostkeys: %w", err)
		}
	}
	return s, nil
}

func (s *HostKeyStore) Callback(hostname string, _ any, key ssh.PublicKey) error {
	return s.Check(hostname, key)
}

func (s *HostKeyStore) Check(hostname string, key ssh.PublicKey) error {
	fingerprint := ssh.FingerprintSHA256(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.keys[hostname]; ok {
		if old != fingerprint {
			return fmt.Errorf("SSH host key changed for %s: expected %s, got %s", hostname, old, fingerprint)
		}
		return nil
	}
	s.keys[hostname] = fingerprint
	b, _ := json.MarshalIndent(s.keys, "", "  ")
	if err := os.WriteFile(s.path, b, 0o600); err != nil {
		delete(s.keys, hostname)
		return err
	}
	return nil
}

func (s *HostKeyStore) Delete(hostname string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, hostname)
	b, _ := json.MarshalIndent(s.keys, "", "  ")
	return os.WriteFile(s.path, b, 0o600)
}
