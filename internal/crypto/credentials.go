package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

type Store struct {
	key []byte
}

func NewStore(dataDir string) (*Store, error) {
	if env := strings.TrimSpace(os.Getenv("POLYCOM_MANAGER_MASTER_KEY")); env != "" {
		key, err := base64.StdEncoding.DecodeString(env)
		if err != nil || len(key) != chacha20poly1305.KeySize {
			return nil, fmt.Errorf("POLYCOM_MANAGER_MASTER_KEY must be base64 encoded %d-byte key", chacha20poly1305.KeySize)
		}
		return &Store{key: key}, nil
	}

	path := filepath.Join(dataDir, "master.key")
	if b, err := os.ReadFile(path); err == nil {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(key) != chacha20poly1305.KeySize {
			return nil, errors.New("invalid local master.key")
		}
		return &Store{key: key}, nil
	}

	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return nil, err
	}
	return &Store{key: key}, nil
}

func (s *Store) Encrypt(plaintext string) (ciphertext, nonce []byte, err error) {
	aead, err := chacha20poly1305.NewX(s.key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = aead.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}

func (s *Store) Decrypt(ciphertext, nonce []byte) (string, error) {
	aead, err := chacha20poly1305.NewX(s.key)
	if err != nil {
		return "", err
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
