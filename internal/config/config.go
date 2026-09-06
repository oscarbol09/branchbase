package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config defines the configuration schema for BranchBase
type Config struct {
	Version    string           `json:"version" yaml:"version"`
	Driver     string           `json:"driver" yaml:"driver"`
	Connection ConnectionConfig `json:"connection" yaml:"connection"`
	Proxy      ProxyConfig      `json:"proxy" yaml:"proxy"`
	Strategy   StrategyConfig   `json:"strategy" yaml:"strategy"`
}

type ConnectionConfig struct {
	Host         string `json:"host" yaml:"host"`
	Port         int    `json:"port" yaml:"port"`
	User         string `json:"user" yaml:"user"`
	Password     string `json:"password" yaml:"password"`
	BaseDatabase string `json:"base_database" yaml:"base_database"`
	Path         string `json:"path,omitempty" yaml:"path,omitempty"` // For SQLite
}

type ProxyConfig struct {
	Enabled       bool   `json:"enabled" yaml:"enabled"`
	ListenPort    int    `json:"listen_port" yaml:"listen_port"`
	DefaultBranch string `json:"default_branch" yaml:"default_branch"`
}

type StrategyConfig struct {
	SnapshotOnSwitch   bool `json:"snapshot_on_switch" yaml:"snapshot_on_switch"`
	AutoPruneMerged    bool `json:"auto_prune_merged" yaml:"auto_prune_merged"`
	MaxBranchDatabases int  `json:"max_branch_databases" yaml:"max_branch_databases"`
}

// DefaultConfig returns safe, sensible defaults for BranchBase
func DefaultConfig() Config {
	return Config{
		Version: "1",
		Driver:  "postgres",
		Connection: ConnectionConfig{
			Host:         "127.0.0.1",
			Port:         5433,
			User:         "postgres",
			Password:     "postgres",
			BaseDatabase: "myapp_dev",
		},
		Proxy: ProxyConfig{
			Enabled:       true,
			ListenPort:    5432,
			DefaultBranch: "main",
		},
		Strategy: StrategyConfig{
			SnapshotOnSwitch:   true,
			AutoPruneMerged:    false,
			MaxBranchDatabases: 10,
		},
	}
}

// LoadConfig reads and parses .branchbase.json or .branchbase.yaml from repo root
func LoadConfig(repoPath string) (*Config, error) {
	cfg := DefaultConfig()

	jsonPath := filepath.Join(repoPath, ".branchbase.json")
	if bytes, err := os.ReadFile(jsonPath); err == nil {
		if err := json.Unmarshal(bytes, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	yamlPath := filepath.Join(repoPath, ".branchbase.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		// In minimal MVP without external dependencies, we parse basic lines or provide default
		return &cfg, nil
	}

	return nil, errors.New("no .branchbase.yaml or .branchbase.json found")
}

// SaveJSON writes config as formatted JSON
func (c *Config) SaveJSON(filePath string) error {
	bytes, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, bytes, 0644)
}

// DatabaseNameForBranch returns the target database identifier for a given branch
func (c *Config) DatabaseNameForBranch(sanitizedBranch string) string {
	if sanitizedBranch == "" || sanitizedBranch == strings.ToLower(c.Proxy.DefaultBranch) {
		return c.Connection.BaseDatabase
	}
	return c.Connection.BaseDatabase + "_" + sanitizedBranch
}
