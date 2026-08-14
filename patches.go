package main

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"

	"github.com/actions-precompiled/foundation"
)

// Same backports as conda-forge/staged-recipes#34531 (skip their 0002:
// they drop the static lib for CFEP-18; we keep static and skip shared
// on Windows so Ninja does not emit two csmith.lib files).
//
//go:embed patches/*.patch
var patchFS embed.FS

var upstreamPatches = []string{
	"0001-fix-FilterKind-enum-range.patch",
	"0003-fix-msvc-x64-inline-asm.patch",
	"0004-avoid-deprecated-bind2nd-ptr_fun.patch",
	"0005-install-csmith-as-cmake-target.patch",
}

const patchMarker = ".apc-upstream-patches"

func applyUpstreamPatches(ctx context.Context, deps foundation.Deps, src string) error {
	marker := filepath.Join(src, patchMarker)
	if _, err := deps.FS.Stat(marker); err == nil {
		deps.Logf("patches: already applied in %s", src)
		return nil
	}
	tmp, err := deps.FS.TempDir("", "apc-patches-")
	if err != nil {
		return err
	}
	defer deps.RemoveAllLog(tmp, "patch staging")

	for _, name := range upstreamPatches {
		data, err := patchFS.ReadFile("patches/" + name)
		if err != nil {
			return fmt.Errorf("embed %s: %w", name, err)
		}
		p := filepath.Join(tmp, name)
		if err := deps.FS.WriteFile(p, data, 0o644); err != nil {
			return err
		}
		deps.Logf("patches: apply %s", name)
		if err := deps.Runner.Run(ctx, "git", "-C", src, "apply", "--whitespace=nowarn", p); err != nil {
			return fmt.Errorf("git apply %s: %w", name, err)
		}
	}
	return deps.FS.WriteFile(marker, []byte("conda-forge/staged-recipes#34531\n"), 0o644)
}
