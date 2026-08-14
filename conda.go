package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/actions-precompiled/foundation"
)

const (
	condaChannel      = "conda-forge"
	condaCmakePin     = "cmake=3.31.6"
	condaNinjaPin     = "ninja=1.12.1"
	condaM4Pin        = "m4=1.4.21"
	envCondaPrefixKey = "APC_CONDA_PREFIX"
)

func condaPrefix(deps foundation.Deps) string {
	if p := deps.Env.Get(envCondaPrefixKey); p != "" {
		return p
	}
	return filepath.Join(deps.WorkDir, ".cache", "conda-env")
}

// condaCreatePkgs is the self-contained native toolchain.
// Windows uses MinGW (m2w64-*) — conda clangxx/cxx-compiler on win still
// want a Visual Studio install.
func condaCreatePkgs() []string {
	pkgs := []string{condaCmakePin, condaNinjaPin}
	if runtime.GOOS == "windows" {
		return append(pkgs, "m2w64-toolchain", "m2-m4")
	}
	return append(pkgs, "clang", "clangxx", "lld", condaM4Pin)
}

func condaCompilers() (cc, cxx string) {
	if runtime.GOOS == "windows" {
		return "gcc", "g++"
	}
	return "clang", "clang++"
}

func condaBinDirs(prefix string) []string {
	candidates := []string{
		filepath.Join(prefix, "bin"),
		filepath.Join(prefix, "Library", "bin"),
		filepath.Join(prefix, "Library", "usr", "bin"),
		filepath.Join(prefix, "Library", "mingw-w64", "bin"),
		filepath.Join(prefix, "Scripts"),
		prefix,
	}
	out := make([]string, 0, len(candidates))
	for _, d := range candidates {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

func prependPATH(env []string, dirs []string) []string {
	if len(dirs) == 0 {
		return env
	}
	extra := strings.Join(dirs, string(os.PathListSeparator))
	const key = "PATH="
	for i, e := range env {
		if strings.HasPrefix(e, key) || strings.HasPrefix(e, "Path=") {
			env[i] = e[:strings.IndexByte(e, '=')+1] + extra + string(os.PathListSeparator) + e[strings.IndexByte(e, '=')+1:]
			return env
		}
	}
	return append(env, "PATH="+extra)
}

func setEnvKV(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func condaEnviron(deps foundation.Deps, base []string) []string {
	prefix := condaPrefix(deps)
	env := append([]string{}, base...)
	env = prependPATH(env, condaBinDirs(prefix))
	env = setEnvKV(env, "CONDA_PREFIX", prefix)
	cc, cxx := condaCompilers()
	env = setEnvKV(env, "CC", cc)
	env = setEnvKV(env, "CXX", cxx)
	return env
}

// resolveCondaExe returns an absolute path under the env prefix.
// exec.LookPath uses the process PATH, not Cmd.Env — names like "m4" would
// miss m2-m4 on Windows and pick /usr/bin/m4 or host cmake on macOS.
func resolveCondaExe(prefix, name string) (string, error) {
	base := strings.TrimSuffix(name, ".exe")
	cands := []string{base}
	if runtime.GOOS == "windows" {
		cands = []string{base + ".exe", base}
	}
	var tried []string
	for _, dir := range condaBinDirs(prefix) {
		for _, c := range cands {
			p := filepath.Join(dir, c)
			tried = append(tried, p)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("%w: %s not under %s (tried %s)", ErrHostToolMissing, name, prefix, strings.Join(tried, ", "))
}

func runInConda(ctx context.Context, deps foundation.Deps, name string, args ...string) error {
	abs, err := resolveCondaExe(condaPrefix(deps), name)
	if err != nil {
		return err
	}
	env := condaEnviron(deps, deps.Env.Environ())
	if rw, ok := deps.Runner.(foundation.RunnerWithOpts); ok {
		return rw.RunWith(ctx, foundation.RunOpts{Env: env}, abs, args...)
	}
	return deps.Runner.Run(ctx, abs, args...)
}

func outputInConda(ctx context.Context, deps foundation.Deps, name string, args ...string) (string, error) {
	abs, err := resolveCondaExe(condaPrefix(deps), name)
	if err != nil {
		return "", err
	}
	env := condaEnviron(deps, deps.Env.Environ())
	if rw, ok := deps.Runner.(foundation.RunnerWithOpts); ok {
		return rw.OutputWith(ctx, foundation.RunOpts{Env: env}, abs, args...)
	}
	return deps.Runner.Output(ctx, abs, args...)
}

func ensureCondaEnv(ctx context.Context, deps foundation.Deps) error {
	prefix := condaPrefix(deps)
	if err := deps.FS.MkdirAll(filepath.Dir(prefix), 0o755); err != nil {
		return err
	}
	args := []string{"create", "-y", "-p", prefix, "-c", condaChannel, "--override-channels"}
	args = append(args, condaCreatePkgs()...)
	deps.Logf("PrepHost: micromamba %s", strings.Join(args, " "))
	if err := deps.Runner.Run(ctx, "micromamba", args...); err != nil {
		return fmt.Errorf("%w: micromamba create: %w", ErrCondaEnv, err)
	}
	return nil
}

func vendorLinkedDylibs(ctx context.Context, deps foundation.Deps, condaPref, destLib, bin string) error {
	out, err := deps.Runner.Output(ctx, "otool", "-L", bin)
	if err != nil {
		return fmt.Errorf("otool -L: %w", err)
	}
	if err := deps.FS.MkdirAll(destLib, 0o755); err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		path := line
		if i := strings.Index(line, " ("); i >= 0 {
			path = line[:i]
		}
		if path == bin || isSystemDylib(path) {
			continue
		}
		src := path
		if strings.HasPrefix(path, "@rpath/") {
			src = filepath.Join(condaPref, "lib", strings.TrimPrefix(path, "@rpath/"))
		}
		if _, err := deps.FS.Stat(src); err != nil {
			deps.Logf("vendor: skip missing %s", src)
			continue
		}
		name := filepath.Base(src)
		dest := filepath.Join(destLib, name)
		data, err := deps.FS.ReadFile(src)
		if err != nil {
			return err
		}
		if err := deps.FS.WriteFile(dest, data, 0o755); err != nil {
			return err
		}
		newRef := "@loader_path/../lib/" + name
		if err := deps.Runner.Run(ctx, "install_name_tool", "-change", path, newRef, bin); err != nil {
			return fmt.Errorf("install_name_tool -change %s: %w", path, err)
		}
		deps.Logf("vendor dylib %s -> lib/%s", path, name)
	}
	// Already-present rpath is not an error.
	_ = deps.Runner.Run(ctx, "install_name_tool", "-add_rpath", "@loader_path/../lib", bin)
	return nil
}

func isSystemDylib(path string) bool {
	return strings.HasPrefix(path, "/usr/lib/") ||
		strings.HasPrefix(path, "/System/") ||
		strings.HasPrefix(path, "/Library/Apple/")
}
