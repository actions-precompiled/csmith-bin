// Command apc builds the actions-precompiled Csmith distribution.
//
//	go run . list
//	go run . build csmith-2.3.0
//	go run . generate workflow --force
//
// All targets run Work on the host. Tools come from mise (conda: backend).
package main

import (
	"context"

	"github.com/actions-precompiled/foundation"
)

func main() {
	foundation.Main(csmithPackage{})
}

type csmithPackage struct{}

func (csmithPackage) Meta() foundation.Meta {
	return foundation.Meta{
		Name:            "csmith",
		UpstreamRepoAPI: "csmith-project/csmith",
		UpstreamGit:     "https://github.com/csmith-project/csmith.git",
		Binary:          "csmith",
		VersionEnv:      "CSMITH_VERSION",
		Description:     "Relocatable Csmith (random C program generator) for compiler testing.",
		Homepage:        "https://github.com/csmith-project/csmith",
		HostOnly:        true,
		DefaultTargets: []string{
			foundation.TargetLinuxAMD64,
			foundation.TargetLinuxAArch64,
			foundation.TargetWindowsAMD64,
			foundation.TargetDarwinAMD64,
			foundation.TargetDarwinAArch64,
		},
	}
}

func (p csmithPackage) Work(ctx context.Context, deps foundation.Deps, req foundation.BuildRequest) error {
	return workCMake(ctx, deps, p.Meta().Normalize(), req)
}

func (p csmithPackage) Smoke(ctx context.Context, deps foundation.Deps, req foundation.SmokeRequest) error {
	return smokeArtifacts(ctx, deps, p.Meta().Normalize(), req)
}
