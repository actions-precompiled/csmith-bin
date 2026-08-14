package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/actions-precompiled/foundation"
)

func hostUnixCompilers() (cc, cxx string) {
	return "clang", "clang++"
}

// lookPathPrefix is the install root of a mise tool (parent of bin/).
func lookPathPrefix(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return filepath.Dir(filepath.Dir(p))
}

func vendorLinkedDylibs(ctx context.Context, deps foundation.Deps, toolPref, destLib, bin string) error {
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
		if strings.HasPrefix(path, "@rpath/") && toolPref != "" {
			src = filepath.Join(toolPref, "lib", strings.TrimPrefix(path, "@rpath/"))
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
	_ = deps.Runner.Run(ctx, "install_name_tool", "-add_rpath", "@loader_path/../lib", bin)
	return nil
}

func isSystemDylib(path string) bool {
	return strings.HasPrefix(path, "/usr/lib/") ||
		strings.HasPrefix(path, "/System/") ||
		strings.HasPrefix(path, "/Library/Apple/")
}
