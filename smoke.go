package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/actions-precompiled/foundation"
)

func smokeLinux(ctx context.Context, deps foundation.Deps, meta foundation.Meta, req foundation.SmokeRequest) error {
	if len(req.Tarballs) == 0 {
		return fmt.Errorf("%w", ErrSmokeNoTarballs)
	}
	for _, tb := range req.Tarballs {
		if err := smokeLinuxTarball(ctx, deps, meta, tb); err != nil {
			return err
		}
	}
	return nil
}

func smokeLinuxTarball(ctx context.Context, deps foundation.Deps, meta foundation.Meta, tarball string) error {
	deps.Logf("Smoke test (csmith): %s", filepath.Base(tarball))
	tmp, err := deps.FS.TempDir("", "csmith-smoke-")
	if err != nil {
		return err
	}
	defer deps.RemoveAllLog(tmp, "smoke cleanup")

	if err := deps.Runner.Run(ctx, "tar", "-xzf", tarball, "-C", tmp); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	root := filepath.Join(tmp, meta.Name)
	csmith := filepath.Join(root, "bin", "csmith")
	if _, err := deps.FS.Stat(csmith); err != nil {
		return fmt.Errorf("%w: bin/csmith", ErrCsmithMissing)
	}

	if err := foundation.CheckLinuxRelocatable(root, foundation.RelocatableOpts{
		RequiredBins: []string{"csmith"},
	}); err != nil {
		return err
	}
	deps.Logf("relocatable: RPATH/$ORIGIN OK")

	env := foundation.CleanSmokeEnv(deps.Env.Environ())

	if err := foundation.SmokeBinDirHelp(ctx, deps, root, foundation.BinHelpOpts{Env: env}); err != nil {
		return err
	}

	// Generate a program with a fixed seed for reproducibility.
	gen := filepath.Join(tmp, "random.c")
	out, err := foundation.OutputWithEnv(ctx, deps, env, csmith, "--seed", "1")
	if err != nil {
		// Older csmith may use -s
		out, err = foundation.OutputWithEnv(ctx, deps, env, csmith, "-s", "1")
		if err != nil {
			return fmt.Errorf("csmith generate: %w\n%s", err, out)
		}
	}
	if len(strings.TrimSpace(out)) < 32 {
		return fmt.Errorf("%w (%d bytes)", ErrGenerateEmpty, len(out))
	}
	if err := deps.FS.WriteFile(gen, []byte(out), 0o644); err != nil {
		return err
	}
	deps.Logf("generated %d bytes of C (seed=1)", len(out))

	// Compile generated C with host gcc using packaged headers.
	inc, err := findCsmithInclude(deps, root)
	if err != nil {
		return err
	}
	obj := filepath.Join(tmp, "random")
	if out, err := foundation.OutputWithEnv(ctx, deps, env, "gcc", "-O0", "-I"+inc, "-o", obj, gen); err != nil {
		return fmt.Errorf("%w: %w\n%s", ErrCompileGenerated, err, out)
	}
	deps.Logf("host gcc compiled generated C with -I%s", inc)

	deps.Logf("✓ Smoke test passed: %s", filepath.Base(tarball))
	return nil
}

func findCsmithInclude(deps foundation.Deps, root string) (string, error) {
	// Preferred: include/csmith-<ver>/ or include/
	candidates := []string{
		filepath.Join(root, "include"),
	}
	entries, err := deps.FS.ReadDir(filepath.Join(root, "include"))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "csmith") {
				candidates = append([]string{filepath.Join(root, "include", e.Name())}, candidates...)
			}
		}
	}
	for _, c := range candidates {
		hdr := filepath.Join(c, "csmith.h")
		if _, err := deps.FS.Stat(hdr); err == nil {
			return c, nil
		}
	}
	// Nested layout: include/csmith-X.Y.Z/csmith.h already covered; try walk one level.
	return "", fmt.Errorf("csmith.h not found under %s/include", root)
}
