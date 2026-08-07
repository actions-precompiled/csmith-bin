package main

import "errors"

var (
	ErrSmokeNoTarballs   = errors.New("smoke: no tarballs")
	ErrUnsupportedTarget = errors.New("unsupported linux target")
	ErrCloneFailed       = errors.New("clone upstream failed")
	ErrCsmithMissing     = errors.New("csmith missing after install")
	ErrGenerateEmpty     = errors.New("csmith generated empty program")
	ErrCompileGenerated  = errors.New("host gcc failed on generated C")
)
