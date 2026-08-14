package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWindowsM4(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	exe := filepath.Join(root, "conda-m2-m4", "1.4.19.2", "Library", "usr", "bin", "m4.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := findWindowsM4([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if got != exe {
		t.Fatalf("got %q want %q", got, exe)
	}
	if _, err := findWindowsM4([]string{t.TempDir()}); err == nil {
		t.Fatal("expected miss")
	}
}
