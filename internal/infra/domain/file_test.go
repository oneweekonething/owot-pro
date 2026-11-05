package domain

import (
	"errors"
	"testing"
	"time"
)

func TestFilePathsRejectTraversal(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"../escape.txt", `..\escape.txt`, "foo/bar.txt", "%2e%2e.txt", "bad\n.txt"} {
		if _, err := NewFilePath(now, "", name); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("name %q returned %v", name, err)
		}
	}
	for _, directory := range []string{"../escape", `sub\escape`, "foo//bar", StagingDirectory} {
		if _, err := NewFilePath(now, directory, "example.txt"); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("directory %q returned %v", directory, err)
		}
	}
	path, err := NewFilePath(now, "images/avatars", "头像.png")
	if err != nil {
		t.Fatal(err)
	}
	if clean, err := SanitizeFilePath(path); err != nil || clean != path {
		t.Fatalf("generated path rejected: %q, %v", clean, err)
	}
}
