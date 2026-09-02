package sysops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDirectory_pathTraversal(t *testing.T) {
	dir := t.TempDir()
	uploadDir := filepath.Join(dir, "upload")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "ok.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ListDirectory(uploadDir, "../../etc")
	if err == nil {
		t.Fatal("expected error for traversal attempt")
	}

	entries, err := ListDirectory(uploadDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "ok.txt" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}
