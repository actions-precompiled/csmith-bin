package main

import "testing"

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

func TestHostUnixCompilers(t *testing.T) {
	t.Parallel()
	cc, cxx := hostUnixCompilers()
	if cc != "clang" || cxx != "clang++" {
		t.Fatalf("got %s/%s", cc, cxx)
	}
}
