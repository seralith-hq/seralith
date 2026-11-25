// Package project locates a Seralith-enabled repository and its configuration and state.
package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/gitx"
	"gopkg.in/yaml.v3"
)

const (
	ConfigDirName = ".seralith"
	ConfigFile    = "config.yaml"
	PolicyFile    = "policy.yaml"
	TrustFile     = "trust.yaml"
	NotesRef      = "refs/notes/seralith"
)

var ErrNotInitialized = errors.New("project: seralith is not initialized here; run `seralith init`")

type AgentProfile struct {
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name,omitempty"`
	Kind       string   `yaml:"kind,omitempty"`
	Command    []string `yaml:"command"`
	ModelFlags []string `yaml:"modelFlags,omitempty"`
	ModelEnv   []string `yaml:"modelEnv,omitempty"`
}

type WorkspaceConfig struct {
	Root  string   `yaml:"root,omitempty"`
	Copy  []string `yaml:"copy,omitempty"`
	Setup []string `yaml:"setup,omitempty"`
}

type Config struct {
	Version    int             `yaml:"version"`
	Name       string          `yaml:"name"`
	Workspaces WorkspaceConfig `yaml:"workspaces,omitempty"`
	Agents     []AgentProfile  `yaml:"agents,omitempty"`
	Control    *ControlConfig  `yaml:"control,omitempty"`
}

type ControlConfig struct {
	URL     string `yaml:"url"`
	Project string `yaml:"project"`
}

// Project is a repository with Seralith configuration.
type Project struct {
	Root     string
	Repo     *gitx.Repo
	StateDir string
	Config   *Config
}

// Find resolves the project containing dir. Worktrees resolve to their main repository.
func Find(ctx context.Context, dir string) (*Project, error) {
	repo, err := gitx.Open(dir)
	if err != nil {
		return nil, err
	}
	common, err := repo.CommonDir(ctx)
	if err != nil {
		return nil, err
	}
	root := repo.Dir
	if filepath.Base(common) == ".git" {
		root = filepath.Dir(common)
	}
	main, err := gitx.Open(root)
	if err != nil {
		return nil, err
	}
	p := &Project{Root: main.Dir, Repo: main, StateDir: filepath.Join(common, "seralith")}
	cfg, err := LoadConfig(p.ConfigPath(ConfigFile))
	if err != nil {
		return nil, err
	}
	p.Config = cfg
	return p, nil
}

// Load is Find that additionally requires `seralith init` to have run.
func Load(ctx context.Context, dir string) (*Project, error) {
	p, err := Find(ctx, dir)
	if err != nil {
		return nil, err
	}
	if p.Config == nil {
		return nil, ErrNotInitialized
	}
	return p, nil
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("project: parse %s: %w", path, err)
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("project: unsupported config version %d", c.Version)
	}
	return &c, nil
}

func (p *Project) ConfigPath(name string) string {
	return filepath.Join(p.Root, ConfigDirName, name)
}

func (p *Project) LedgerPath() string     { return filepath.Join(p.StateDir, "ledger.jsonl") }
func (p *Project) WorkspaceDB() string    { return filepath.Join(p.StateDir, "workspaces.json") }
func (p *Project) SessionsDir() string    { return filepath.Join(p.StateDir, "sessions") }
func (p *Project) MachineKeyPath() string { return filepath.Join(p.StateDir, "machine.pem") }

// WorktreeRoot is where workspaces for this project are created.
func (p *Project) WorktreeRoot() string {
	if p.Config != nil && p.Config.Workspaces.Root != "" {
		root := expandHome(p.Config.Workspaces.Root)
		if !filepath.IsAbs(root) {
			root = filepath.Join(p.Root, root)
		}
		return root
	}
	sum := sha256.Sum256([]byte(p.Root))
	return filepath.Join(DataDir(), "worktrees", filepath.Base(p.Root)+"-"+hex.EncodeToString(sum[:4]))
}

// SaveConfig writes the project configuration.
func (p *Project) SaveConfig() error {
	data, err := yaml.Marshal(p.Config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(p.Root, ConfigDirName), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p.ConfigPath(ConfigFile), data, 0o644)
}

// DataDir is the per-user data directory for Seralith.
func DataDir() string {
	if v := os.Getenv("SERALITH_DATA_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "seralith")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "seralith")
}

// ConfigHome is the per-user configuration directory for Seralith.
func ConfigHome() string {
	if v := os.Getenv("SERALITH_CONFIG_DIR"); v != "" {
		return v
	}
	if v, err := os.UserConfigDir(); err == nil {
		return filepath.Join(v, "seralith")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "seralith")
}

// UserKeyPath is the default location of the developer's personal signing key.
func UserKeyPath() string {
	if v := os.Getenv("SERALITH_KEY"); v != "" {
		return v
	}
	return filepath.Join(ConfigHome(), "keys", "default.pem")
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
