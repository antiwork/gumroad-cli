package adminconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/config"
)

type Actor struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type Config struct {
	Token           string `json:"token,omitempty"`
	TokenExternalID string `json:"token_external_id,omitempty"`
	Actor           Actor  `json:"actor,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
}

const (
	EnvAccessToken    = "GUMROAD_ADMIN_TOKEN"                                                                                                                              //nolint:gosec // G101: env var name, not a credential.
	HintSetAdminToken = "Run `gumroad auth login` and check the admin box. For CI/agents, set GUMROAD_ADMIN_TOKEN and pass --non-interactive for mutating admin commands." //nolint:gosec // G101: remediation text, not a credential.

	adminConfigFile       = "admin.token"
	adminConfigTempPrefix = "admin.token.tmp-*"
)

var (
	ErrNotAuthenticated = errors.New("not authenticated")
	goos                = runtime.GOOS
)

type TokenSource string

const (
	TokenSourceEnv    TokenSource = "env"
	TokenSourceConfig TokenSource = "config"
)

type TokenInfo struct {
	Value           string
	Source          TokenSource
	TokenExternalID string
	Actor           Actor
	ExpiresAt       string
}

func Dir() (string, error) {
	return config.Dir()
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, adminConfigFile), nil
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	cfg, _, err := loadConfigFile(p)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func loadConfigFile(p string) (*Config, bool, error) {
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			if goos == "windows" {
				if recoverErr := recoverBackup(p); recoverErr == nil {
					info, err = os.Stat(p)
				}
			}
			if err != nil {
				return &Config{}, false, nil
			}
		} else {
			return nil, false, fmt.Errorf("could not read admin config: %w", err)
		}
	}
	if err := validateConfigPermissions(p, info.Mode()); err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false, fmt.Errorf("could not read admin config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false, fmt.Errorf("could not parse admin config: %w", err)
	}
	return &cfg, true, nil
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
		return fmt.Errorf("could not marshal admin config: %w", err)
	}
	if err := writeConfigAtomically(p, data); err != nil {
		return fmt.Errorf("could not write admin config: %w", err)
	}
	return nil
}

func Delete() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not delete admin config: %w", err)
	}
	if err := os.Remove(p + ".bak"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not delete admin config backup: %w", err)
	}
	return nil
}

func ResolveToken() (TokenInfo, error) {
	if token := strings.TrimSpace(os.Getenv(EnvAccessToken)); token != "" {
		return TokenInfo{Value: token, Source: TokenSourceEnv}, nil
	}
	return ResolveStoredToken()
}

func HasEnvToken() bool {
	return strings.TrimSpace(os.Getenv(EnvAccessToken)) != ""
}

func ResolveStoredToken() (TokenInfo, error) {
	cfg, err := Load()
	if err != nil {
		return TokenInfo{}, err
	}
	if cfg.tokenValue() == "" {
		return TokenInfo{}, notLoggedInError()
	}
	return cfg.tokenInfo(), nil
}

func Token() (string, error) {
	info, err := ResolveToken()
	if err != nil {
		return "", err
	}
	return info.Value, nil
}

func (cfg *Config) tokenValue() string {
	if cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.Token) != "" {
		return strings.TrimSpace(cfg.Token)
	}
	return ""
}

func (cfg *Config) tokenInfo() TokenInfo {
	return TokenInfo{
		Value:           cfg.tokenValue(),
		Source:          TokenSourceConfig,
		TokenExternalID: cfg.TokenExternalID,
		Actor:           cfg.Actor,
		ExpiresAt:       cfg.ExpiresAt,
	}
}

func notLoggedInError() error {
	return fmt.Errorf("%w. not logged in for admin; run 'gumroad auth login' and check the admin box", ErrNotAuthenticated)
}

func writeConfigAtomically(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), adminConfigTempPrefix)
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
	backupPath := path + ".bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func recoverBackup(path string) error {
	return os.Rename(path+".bak", path)
}

func validateConfigPermissions(path string, mode os.FileMode) error {
	if goos == "windows" {
		return nil
	}

	perm := mode.Perm()
	if perm&0077 != 0 {
		return fmt.Errorf("could not read admin config: config file %s has insecure permissions %04o; run chmod 600 %s", path, perm, path)
	}
	return nil
}
