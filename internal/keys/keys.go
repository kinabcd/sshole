package keys

import (
	"bytes"
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// LoadOrGenerateHostKey loads the server's ed25519 host key from dir,
// generating and persisting a new one if none exists yet.
func LoadOrGenerateHostKey(dir string) (ssh.Signer, error) {
	path := filepath.Join(dir, "host_ed25519_key")

	if data, err := os.ReadFile(path); err == nil {
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parse host key %s: %w", path, err)
		}
		return signer, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read host key %s: %w", path, err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "sshole host key")
	if err != nil {
		return nil, fmt.Errorf("marshal host key: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, fmt.Errorf("write host key %s: %w", path, err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("load generated host key: %w", err)
	}
	return signer, nil
}

// LoadAuthorizedKeys reads authorized_keys from dir, creating an empty file
// if one doesn't exist yet (mirroring OpenSSH's convention: an admin adds
// trusted public keys to this file by hand).
func LoadAuthorizedKeys(dir string) ([]ssh.PublicKey, error) {
	path := filepath.Join(dir, "authorized_keys")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
			return nil, fmt.Errorf("create %s: %w", dir, mkErr)
		}
		if wErr := os.WriteFile(path, nil, 0o600); wErr != nil {
			return nil, fmt.Errorf("create %s: %w", path, wErr)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var authorized []ssh.PublicKey
	for len(bytes.TrimSpace(data)) > 0 {
		key, _, _, rest, err := ssh.ParseAuthorizedKey(data)
		if err != nil {
			break
		}
		authorized = append(authorized, key)
		data = rest
	}
	return authorized, nil
}

// Contains reports whether key matches one of the authorized keys.
func Contains(authorized []ssh.PublicKey, key ssh.PublicKey) bool {
	marshaled := key.Marshal()
	for _, k := range authorized {
		if bytes.Equal(k.Marshal(), marshaled) {
			return true
		}
	}
	return false
}
