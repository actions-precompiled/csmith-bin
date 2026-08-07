package main

import (
	"context"
	"fmt"
	"runtime"

	"github.com/actions-precompiled/foundation"
)

// PrepHost implements foundation.HostPrep.
func (csmithPackage) PrepHost(ctx context.Context, deps foundation.Deps, cfg foundation.Config) error {
	startPreclones(ctx, deps, csmithPackage{}.Meta(), cfg.Versions)

	switch runtime.GOOS {
	case "windows":
		// Linux-only package; host orchestration still needs docker on non-windows.
		return nil
	default:
		return prepLinuxHost(ctx, deps)
	}
}

func prepLinuxHost(ctx context.Context, deps foundation.Deps) error {
	if _, err := deps.Runner.Output(ctx, "docker", "version"); err != nil {
		return fmt.Errorf("docker required on linux host for package builds: %w", err)
	}
	deps.Logf("PrepHost(linux): docker OK")
	return nil
}
