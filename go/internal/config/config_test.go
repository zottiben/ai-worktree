package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxTrees != 16 {
		t.Errorf("default MaxTrees = %d, want 16", cfg.MaxTrees)
	}
}

func TestResolvePoolRootDefault(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	root, err := ResolvePoolRoot("/some/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".awt")
	if root != want {
		t.Errorf("ResolvePoolRoot(empty) = %q, want %q", root, want)
	}
}

func TestResolvePoolRootRelativeNeedsRepo(t *testing.T) {
	if _, err := ResolvePoolRoot("", "./work"); err == nil {
		t.Error("relative root without repo should error")
	}
	root, err := ResolvePoolRoot("/repo", "./work")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/repo", "work", ".awt")
	if root != want {
		t.Errorf("ResolvePoolRoot(relative) = %q, want %q", root, want)
	}
}

func TestResolvePoolRootAbsolute(t *testing.T) {
	root, err := ResolvePoolRoot("", "/abs/trees")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/abs/trees", ".awt")
	if root != want {
		t.Errorf("ResolvePoolRoot(absolute) = %q, want %q", root, want)
	}
}

// TestLoadIgnoresRepoHooks verifies the safety rule: hooks declared in the
// repo-level awt.toml are dropped, so a cloned repo cannot ship executable
// lifecycle commands. Only user-level hooks are honored.
func TestLoadIgnoresRepoHooks(t *testing.T) {
	repo := t.TempDir()
	repoToml := "max_trees = 8\n[hooks]\npost_create = [\"echo repo-hook\"]\n"
	if err := os.WriteFile(filepath.Join(repo, "awt.toml"), []byte(repoToml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Isolate HOME so a real user config never leaks into the test.
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxTrees != 8 {
		t.Errorf("MaxTrees = %d, want 8 (from repo config)", cfg.MaxTrees)
	}
	if len(cfg.Hooks.PostCreate) != 0 {
		t.Errorf("repo-level hooks should be ignored, got %v", cfg.Hooks.PostCreate)
	}
}

// TestLoadUserHooksApplied verifies user-level hooks ARE honored and merged
// over a repo-level config.
func TestLoadUserHooksApplied(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "awt.toml"), []byte("max_trees = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	userDir := filepath.Join(home, ".config", "awt")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userToml := "[hooks]\npost_create = [\"echo user-hook\"]\n"
	if err := os.WriteFile(filepath.Join(userDir, "config.toml"), []byte(userToml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxTrees != 5 {
		t.Errorf("MaxTrees = %d, want 5 (repo wins for repo-safe settings)", cfg.MaxTrees)
	}
	if len(cfg.Hooks.PostCreate) != 1 || cfg.Hooks.PostCreate[0] != "echo user-hook" {
		t.Errorf("user hooks = %v, want [echo user-hook]", cfg.Hooks.PostCreate)
	}
}
