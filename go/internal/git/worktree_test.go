package git

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestListLinkedWorktrees(t *testing.T) {
	repo := newRepo(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(root, "1 space"), filepath.Join(root, "2 space")}
	if runtime.GOOS != "windows" {
		paths[1] = filepath.Join(root, "2\tquote\"\n")
	}
	for _, path := range paths {
		if err := AddWorktree(repo, path, "main"); err != nil {
			t.Fatal(err)
		}
	}
	run(t, repo, "worktree", "lock", "--reason", "keep\nthis", paths[1])
	if err := os.RemoveAll(paths[1]); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{repo, paths[0]} {
		got, err := ListLinkedWorktrees(cwd)
		if err != nil {
			t.Fatal(err)
		}
		var gotPaths []string
		for _, wt := range got {
			gotPaths = append(gotPaths, wt.Path)
		}
		if !reflect.DeepEqual(gotPaths, paths) {
			t.Fatalf("want linked paths %q, got %q", paths, gotPaths)
		}
		if got[0].Head == "" || got[0].Locked || !got[1].Locked {
			t.Fatalf("incorrect registration metadata: %+v", got)
		}
	}
}
