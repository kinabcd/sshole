package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
)

// DefaultListen is used when no config.json exists yet.
const DefaultListen = ":22007"

type ServerConfig struct {
	Listen       string `json:"listen"`
	PasswordHash string `json:"passwordHash,omitempty"`
}

// Load reads config.json from dir. If it doesn't exist yet, it is created
// with a freshly generated random password; the plaintext password is
// returned once via generatedPassword so the caller can display it (it is
// never stored or shown again — only its bcrypt hash is persisted).
func Load(dir string) (cfg *ServerConfig, generatedPassword string, err error) {
	path := filepath.Join(dir, "config.json")

	data, err := os.ReadFile(path)
	if err == nil {
		cfg = &ServerConfig{}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, "", fmt.Errorf("parse %s: %w", path, err)
		}
		if cfg.Listen == "" {
			cfg.Listen = DefaultListen
		}
		return cfg, "", nil
	}
	if !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", fmt.Errorf("create %s: %w", dir, err)
	}

	password, err := randomPassword()
	if err != nil {
		return nil, "", fmt.Errorf("generate password: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("hash password: %w", err)
	}

	cfg = &ServerConfig{Listen: DefaultListen, PasswordHash: string(hash)}
	if err := save(path, cfg); err != nil {
		return nil, "", err
	}
	return cfg, password, nil
}

// SetPassword replaces the login password stored in dir's config.json. Other
// settings are kept; if config.json doesn't exist yet it is created with
// defaults.
func SetPassword(dir, password string) error {
	if password == "" {
		return fmt.Errorf("password must not be empty")
	}
	path := filepath.Join(dir, "config.json")

	cfg := &ServerConfig{Listen: DefaultListen}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, cfg); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	default:
		return fmt.Errorf("read %s: %w", path, err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	cfg.PasswordHash = string(hash)
	return save(path, cfg)
}

func save(path string, cfg *ServerConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func randomPassword() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
