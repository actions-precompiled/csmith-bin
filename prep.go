package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/actions-precompiled/foundation"
)

// PrepHost implements foundation.HostPrep.
func (csmithPackage) PrepHost(ctx context.Context, deps foundation.Deps, cfg foundation.Config) error {
	startPreclones(ctx, deps, csmithPackage{}.Meta(), cfg.Versions)

	if foundation.TargetsNeedDocker(cfg.Targets) {
		if _, err := deps.Runner.Output(ctx, "docker", "version"); err != nil {
			return fmt.Errorf("docker required for linux targets: %w", err)
		}
		deps.Logf("PrepHost: docker OK")
	}
	if !hasNativeTarget(cfg.Targets) {
		return nil
	}
	return prepNativeHost(ctx, deps)
}

func hasNativeTarget(targets []string) bool {
	for _, t := range targets {
		if foundation.IsNativeTarget(t) {
			return true
		}
	}
	return false
}

func prepNativeHost(ctx context.Context, deps foundation.Deps) error {
	if _, err := deps.Runner.Output(ctx, "micromamba", "--version"); err != nil {
		return fmt.Errorf("%w: micromamba (install via mise): %w", ErrHostToolMissing, err)
	}
	if err := ensureCondaEnv(ctx, deps); err != nil {
		return err
	}
	cc, cxx := condaCompilers()
	for _, tool := range []struct {
		name string
		args []string
	}{
		{"cmake", []string{"--version"}},
		{"ninja", []string{"--version"}},
		{"m4", []string{"--version"}},
		{cc, []string{"--version"}},
		{cxx, []string{"--version"}},
	} {
		out, err := outputInConda(ctx, deps, tool.name, tool.args...)
		if err != nil {
			return fmt.Errorf("%w: %s in conda env %s: %w", ErrHostToolMissing, tool.name, condaPrefix(deps), err)
		}
		deps.Logf("PrepHost: %s %s", tool.name, firstLine(out))
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
