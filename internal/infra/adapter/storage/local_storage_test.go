package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
)

func TestLocalStorageRejectsSymlinkOutsideRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "uploads")); err != nil {
		t.Fatal(err)
	}
	engine := &localEngine{root: root, directory: "uploads"}
	path, err := domain.NewFilePath(time.Now(), "", "example.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Put(context.Background(), path, []byte("test"), "text/plain"); err == nil {
		t.Fatal("upload followed a symlink outside the storage root")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("upload wrote outside the storage root: %v %v", entries, err)
	}
}
