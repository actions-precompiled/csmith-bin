package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/actions-precompiled/foundation"
)

// workCMake builds Csmith with cmake+ninja on the current host (or inside
// the Linux build container as `/apc work`).
func workCMake(ctx context.Context, deps foundation.Deps, meta foundation.Meta, req foundation.BuildRequest) error {
	suffix, err := archiveSuffix(req.Target)
	if err != nil {
		return err
	}

	jobs := envOr(deps, "JOBS", strconv.Itoa(runtime.NumCPU()))
	work := buildWorkRoot(deps, meta)
	src := filepath.Join(work, "src")
	build := filepath.Join(work, "build")
	stage := filepath.Join(work, "stage")
	prefix := filepath.Join(stage, meta.Name)

	deps.RemoveAllLog(build, "remove")
	deps.RemoveAllLog(stage, "remove")
	for _, d := range []string{build, prefix, req.OutDir} {
		if err := deps.FS.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	src, ref, artifactVer, sha, err := resolveSource(ctx, deps, meta, req.Version, src)
	if err != nil {
		return err
	}
	deps.Logf("Resolved ref=%s sha=%s artifact=%s src=%s", ref, sha, artifactVer, src)

	src, err = ensurePatchedSrc(ctx, deps, src, filepath.Join(work, "src"))
	if err != nil {
		return err
	}

	cmakeArgs := []string{
		"-G", "Ninja",
		"-S", src,
		"-B", build,
		"-DCMAKE_BUILD_TYPE=Release",
		"-DCMAKE_INSTALL_PREFIX=" + prefix,
		// Same knobs as conda-forge/staged-recipes#34531.
		"-DCMAKE_POLICY_VERSION_MINIMUM=3.5",
		"-DCMAKE_CXX_STANDARD=14",
	}
	cmakeArgs = append(cmakeArgs, cmakeRPathArgs(req.Target)...)
	if foundation.IsWindowsTarget(req.Target) {
		// GHA PATH has LLVM clang first; keep MSVC for the Windows binary.
		cmakeArgs = append(cmakeArgs,
			"-DCMAKE_C_COMPILER=cl",
			"-DCMAKE_CXX_COMPILER=cl",
		)
		if err := disableWindowsSharedRuntime(deps, src); err != nil {
			return err
		}
	}
	if foundation.IsDarwinTarget(req.Target) {
		pref := condaPrefix(deps)
		cc, cxx := condaCompilers()
		ccAbs, err := resolveCondaExe(pref, cc)
		if err != nil {
			return err
		}
		cxxAbs, err := resolveCondaExe(pref, cxx)
		if err != nil {
			return err
		}
		cmakeArgs = append(cmakeArgs,
			"-DCMAKE_C_COMPILER="+ccAbs,
			"-DCMAKE_CXX_COMPILER="+cxxAbs,
		)
	}
	run := deps.Runner.Run
	if foundation.IsNativeTarget(req.Target) {
		run = func(ctx context.Context, name string, args ...string) error {
			return runInConda(ctx, deps, name, args...)
		}
	}
	if err := run(ctx, "cmake", cmakeArgs...); err != nil {
		return fmt.Errorf("cmake configure: %w", err)
	}
	if err := run(ctx, "cmake", "--build", build, "--parallel", jobs); err != nil {
		return fmt.Errorf("cmake build: %w", err)
	}
	if err := run(ctx, "cmake", "--install", build); err != nil {
		return fmt.Errorf("cmake install: %w", err)
	}

	deps.RemoveAllLog(build, "remove")
	if deps.Env.Get("APC_PREBUILT_SRC") == "" && !isUnderCache(src) {
		deps.RemoveAllLog(src, "remove")
	}

	if err := ensureCsmithInBin(deps, prefix); err != nil {
		return err
	}

	// Upstream also installs Perl drivers + data into bin/ (compiler_test.pl,
	// launchn.pl, compiler_test.in). Those need a full source tree and break
	// SmokeBinDirHelp (exec format error on .in). Ship only the generator.
	if err := pruneBinToCsmith(deps, filepath.Join(prefix, "bin")); err != nil {
		return err
	}

	deps.RemoveAllLog(filepath.Join(prefix, "lib", "cmake"), "prune")
	deps.RemoveAllLog(filepath.Join(prefix, "lib", "pkgconfig"), "prune")

	switch {
	case foundation.IsDarwinTarget(req.Target):
		bin, err := findCsmithBin(deps, prefix)
		if err != nil {
			return err
		}
		if err := vendorLinkedDylibs(ctx, deps, condaPrefix(deps), filepath.Join(prefix, "lib"), bin); err != nil {
			return err
		}
	case !foundation.IsNativeTarget(req.Target):
		if err := foundation.PatchLinuxOriginRPath(ctx, deps, prefix); err != nil {
			return fmt.Errorf("relocatable rpath: %w", err)
		}
		if err := foundation.CheckLinuxRelocatable(prefix, foundation.RelocatableOpts{
			RequiredBins: []string{"csmith"},
		}); err != nil {
			return fmt.Errorf("post-install relocatable check: %w", err)
		}
	}

	info := fmt.Sprintf(`package=%s
version=%s
upstream_ref=%s
upstream_sha=%s
build_target=%s
distributor=actions-precompiled
built_at=%s
`, meta.Name, artifactVer, ref, sha, req.Target, time.Now().UTC().Format(time.RFC3339))
	if err := deps.FS.WriteFile(filepath.Join(prefix, "BUILDINFO.txt"), []byte(info), 0o644); err != nil {
		return err
	}

	archive := filepath.Join(req.OutDir, foundation.ArtifactName(meta.Name, artifactVer, suffix))
	if err := deps.Runner.Run(ctx, "tar", "-czf", archive, "-C", stage, meta.Name); err != nil {
		return fmt.Errorf("tar: %w", err)
	}
	deps.Logf("Done: %s", archive)
	return nil
}

func buildWorkRoot(deps foundation.Deps, meta foundation.Meta) string {
	if w := deps.Env.Get("APC_WORK_ROOT"); w != "" {
		return filepath.Join(w, meta.Name+"-build")
	}
	if runtime.GOOS == "windows" {
		tmp := deps.Env.Get("TEMP")
		if tmp == "" {
			tmp = deps.Env.Get("TMP")
		}
		if tmp == "" {
			tmp = "."
		}
		return filepath.Join(tmp, meta.Name+"-build")
	}
	return filepath.Join("/tmp", meta.Name+"-build")
}

func cmakeRPathArgs(target string) []string {
	switch {
	case foundation.IsWindowsTarget(target):
		return nil
	case foundation.IsDarwinTarget(target):
		return []string{
			"-DCMAKE_INSTALL_RPATH=@loader_path/../lib",
			"-DCMAKE_BUILD_WITH_INSTALL_RPATH=ON",
		}
	default:
		return []string{
			"-DCMAKE_INSTALL_RPATH=$ORIGIN/../lib",
			"-DCMAKE_BUILD_RPATH_USE_ORIGIN=ON",
			"-DCMAKE_BUILD_WITH_INSTALL_RPATH=ON",
			"-DCMAKE_INSTALL_RPATH_USE_LINK_PATH=OFF",
		}
	}
}

func archiveSuffix(target string) (string, error) {
	switch target {
	case foundation.TargetLinuxAMD64, "linux-x86_64":
		return foundation.TargetLinuxAMD64, nil
	case foundation.TargetLinuxAArch64, "linux-arm64":
		return foundation.TargetLinuxAArch64, nil
	case foundation.TargetWindowsAMD64, "windows-x86_64":
		return foundation.TargetWindowsAMD64, nil
	case foundation.TargetWindowsARM64, "windows-aarch64":
		return foundation.TargetWindowsARM64, nil
	case foundation.TargetDarwinAMD64, "macos-amd64", "macos-x86_64":
		return foundation.TargetDarwinAMD64, nil
	case foundation.TargetDarwinAArch64, "darwin-arm64", "macos-arm64", "macos-aarch64":
		return foundation.TargetDarwinAArch64, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedTarget, target)
	}
}

func ensureCsmithInBin(deps foundation.Deps, prefix string) error {
	if _, err := findCsmithBin(deps, prefix); err == nil {
		return nil
	}
	// Some older tags install to prefix root.
	for _, name := range []string{"csmith.exe", "csmith"} {
		alt := filepath.Join(prefix, name)
		if _, err := deps.FS.Stat(alt); err != nil {
			continue
		}
		binDir := filepath.Join(prefix, "bin")
		if err := deps.FS.MkdirAll(binDir, 0o755); err != nil {
			return err
		}
		dest := filepath.Join(binDir, name)
		data, err := deps.FS.ReadFile(alt)
		if err != nil {
			return err
		}
		if err := deps.FS.WriteFile(dest, data, 0o755); err != nil {
			return err
		}
		deps.RemoveAllLog(alt, "move")
		return nil
	}
	return fmt.Errorf("%w under %s", ErrCsmithMissing, prefix)
}

func cloneUpstream(ctx context.Context, deps foundation.Deps, upstream, versionRaw, src string) (ref, artifact, sha string, err error) {
	tryClone := func(branch string) error {
		deps.RemoveAllLog(src, "remove")
		return deps.Runner.Run(ctx, "git", "clone", "--depth", "1", "--branch", branch, upstream, src)
	}

	switch {
	case versionRaw == "trunk" || versionRaw == "main" || versionRaw == "master" || versionRaw == "git-main" || strings.HasPrefix(versionRaw, "trunk-"):
		ref = "master"
		if err := tryClone("master"); err != nil {
			ref = "main"
			if err2 := tryClone("main"); err2 != nil {
				return "", "", "", fmt.Errorf("%w: %v / %v", ErrCloneFailed, err, err2)
			}
		}
	default:
		ref = versionRaw
		if err := tryClone(ref); err != nil {
			// Accept bare "2.3.0" as "csmith-2.3.0"
			if !strings.HasPrefix(versionRaw, "csmith-") {
				ref = "csmith-" + strings.TrimPrefix(versionRaw, "v")
				if err2 := tryClone(ref); err2 != nil {
					return "", "", "", fmt.Errorf("%w: %s: %w", ErrCloneFailed, versionRaw, err)
				}
			} else {
				return "", "", "", fmt.Errorf("%w: %w", ErrCloneFailed, err)
			}
		}
	}

	out, err := deps.Runner.Output(ctx, "git", "-C", src, "rev-parse", "--short=12", "HEAD")
	if err != nil {
		return "", "", "", err
	}
	sha = strings.TrimSpace(out)
	artifact = artifactVersion(versionRaw, sha)
	return ref, artifact, sha, nil
}

func envOr(deps foundation.Deps, key, def string) string {
	if v := deps.Env.Get(key); v != "" {
		return v
	}
	return def
}

// pruneBinToCsmith removes everything under bin/ except the csmith binary.
func pruneBinToCsmith(deps foundation.Deps, binDir string) error {
	entries, err := deps.FS.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("read bin: %w", err)
	}
	for _, e := range entries {
		if isCsmithBinName(e.Name()) {
			continue
		}
		path := filepath.Join(binDir, e.Name())
		deps.Logf("prune bin: remove %s", e.Name())
		deps.RemoveAllLog(path, "prune")
	}
	if _, err := findCsmithBin(deps, filepath.Dir(binDir)); err != nil {
		return fmt.Errorf("%w after prune", ErrCsmithMissing)
	}
	return nil
}

func isCsmithBinName(name string) bool {
	return name == "csmith" || name == "csmith.exe"
}

func findCsmithBin(deps foundation.Deps, root string) (string, error) {
	for _, name := range []string{"csmith.exe", "csmith"} {
		p := filepath.Join(root, "bin", name)
		if _, err := deps.FS.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: bin/csmith", ErrCsmithMissing)
}

func isUnderCache(src string) bool {
	s := filepath.ToSlash(src)
	return strings.Contains(s, "/.cache/src/") || s == "/src" || strings.HasPrefix(s, "/src/")
}

// disableWindowsSharedRuntime skips libcsmith_so. On Windows both the static
// lib and the DLL import lib are named csmith.lib; Ninja rejects the clash.
func disableWindowsSharedRuntime(deps foundation.Deps, src string) error {
	path := filepath.Join(src, "runtime", "CMakeLists.txt")
	data, err := deps.FS.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read runtime CMakeLists: %w", err)
	}
	out, ok := wrapSharedRuntimeCMake(string(data))
	if !ok {
		return fmt.Errorf("runtime CMakeLists: no libcsmith_so block to wrap")
	}
	if out == string(data) {
		return nil
	}
	deps.Logf("Windows: skip shared libcsmith (duplicate csmith.lib)")
	return deps.FS.WriteFile(path, []byte(out), 0o644)
}

func wrapSharedRuntimeCMake(s string) (string, bool) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !strings.Contains(s, "add_library(libcsmith_so") {
		return s, false
	}
	if strings.Contains(s, "APC_SKIP_SHARED_RUNTIME") {
		return s, true
	}
	const start = "# Build and install the shared library."
	idx := strings.Index(s, start)
	if idx < 0 {
		idx = strings.Index(s, "add_library(libcsmith_so")
	}
	if idx < 0 {
		return s, false
	}
	const end = "RUNTIME DESTINATION \"${LIB_DIR}\"\n  )"
	endIdx := strings.Index(s[idx:], end)
	if endIdx < 0 {
		return s, false
	}
	endIdx = idx + endIdx + len(end)
	var b strings.Builder
	b.WriteString(s[:idx])
	b.WriteString("if(NOT WIN32) # APC_SKIP_SHARED_RUNTIME\n")
	b.WriteString(s[idx:endIdx])
	b.WriteString("\nendif() # APC_SKIP_SHARED_RUNTIME")
	b.WriteString(s[endIdx:])
	return b.String(), true
}
