package shell

import (
	"os"
	"os/exec"
	"runtime"
)

// Spawn opens an interactive subshell rooted at dir with the given extra
// environment variables appended to the current environment. It returns the
// subshell's exit code.
func Spawn(dir string, env []string) (int, error) {
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		if runtime.GOOS == "windows" {
			shellPath = os.Getenv("COMSPEC")
			if shellPath == "" {
				shellPath = "cmd.exe"
			}
		} else {
			shellPath = "/bin/sh"
		}
	}

	cmd := exec.Command(shellPath)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}
