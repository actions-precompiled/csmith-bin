package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/actions-precompiled/foundation"
)

// PrepHost implements foundation.HostPrep.
func (csmithPackage) PrepHost(ctx context.Context, deps foundation.Deps, cfg foundation.Config) error {
	startPreclones(ctx, deps, csmithPackage{}.Meta(), cfg.Versions)
	return prepHostTools(ctx, deps)
}

func prepHostTools(ctx context.Context, deps foundation.Deps) error {
	// mise exec already put conda: tools on PATH.
	tools := []struct {
		name string
		args []string
	}{
		{"cmake", []string{"--version"}},
		{"ninja", []string{"--version"}},
		{"m4", []string{"--version"}},
	}
	if runtime.GOOS != "windows" {
		cc, cxx := hostUnixCompilers()
		tools = append(tools,
			struct {
				name string
				args []string
			}{cc, []string{"--version"}},
			struct {
				name string
				args []string
			}{cxx, []string{"--version"}},
		)
	}
	for _, tool := range tools {
		out, err := deps.Runner.Output(ctx, tool.name, tool.args...)
		if err != nil {
			return fmt.Errorf("%w: %s (install via mise): %w", ErrHostToolMissing, tool.name, err)
		}
		deps.Logf("PrepHost: %s %s", tool.name, firstLine(out))
	}
	if runtime.GOOS == "windows" {
		return requireMSVC(ctx, deps)
	}
	return nil
}

func requireMSVC(ctx context.Context, deps foundation.Deps) error {
	out, err := deps.Runner.Output(ctx, "where", "cl")
	if err != nil {
		return fmt.Errorf("%w", ErrMSVCNotOnPATH)
	}
	deps.Logf("PrepHost: MSVC cl %s", firstLine(out))
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
