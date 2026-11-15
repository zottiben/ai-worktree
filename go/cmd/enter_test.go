package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnterFromLinkedWorktree(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "awt")
	build := exec.Command("go", "build", "-o", binary, "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build awt: %v\n%s", err, out)
	}
	for _, root := range []string{"", "worktrees"} {
		t.Run("root="+root, func(t *testing.T) {
			_, poolDir, target, sibling := newDestroyPool(t, root)
			command := exec.Command(binary, "enter", "--print-path", "6")
			command.Dir = target
			out, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("enter 6 from worktree 5: %v\n%s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != sibling {
				t.Fatalf("enter 6 printed %q, want %q", got, sibling)
			}
			pools, err := os.ReadDir(filepath.Dir(poolDir))
			if err != nil {
				t.Fatal(err)
			}
			if len(pools) != 1 {
				t.Fatalf("enter must not create another pool: %v", pools)
			}
		})
	}
}
