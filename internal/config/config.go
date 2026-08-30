package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Context struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

type Config struct {
	CurrentContext string    `json:"current_context"`
	Contexts       []Context `json:"contexts"`
}

var ErrNoContext = errors.New("no active context configured")

func getConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".runstack", "config"), nil
}

func LoadConfig() (*Config, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config (corrupt?): %w", err)
	}

	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0600)
}

func (c *Config) GetActiveContext() (*Context, error) {
	if c.CurrentContext == "" {
		return nil, ErrNoContext
	}
	for _, ctx := range c.Contexts {
		if ctx.Name == c.CurrentContext {
			return &ctx, nil
		}
	}
	return nil, ErrNoContext
}
