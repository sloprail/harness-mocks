package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// untar extracts a .tar.gz into dir, dropping each name's first path element
// (tar --strip-components=1) and refusing any entry that would land outside dir.
func untar(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if filepath.IsAbs(h.Name) || slices.Contains(strings.Split(filepath.ToSlash(h.Name), "/"), "..") {
			return fmt.Errorf("tar entry %q escapes the install directory", h.Name)
		}
		_, rel, ok := strings.Cut(filepath.ToSlash(filepath.Clean(h.Name)), "/")
		if !ok {
			continue
		}
		dest := filepath.Join(dir, rel)
		if dest != dir && !strings.HasPrefix(dest, dir+string(filepath.Separator)) {
			return fmt.Errorf("tar entry %q escapes the install directory", h.Name)
		}
		if err := extract(tr, h, dest); err != nil {
			return err
		}
	}
}

func extract(tr *tar.Reader, h *tar.Header, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	switch h.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(dest, 0o755)
	case tar.TypeSymlink:
		return os.Symlink(h.Linkname, dest)
	case tar.TypeReg:
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, h.FileInfo().Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	return nil
}
