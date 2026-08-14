package main

import (
	"errors"
	"testing"

	"github.com/actions-precompiled/foundation"
)

func TestArchiveSuffix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{foundation.TargetLinuxAMD64, foundation.TargetLinuxAMD64},
		{"linux-x86_64", foundation.TargetLinuxAMD64},
		{foundation.TargetLinuxAArch64, foundation.TargetLinuxAArch64},
		{"linux-arm64", foundation.TargetLinuxAArch64},
		{foundation.TargetWindowsAMD64, foundation.TargetWindowsAMD64},
		{foundation.TargetWindowsARM64, foundation.TargetWindowsARM64},
		{foundation.TargetDarwinAMD64, foundation.TargetDarwinAMD64},
		{"macos-amd64", foundation.TargetDarwinAMD64},
		{foundation.TargetDarwinAArch64, foundation.TargetDarwinAArch64},
		{"darwin-arm64", foundation.TargetDarwinAArch64},
		{"macos-arm64", foundation.TargetDarwinAArch64},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := archiveSuffix(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("archiveSuffix(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestArchiveSuffixUnknown(t *testing.T) {
	t.Parallel()
	_, err := archiveSuffix("solaris-sparc")
	if !errors.Is(err, ErrUnsupportedTarget) {
		t.Fatalf("got %v", err)
	}
}
