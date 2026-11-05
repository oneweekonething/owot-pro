package security

import (
	"strings"
	"testing"
)

func TestSpringBCryptCompatibility(t *testing.T) {
	h := NewPasswordHasher()
	hash, err := h.Hash("admin123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$2a$10$") {
		t.Fatal("unexpected Spring password format")
	}
	if err := h.Compare(hash, "admin123"); err != nil {
		t.Fatal(err)
	}
	if h.Compare(hash, "wrong") == nil {
		t.Fatal("wrong password accepted")
	}
}
