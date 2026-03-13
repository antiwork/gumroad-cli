package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	AccessToken string `json:"access_token"`
}

var userHomeDir = os.UserHomeDir
var goos = runtime.GOOS

var ErrNotAuthenticated = errors.New("not authenticated")

const EnvAccessToken = "GUMROAD_ACCESS_TOKEN"

type TokenSource string

const (
	TokenSourceEnv    TokenSource = "env"
	TokenSourceConfig TokenSource = "config"
)

type TokenInfo struct {
	Value  string
	Source TokenSource
}

func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "gumroad"), nil
	}
	if goos == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "gumroad"), nil
		}
	}
	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gumroad"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("could not read config: %w", err)
	}
	if err := validateConfigPermissions(p, info.Mode()); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("could not read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal config: %w", err)
	}
	if err := writeConfigAtomically(p, data); err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}
	return nil
}

func writeConfigAtomically(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "config.json.tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if err = tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return replaceFile(tmpPath, path)
}

func replaceFile(tmpPath, path string) error {
	if goos != "windows" {
		return os.Rename(tmpPath, path)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, path)
}

func validateConfigPermissions(path string, mode os.FileMode) error {
	if goos == "windows" {
		return nil
	}

	perm := mode.Perm()
	if perm&0077 != 0 {
		return fmt.Errorf("could not read config: config file %s has insecure permissions %04o; run chmod 600 %s", path, perm, path)
	}
	return nil
}

func Delete() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not delete config: %w", err)
	}
	return nil
}

func ResolveToken() (TokenInfo, error) {
	if token := strings.TrimSpace(os.Getenv(EnvAccessToken)); token != "" {
		return TokenInfo{Value: token, Source: TokenSourceEnv}, nil
	}

	cfg, err := Load()
	if err != nil {
		return TokenInfo{}, err
	}
	if cfg.AccessToken == "" {
		return TokenInfo{}, fmt.Errorf("%w. Run `gumroad auth login` first or set `%s`", ErrNotAuthenticated, EnvAccessToken)
	}
	return TokenInfo{Value: cfg.AccessToken, Source: TokenSourceConfig}, nil
}

func Token() (string, error) {
	info, err := ResolveToken()
	if err != nil {
		return "", err
	}
	return info.Value, nil
}
