package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNotFound is the error of reading a file that does not exist.
var ErrNotFound = errors.New("file not found")

// ErrIsDir is the error of reading a directory: a read reads files.
var ErrIsDir = errors.New("is a directory")

// ReadFile is a file's contents, ErrNotFound when there is no such file, or
// ErrIsDir when the path is a directory.
func ReadFile(path string) (string, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return "", ErrIsDir
	}
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
