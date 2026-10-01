package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/branchbase/branchbase/internal/git"
	"gopkg.in/yaml.v3"
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
	Path         string `json:"path,omitempty" yaml:"path,omitempty"`               // For SQLite
	SocketPath   string `json:"socket_path,omitempty" yaml:"socket_path,omitempty"` // For UNIX domain sockets
}

type TLSConfig struct {
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	CertFile string `json:"cert_file,omitempty" yaml:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty" yaml:"key_file,omitempty"`
	AutoCert bool   `json:"auto_cert" yaml:"auto_cert"`
}

type ProxyConfig struct {
	Enabled       bool      `json:"enabled" yaml:"enabled"`
	ListenHost    string    `json:"listen_host" yaml:"listen_host"`
	ListenPort    int       `json:"listen_port" yaml:"listen_port"`
	DefaultBranch string    `json:"default_branch" yaml:"default_branch"`
	SocketPath    string    `json:"socket_path,omitempty" yaml:"socket_path,omitempty"` // For UNIX domain sockets
	TLS           TLSConfig `json:"tls" yaml:"tls"`
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
			ListenHost:    "127.0.0.1",
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

// LoadConfig reads and parses a BranchBase configuration file.
// If path points directly to an existing file, it loads and parses that file (JSON or YAML).
// Otherwise, path is treated as a directory root and LoadConfig searches for
// .branchbase.json, .branchbase.yaml, or .branchbase.yml in that directory.
// It automatically expands environment variables (${ENV_VAR} or $ENV_VAR) found in the file.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	// If path points directly to a file, parse it directly
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		expanded := os.ExpandEnv(string(bytes))
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".json" {
			if err := json.Unmarshal([]byte(expanded), &cfg); err != nil {
				return nil, err
			}
			return &cfg, nil
		}
		if ext == ".yaml" || ext == ".yml" {
			if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
				return nil, err
			}
			return &cfg, nil
		}
		// If unknown extension, try JSON first, then YAML
		if err := json.Unmarshal([]byte(expanded), &cfg); err == nil {
			return &cfg, nil
		}
		if err := yaml.Unmarshal([]byte(expanded), &cfg); err == nil {
			return &cfg, nil
		}
		return nil, errors.New("unsupported or invalid config file format (expected JSON or YAML)")
	}

	// If path has a config file extension but does not exist, return an error directly
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" || ext == ".yaml" || ext == ".yml" {
		return nil, os.ErrNotExist
	}

	// 1. Check .branchbase.json in directory
	jsonPath := filepath.Join(path, ".branchbase.json")
	if bytes, err := os.ReadFile(jsonPath); err == nil {
		expanded := os.ExpandEnv(string(bytes))
		if err := json.Unmarshal([]byte(expanded), &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	// 2. Check .branchbase.yaml in directory
	yamlPath := filepath.Join(path, ".branchbase.yaml")
	if bytes, err := os.ReadFile(yamlPath); err == nil {
		expanded := os.ExpandEnv(string(bytes))
		if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	// 3. Check .branchbase.yml in directory
	ymlPath := filepath.Join(path, ".branchbase.yml")
	if bytes, err := os.ReadFile(ymlPath); err == nil {
		expanded := os.ExpandEnv(string(bytes))
		if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	return nil, errors.New("no .branchbase.yaml, .branchbase.yml, or .branchbase.json found")
}

// SaveJSON writes config as formatted JSON
func (c *Config) SaveJSON(filePath string) error {
	bytes, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, bytes, 0644)
}

// DatabaseNameForBranch returns the target database identifier for a given branch.
// sanitizedBranch is the already-sanitized Git branch (see git.SanitizeBranchName).
// DefaultBranch is compared after the same sanitization so names like "release/v1"
// match "release_v1" and resolve to the base database.
func (c *Config) DatabaseNameForBranch(sanitizedBranch string) string {
	if sanitizedBranch == "" || sanitizedBranch == git.SanitizeBranchName(c.Proxy.DefaultBranch) {
		return c.Connection.BaseDatabase
	}
	return c.Connection.BaseDatabase + "_" + sanitizedBranch
}
