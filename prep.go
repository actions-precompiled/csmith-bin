package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/actions-precompiled/foundation"
)

const windowsM4Tool = "conda:m2-m4@1.4.19.2"

// PrepHost implements foundation.HostPrep.
func (csmithPackage) PrepHost(ctx context.Context, deps foundation.Deps, cfg foundation.Config) error {
	startPreclones(ctx, deps, csmithPackage{}.Meta(), cfg.Versions)
	return prepHostTools(ctx, deps)
}

func prepHostTools(ctx context.Context, deps foundation.Deps) error {
	if err := ensureM4(ctx, deps); err != nil {
		return err
	}
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

// ensureM4 puts m4 on PATH. mise.windows.toml is not loaded by mise-action
// install (only mise.toml), and m2-m4's m4.exe lives under Library/usr/bin.
func ensureM4(ctx context.Context, deps foundation.Deps) error {
	if _, err := deps.Runner.Output(ctx, "m4", "--version"); err == nil {
		return nil
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("%w: m4 (install via mise)", ErrHostToolMissing)
	}
	deps.Logf("PrepHost: mise install %s", windowsM4Tool)
	if err := deps.Runner.Run(ctx, "mise", "install", windowsM4Tool); err != nil {
		return fmt.Errorf("%w: mise install %s: %w", ErrHostToolMissing, windowsM4Tool, err)
	}
	exe, err := findWindowsM4(windowsM4SearchRoots())
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	path := dir + string(os.PathListSeparator) + os.Getenv("PATH")
	if err := os.Setenv("PATH", path); err != nil {
		return err
	}
	deps.Logf("PrepHost: PATH prepend %s", dir)
	return nil
}

func isMiseShim(p string) bool {
	s := filepath.ToSlash(strings.ReplaceAll(p, `\`, "/"))
	return strings.Contains(s, "/shims/")
}

func resolveRealNinja() (string, error) {
	names := []string{"ninja"}
	if runtime.GOOS == "windows" {
		names = []string{"ninja.exe", "ninja"}
	}
	if p, err := exec.LookPath("ninja"); err == nil && !isMiseShim(p) {
		return p, nil
	}
	globs := []string{
		filepath.Join("conda-ninja", "*", "Library", "bin", "ninja.exe"),
		filepath.Join("conda-ninja", "*", "Library", "bin", "ninja"),
		filepath.Join("conda-ninja", "*", "bin", "ninja.exe"),
		filepath.Join("conda-ninja", "*", "bin", "ninja"),
		filepath.Join("ninja", "*", "bin", "ninja.exe"),
		filepath.Join("ninja", "*", "bin", "ninja"),
	}
	for _, root := range windowsM4SearchRoots() {
		for _, g := range globs {
			matches, err := filepath.Glob(filepath.Join(root, g))
			if err == nil && len(matches) > 0 {
				return matches[0], nil
			}
		}
		var found string
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			base := strings.ToLower(d.Name())
			for _, n := range names {
				if base == strings.ToLower(n) && !isMiseShim(p) {
					found = p
					return fs.SkipAll
				}
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("%w: ninja.exe under mise installs (not a shim)", ErrHostToolMissing)
}

func windowsM4SearchRoots() []string {
	var roots []string
	if d := os.Getenv("MISE_DATA_DIR"); d != "" {
		roots = append(roots, filepath.Join(d, "installs"))
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		roots = append(roots, filepath.Join(la, "mise", "installs"))
	}
	if h := os.Getenv("HOME"); h != "" {
		roots = append(roots, filepath.Join(h, ".local", "share", "mise", "installs"))
	}
	if h := os.Getenv("USERPROFILE"); h != "" {
		roots = append(roots, filepath.Join(h, ".local", "share", "mise", "installs"))
	}
	return roots
}

func findWindowsM4(roots []string) (string, error) {
	globs := []string{
		filepath.Join("conda-m2-m4", "*", "Library", "usr", "bin", "m4.exe"),
		filepath.Join("conda-m2-m4", "*", "Library", "bin", "m4.exe"),
		filepath.Join("conda-m2-m4", "*", "bin", "m4.exe"),
		filepath.Join("m2-m4", "*", "Library", "usr", "bin", "m4.exe"),
	}
	for _, root := range roots {
		for _, g := range globs {
			matches, err := filepath.Glob(filepath.Join(root, g))
			if err == nil && len(matches) > 0 {
				return matches[0], nil
			}
		}
		var found string
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.EqualFold(d.Name(), "m4.exe") {
				found = p
				return fs.SkipAll
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("%w: m4.exe under mise installs", ErrHostToolMissing)
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
