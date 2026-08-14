package main

import (
	"strings"
	"testing"
)

func TestEmbeddedPatchesPresent(t *testing.T) {
	t.Parallel()
	for _, name := range upstreamPatches {
		data, err := patchFS.ReadFile("patches/" + name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		body := string(data)
		if !strings.Contains(body, "diff --git") && !strings.Contains(body, "--- a/") {
			t.Fatalf("%s: not a patch", name)
		}
	}
	if _, err := patchFS.ReadFile("patches/0002-skip-static-csmith-lib.patch"); err == nil {
		t.Fatal("do not vendor conda-forge 0002; we keep the static lib")
	}
}
