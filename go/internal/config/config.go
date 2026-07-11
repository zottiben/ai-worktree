package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/zottiben/ai-worktree/go/internal/git"
)

// Config is the resolved awt configuration.
type Config struct {
	MaxTrees int    `toml:"max_trees"`
	Root     string `toml:"root"`
	Hooks    Hooks  `toml:"hooks,omitempty"`
}

// Hooks are the user-configured lifecycle commands. They are honored only from
// the user-level config file, never from repo-level config, for safety.
type Hooks struct {
	PostCreate []string `toml:"post_create,omitempty"`
	PreDestroy []string `toml:"pre_destroy,omitempty"`
}

// DefaultConfig returns the built-in defaults used when no config file exists.
func DefaultConfig() Config {
	return Config{
		MaxTrees: 16,
	}
}

// Load resolves configuration for a repository. Repo-level awt.toml takes
// precedence for repo-safe settings, but hooks are always taken only from the
// user-level config so a cloned repo cannot ship executable lifecycle commands.
func Load(repoRoot string) (Config, error) {
	cfg := DefaultConfig()

	repoPath := filepath.Join(repoRoot, "awt.toml")
	hasRepoConfig := false
	if _, err := os.Stat(repoPath); err == nil {
		hasRepoConfig = true
		if _, err := toml.DecodeFile(repoPath, &cfg); err != nil {
			return cfg, err
		}
		cfg.Hooks = Hooks{}
	}

	userCfg, hasUserConfig, err := loadUser()
	if err != nil {
		return cfg, err
	}
	if hasUserConfig {
		if !hasRepoConfig {
			cfg = userCfg
		} else {
			cfg.Hooks = userCfg.Hooks
		}
	}

	return cfg, nil
}

// LoadGlobal returns the default configuration merged with user-level config.
// It intentionally ignores repo-level config because callers may run without a
// repository context (e.g. prune --all, destroy <pool>).
func LoadGlobal() (Config, error) {
	cfg := DefaultConfig()
	userCfg, hasUserConfig, err := loadUser()
	if err != nil {
		return cfg, err
	}
	if hasUserConfig {
		cfg = userCfg
	}
	return cfg, nil
}

func loadUser() (Config, bool, error) {
	cfg := DefaultConfig()
	if home, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(home, ".config", "awt", "config.toml")
		if _, err := os.Stat(userPath); err == nil {
			if _, err := toml.DecodeFile(userPath, &cfg); err != nil {
				return cfg, false, err
			}
			return cfg, true, nil
		}
	}

	return cfg, false, nil
}

// ResolvePoolDir returns the per-repository pool directory. The pool name is the
// repository's base name plus a short hash of its remote URL (or absolute path
// for purely-local repositories), so different clones of the same repo share a
// pool while unrelated repos never collide.
func ResolvePoolDir(repoRoot string, root string) (string, error) {
	// Use remote URL for the hash when available; fall back to the
	// absolute repo path for purely-local repositories.
	hashInput, err := git.GetRemoteURL(repoRoot)
	if err != nil {
		hashInput = repoRoot
	}

	repoName := filepath.Base(repoRoot)
	shortHash := git.ShortHash(hashInput)
	poolName := repoName + "-" + shortHash

	poolRoot, err := ResolvePoolRoot(repoRoot, root)
	if err != nil {
		return "", err
	}
	return filepath.Join(poolRoot, poolName), nil
}

// ResolvePoolRoot resolves the directory that contains per-repository pools.
// Relative roots require repoRoot because they are resolved from the repository
// root. An empty root defaults to $HOME/.awt.
func ResolvePoolRoot(repoRoot string, root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".awt"), nil
	}

	expanded := os.ExpandEnv(root)
	if !filepath.IsAbs(expanded) {
		if repoRoot == "" {
			return "", fmt.Errorf("relative awt root %q requires a repository", root)
		}
		expanded = filepath.Join(repoRoot, expanded)
	}
	return filepath.Join(expanded, ".awt"), nil
}
