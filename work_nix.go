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

// workLinux builds Csmith inside the container (or any Linux host with deps).
// Invoked as: /apc work
func workLinux(ctx context.Context, deps foundation.Deps, meta foundation.Meta, req foundation.BuildRequest) error {
	archiveSuffix, err := linuxArchiveSuffix(req.Target)
	if err != nil {
		return err
	}

	jobs := envOr(deps, "JOBS", strconv.Itoa(runtime.NumCPU()))
	work := filepath.Join("/tmp", meta.Name+"-build")
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

	// In-source layout is OK for csmith; out-of-source is cleaner for rebuilds.
	cmakeArgs := []string{
		"-G", "Ninja",
		"-S", src,
		"-B", build,
		"-DCMAKE_BUILD_TYPE=Release",
		"-DCMAKE_INSTALL_PREFIX=" + prefix,
		"-DCMAKE_INSTALL_RPATH=$ORIGIN/../lib",
		"-DCMAKE_BUILD_RPATH_USE_ORIGIN=ON",
		"-DCMAKE_BUILD_WITH_INSTALL_RPATH=ON",
		"-DCMAKE_INSTALL_RPATH_USE_LINK_PATH=OFF",
	}
	if err := deps.Runner.Run(ctx, "cmake", cmakeArgs...); err != nil {
		return fmt.Errorf("cmake configure: %w", err)
	}
	if err := deps.Runner.Run(ctx, "cmake", "--build", build, "--parallel", jobs); err != nil {
		return fmt.Errorf("cmake build: %w", err)
	}
	if err := deps.Runner.Run(ctx, "cmake", "--install", build); err != nil {
		return fmt.Errorf("cmake install: %w", err)
	}

	deps.RemoveAllLog(build, "remove")
	if deps.Env.Get("APC_PREBUILT_SRC") == "" && !isUnderCache(src) {
		deps.RemoveAllLog(src, "remove")
	}

	bin := filepath.Join(prefix, "bin", "csmith")
	if _, err := deps.FS.Stat(bin); err != nil {
		// Some older tags install to prefix root
		alt := filepath.Join(prefix, "csmith")
		if _, err2 := deps.FS.Stat(alt); err2 == nil {
			if err := deps.FS.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
				return err
			}
			if err := deps.Runner.Run(ctx, "mv", alt, bin); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%w under %s", ErrCsmithMissing, prefix)
		}
	}

	// Upstream also installs Perl drivers + data into bin/ (compiler_test.pl,
	// launchn.pl, compiler_test.in). Those need a full source tree and break
	// SmokeBinDirHelp (exec format error on .in). Ship only the generator.
	if err := pruneBinToCsmith(deps, filepath.Join(prefix, "bin")); err != nil {
		return err
	}

	// Drop cmake/pkgconfig metadata. Keep include/ + shared libs.
	_ = deps.Runner.Run(ctx, "bash", "-c",
		"rm -rf "+shellQuote(filepath.Join(prefix, "lib", "cmake"))+" "+
			shellQuote(filepath.Join(prefix, "lib", "pkgconfig"))+" 2>/dev/null; true")

	if err := foundation.PatchLinuxOriginRPath(ctx, deps, prefix); err != nil {
		return fmt.Errorf("relocatable rpath: %w", err)
	}
	if err := foundation.CheckLinuxRelocatable(prefix, foundation.RelocatableOpts{
		RequiredBins: []string{"csmith"},
	}); err != nil {
		return fmt.Errorf("post-install relocatable check: %w", err)
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

	archive := filepath.Join(req.OutDir, foundation.ArtifactName(meta.Name, artifactVer, archiveSuffix))
	if err := deps.Runner.Run(ctx, "tar", "-czf", archive, "-C", stage, meta.Name); err != nil {
		return fmt.Errorf("tar: %w", err)
	}
	deps.Logf("Done: %s", archive)
	return nil
}

func linuxArchiveSuffix(target string) (string, error) {
	switch target {
	case foundation.TargetLinuxAMD64, "linux-x86_64":
		return foundation.TargetLinuxAMD64, nil
	case foundation.TargetLinuxAArch64, "linux-arm64":
		return foundation.TargetLinuxAArch64, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedTarget, target)
	}
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
		if e.Name() == "csmith" {
			continue
		}
		path := filepath.Join(binDir, e.Name())
		deps.Logf("prune bin: remove %s", e.Name())
		deps.RemoveAllLog(path, "prune")
	}
	if _, err := deps.FS.Stat(filepath.Join(binDir, "csmith")); err != nil {
		return fmt.Errorf("%w after prune", ErrCsmithMissing)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func isUnderCache(src string) bool {
	s := filepath.ToSlash(src)
	return strings.Contains(s, "/.cache/src/") || s == "/src" || strings.HasPrefix(s, "/src/")
}
