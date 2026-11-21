package server

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"

	"github.com/kinabcd/sshole/internal/config"
	"github.com/kinabcd/sshole/internal/keys"
)

// BuildSSHConfig assembles an *ssh.ServerConfig that authenticates against
// the given authorized keys and, if cfg has a password hash configured,
// a password. hostKey must already be registered via AddHostKey.
func BuildSSHConfig(hostKey ssh.Signer, authorizedKeys []ssh.PublicKey, cfg *config.ServerConfig) *ssh.ServerConfig {
	sshConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if keys.Contains(authorizedKeys, key) {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("unknown public key")
		},
	}

	if cfg.PasswordHash != "" {
		sshConfig.PasswordCallback = func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), password); err != nil {
				return nil, fmt.Errorf("invalid password")
			}
			return &ssh.Permissions{}, nil
		}
	}

	sshConfig.AddHostKey(hostKey)
	return sshConfig
}
