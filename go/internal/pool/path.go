package pool

import (
	"os"
	"path/filepath"
)

func samePath(a, b string) (bool, error) {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true, nil
	}
	resolvedA, err := resolvePath(a)
	if err != nil {
		return false, err
	}
	resolvedB, err := resolvePath(b)
	if err != nil {
		return false, err
	}
	if resolvedA == resolvedB {
		return true, nil
	}
	infoA, err := os.Stat(resolvedA)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	infoB, err := os.Stat(resolvedB)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return os.SameFile(infoA, infoB), nil
}

func resolvePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) || filepath.Dir(absolute) == absolute {
		return "", err
	}
	parent, err := resolvePath(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}
