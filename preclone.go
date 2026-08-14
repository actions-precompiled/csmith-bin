package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/actions-precompiled/foundation"
)

type precloneResult struct {
	Src      string
	Ref      string
	Artifact string
	SHA      string
	Err      error
}

type precloneEntry struct {
	wg  sync.WaitGroup
	res precloneResult
}

var preclones sync.Map // version → *precloneEntry

func precloneCacheDir(workDir, version string) string {
	return filepath.Join(workDir, ".cache", "src", foundation.SafePathComponent(version))
}

func startPreclones(ctx context.Context, deps foundation.Deps, meta foundation.Meta, versions []string) {
	if deps.WorkDir == "" || len(versions) == 0 {
		return
	}
	meta = meta.Normalize()
	for _, v := range versions {
		v := strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, loaded := preclones.Load(v); loaded {
			continue
		}
		e := &precloneEntry{}
		if _, loaded := preclones.LoadOrStore(v, e); loaded {
			continue
		}
		e.wg.Add(1)
		src := precloneCacheDir(deps.WorkDir, v)
		deps.Logf("preclone: starting %s → %s", v, src)
		go func(version, src string, e *precloneEntry) {
			defer e.wg.Done()
			ref, art, sha, err := cloneUpstream(ctx, deps, meta.UpstreamGit, version, src)
			if err != nil {
				deps.Logf("preclone: %s failed: %v", version, err)
				e.res = precloneResult{Err: err}
				return
			}
			if err := applyUpstreamPatches(ctx, deps, src); err != nil {
				deps.Logf("preclone: %s patch: %v", version, err)
				e.res = precloneResult{Err: err}
				return
			}
			deps.Logf("preclone: %s ready ref=%s sha=%s", version, ref, sha)
			e.res = precloneResult{Src: src, Ref: ref, Artifact: art, SHA: sha}
		}(v, src, e)
	}
}

// WaitPrefetch implements optional foundation.PrefetchWaiter.
func (csmithPackage) WaitPrefetch(ctx context.Context, version string) error {
	_ = ctx
	e, ok := loadPreclone(version)
	if !ok {
		return nil
	}
	e.wg.Wait()
	return e.res.Err
}

func loadPreclone(version string) (*precloneEntry, bool) {
	v, ok := preclones.Load(version)
	if !ok {
		return nil, false
	}
	return v.(*precloneEntry), true
}

func resolveSource(ctx context.Context, deps foundation.Deps, meta foundation.Meta, version, fallbackSrc string) (src, ref, artifact, sha string, err error) {
	if pre := deps.Env.Get("APC_PREBUILT_SRC"); pre != "" {
		if st, e := deps.FS.Stat(pre); e == nil && st.IsDir() {
			ref, artifact, sha, err = gitIdentity(ctx, deps, pre, version)
			if err != nil {
				return "", "", "", "", err
			}
			deps.Logf("source: using prebuilt mount %s", pre)
			return pre, ref, artifact, sha, nil
		}
	}

	if e, ok := loadPreclone(version); ok {
		deps.Logf("source: waiting for preclone %s", version)
		e.wg.Wait()
		if e.res.Err != nil {
			return "", "", "", "", e.res.Err
		}
		return e.res.Src, e.res.Ref, e.res.Artifact, e.res.SHA, nil
	}

	ref, artifact, sha, err = cloneUpstream(ctx, deps, meta.UpstreamGit, version, fallbackSrc)
	return fallbackSrc, ref, artifact, sha, err
}

func gitIdentity(ctx context.Context, deps foundation.Deps, src, versionRaw string) (ref, artifact, sha string, err error) {
	out, err := deps.Runner.Output(ctx, "git", "-C", src, "rev-parse", "--short=12", "HEAD")
	if err != nil {
		return "", "", "", err
	}
	sha = strings.TrimSpace(out)
	out, err = deps.Runner.Output(ctx, "git", "-C", src, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		ref = versionRaw
	} else {
		ref = strings.TrimSpace(out)
		if ref == "HEAD" {
			ref = versionRaw
		}
	}
	artifact = artifactVersion(versionRaw, sha)
	return ref, artifact, sha, nil
}

// artifactVersion turns upstream tags into tarball version segments.
// csmith-2.3.0 → 2.3.0 so artifacts are csmith-2.3.0-linux-amd64.tar.gz.
func artifactVersion(versionRaw, sha string) string {
	v := strings.TrimSpace(versionRaw)
	switch {
	case v == "trunk" || v == "main" || v == "master" || strings.HasPrefix(v, "trunk-"):
		return "trunk-" + sha
	case strings.HasPrefix(v, "csmith-"):
		return strings.TrimPrefix(v, "csmith-")
	default:
		return foundation.VersionBare(v)
	}
}
