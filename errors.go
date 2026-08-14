package main

import "errors"

var (
	ErrSmokeNoTarballs   = errors.New("smoke: no tarballs")
	ErrUnsupportedTarget = errors.New("unsupported target")
	ErrCloneFailed       = errors.New("clone upstream failed")
	ErrCsmithMissing     = errors.New("csmith missing after install")
	ErrGenerateEmpty     = errors.New("csmith generated empty program")
	ErrCompileGenerated  = errors.New("host compiler failed on generated C")
	ErrHostToolMissing   = errors.New("required host build tool missing")
	ErrCondaEnv          = errors.New("conda env setup failed")
	ErrMSVCNotOnPATH     = errors.New("cl.exe not on PATH; run from VS dev shell or GHA msvc-dev-cmd")
)
