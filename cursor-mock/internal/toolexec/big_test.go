package toolexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFrame(t *testing.T, content string, extra map[string]any) map[string]any {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"path": filepath.Join(dir, "f.txt")}
	for k, v := range extra {
		args[k] = v
	}
	r := Execute(context.Background(), call("readToolCall", args), dir, env)
	body, _ := r.Frame["success"].(map[string]any)
	return body
}

func TestReadTooBigForTheFrameNamesItsContentByAnID(t *testing.T) {
	small := readFrame(t, "hello\n", nil)
	if small["content"] != "hello\n" || small["contentBlobId"] != nil {
		t.Fatalf("a small file is carried whole: %v", small)
	}
	big := readFrame(t, strings.Repeat("x", BigBytes+1), nil)
	if _, has := big["content"]; has {
		t.Fatal("a file over the threshold is not carried in the frame")
	}
	if id, _ := big["contentBlobId"].(string); len(id) != 44 { // base64 of a SHA-256
		t.Fatalf("contentBlobId = %q", id)
	}
}

func TestReadWithALimitOfAFileThatExistsIsNotModeled(t *testing.T) {
	dir := t.TempDir()
	r := Execute(context.Background(), call("readToolCall", map[string]any{"path": filepath.Join(dir, "none"), "limit": 50}), dir, env)
	if !r.Failed || !strings.Contains(r.ErrorMessage, "File not found") {
		t.Fatalf("a missing file is as recorded: %+v", r)
	}
	body := readFrame(t, "a\n", map[string]any{"limit": 50})
	if body != nil {
		t.Fatalf("a limit on a file that exists is refused, not guessed: %v", body)
	}
}
