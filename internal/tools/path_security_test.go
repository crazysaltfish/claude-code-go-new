package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalizePathResolvesExistingSymlinkParent(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(workspace, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	target, err := CanonicalizePath(filepath.Join("linked", "new.txt"), workspace)
	if err != nil {
		t.Fatal(err)
	}
	canonicalOutside, err := CanonicalizePath(outside, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalOutside, "new.txt")
	if target != want {
		t.Fatalf("canonical target = %q, want %q", target, want)
	}
	if IsPathWithin(workspace, target) {
		t.Fatalf("symlink escape %q was considered inside %q", target, workspace)
	}
}

func TestIsPathWithinRejectsSiblingPrefix(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	sibling := filepath.Join(parent, "project-secret")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatal(err)
	}
	if IsPathWithin(root, filepath.Join(sibling, "key")) {
		t.Fatal("sibling sharing the root prefix was considered inside")
	}
	if !IsPathWithin(root, filepath.Join(root, "subdir", "file")) {
		t.Fatal("normal child path was considered outside")
	}
}
