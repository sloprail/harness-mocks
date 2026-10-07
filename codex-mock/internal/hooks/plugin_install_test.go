package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

// A symbolic link in a plugin is not copied into the cache (recorded: runs/plugin-install, whose cache has no link).
func TestInstallPluginSkipsSymbolicLinks(t *testing.T) {
	src, home := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("r"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("README.md", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	dest, err := installPlugin(home, "mk", "p", src)
	if err != nil {
		t.Fatal(err)
	}
	if dest != filepath.Join(home, "plugins", "cache", "mk", "p", "local") {
		t.Errorf("installed at %s", dest)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); err == nil {
		t.Error("the link was copied")
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Error("the file was not")
	}
}
