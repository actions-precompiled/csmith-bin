package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCondaCreatePkgs(t *testing.T) {
	t.Parallel()
	pkgs := condaCreatePkgs()
	joined := strings.Join(pkgs, " ")
	if !strings.Contains(joined, "cmake=") || !strings.Contains(joined, "ninja=") {
		t.Fatalf("missing cmake/ninja: %v", pkgs)
	}
	if runtime.GOOS == "windows" {
		if !containsStr(pkgs, "m2w64-toolchain") || !containsStr(pkgs, "m2-m4") {
			t.Fatalf("windows pkgs: %v", pkgs)
		}
		if containsStr(pkgs, "clang") {
			t.Fatalf("windows must not use conda clang (needs MSVC): %v", pkgs)
		}
	} else {
		if !containsStr(pkgs, "clang") || !containsStr(pkgs, "clangxx") {
			t.Fatalf("unix pkgs: %v", pkgs)
		}
	}
}

func TestCondaCompilers(t *testing.T) {
	t.Parallel()
	cc, cxx := condaCompilers()
	if runtime.GOOS == "windows" {
		if cc != "gcc" || cxx != "g++" {
			t.Fatalf("got %s/%s", cc, cxx)
		}
		return
	}
	if cc != "clang" || cxx != "clang++" {
		t.Fatalf("got %s/%s", cc, cxx)
	}
}

func TestPrependPATH(t *testing.T) {
	t.Parallel()
	env := []string{"FOO=1", "PATH=/usr/bin"}
	got := prependPATH(env, []string{"/conda/bin", "/conda/lib"})
	var path string
	for _, e := range got {
		if strings.HasPrefix(e, "PATH=") {
			path = e
		}
	}
	if !strings.HasPrefix(path, "PATH=/conda/bin") {
		t.Fatalf("PATH not prepended: %q", path)
	}
	if !strings.Contains(path, "/usr/bin") {
		t.Fatalf("lost old PATH: %q", path)
	}
}

func TestIsSystemDylib(t *testing.T) {
	t.Parallel()
	if !isSystemDylib("/usr/lib/libc++.1.dylib") {
		t.Fatal("usr")
	}
	if !isSystemDylib("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation") {
		t.Fatal("system")
	}
	if isSystemDylib("/opt/conda/lib/libc++.1.dylib") {
		t.Fatal("conda lib must be vendored")
	}
}

func TestResolveCondaExe(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "m4"
	if runtime.GOOS == "windows" {
		name = "m4.exe"
	}
	path := filepath.Join(bin, name)
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveCondaExe(root, "m4")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("got %q want %q", got, path)
	}
	_, err = resolveCondaExe(root, "definitely-missing-tool")
	if !errors.Is(err, ErrHostToolMissing) {
		t.Fatalf("got %v", err)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want || strings.HasPrefix(s, want+"=") {
			return true
		}
	}
	return false
}
