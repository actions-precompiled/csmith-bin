package main

import (
	"strings"
	"testing"
)

func TestWrapSharedRuntimeCMake(t *testing.T) {
	t.Parallel()
	in := `
# Build and install the static library.
add_library(libcsmith_a STATIC volatile_runtime.c)
set_target_properties(libcsmith_a PROPERTIES OUTPUT_NAME "csmith")

# Build and install the shared library.
add_library(libcsmith_so SHARED volatile_runtime.c)
set_target_properties(libcsmith_so PROPERTIES OUTPUT_NAME "csmith")
install(TARGETS libcsmith_so
  LIBRARY DESTINATION "${LIB_DIR}"
  RUNTIME DESTINATION "${LIB_DIR}"
  )

# headers
`
	out, ok := wrapSharedRuntimeCMake(in)
	if !ok {
		t.Fatal("expected wrap")
	}
	if !strings.Contains(out, "if(NOT WIN32) # APC_SKIP_SHARED_RUNTIME") {
		t.Fatalf("missing if:\n%s", out)
	}
	if !strings.Contains(out, "endif() # APC_SKIP_SHARED_RUNTIME") {
		t.Fatalf("missing endif:\n%s", out)
	}
	if !strings.Contains(out, "add_library(libcsmith_a") {
		t.Fatal("static lib dropped")
	}
	again, ok := wrapSharedRuntimeCMake(out)
	if !ok || again != out {
		t.Fatal("not idempotent")
	}

	crlf := strings.ReplaceAll(in, "\n", "\r\n")
	out2, ok := wrapSharedRuntimeCMake(crlf)
	if !ok || !strings.Contains(out2, "if(NOT WIN32) # APC_SKIP_SHARED_RUNTIME") {
		t.Fatalf("CRLF wrap failed:\n%s", out2)
	}
}

func TestWrapSharedRuntimeCMakeMissing(t *testing.T) {
	t.Parallel()
	if _, ok := wrapSharedRuntimeCMake("add_library(foo STATIC x.c)\n"); ok {
		t.Fatal("expected miss")
	}
}
