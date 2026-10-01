package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNotFound is the error of reading a file that does not exist.
var ErrNotFound = errors.New("file not found")

// ReadFile is a file's contents, or ErrNotFound when there is no such file.
func ReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNotFound
	}
	return string(b), err
}

// WriteFile replaces a file's contents, creating the file and its directories
// as needed. It returns what the file held before, and whether it existed.
func WriteFile(path, content string) (old string, existed bool, err error) {
	if prev, rerr := ReadFile(path); rerr == nil {
		old, existed = prev, true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, err
	}
	return old, existed, os.WriteFile(path, []byte(content), 0o644)
}
