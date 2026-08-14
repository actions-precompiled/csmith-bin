# actions-precompiled / csmith-bin

Relocatable **[Csmith](https://github.com/csmith-project/csmith)** builds with
[`foundation`](https://github.com/actions-precompiled/foundation) (Cobra CLI).

Csmith is a random generator of C programs (compiler differential testing).

**Tagged releases only** for publish — use upstream tags like `csmith-2.3.0`.

Every target builds on the GHA host. `mise install` pulls cmake, ninja, m4,
and clang from conda-forge (`mise exec` puts them on PATH). Windows compiles
with MSVC `cl`. After clone we apply the same 2.3.0 backports as
[conda-forge/staged-recipes#34531](https://github.com/conda-forge/staged-recipes/pull/34531).

Linux trees still get `$ORIGIN` RPATH via `patchelf`.

## Layout

```text
csmith-2.3.0-linux-amd64.tar.gz
└── csmith/
    ├── bin/csmith
    ├── include/csmith-*/   # headers for compiling generated C
    ├── lib/                # optional shared deps / libcsmith
    └── BUILDINFO.txt
```

## CLI

```bash
mise install
mise exec -- go run . plan                    # → latest missing/upstream tag
mise exec -- go run . list                    # missing tags (one per line)
mise exec -- go run . list --all
mise exec -- go run . build csmith-2.3.0
mise exec -- go run . smoke csmith-2.3.0
mise exec -- go run . generate workflow --force
```

Bare versions work too: `go run . build 2.3.0` clones tag `csmith-2.3.0`.

### Architecture

| Layer | What runs |
|-------|-----------|
| Host | `mise exec -- go run . build <tag>` — Work + smoke + publish |

### CI

- `build.yml` — `go run . plan` then matrix `go run . build <tag>`
- `dispatch-missing.yml` — `go run . list` → one Build per **tag**

## Targets

- `linux-amd64`, `linux-aarch64` — host Work on ubuntu runners (mise clang)
- `windows-amd64` — `windows-latest` (MSVC `cl` + mise cmake/ninja/m4)
- `darwin-amd64` — `macos-13` (mise clang)
- `darwin-aarch64` — `macos-latest` (mise clang)

## Use after unpack

```bash
tar -xzf csmith-2.3.0-linux-amd64.tar.gz
export PATH="$PWD/csmith/bin:$PATH"
csmith --seed 1 > random.c
cc -I"$PWD/csmith/include/csmith-"* -o random random.c
./random
```

## Notes

- Upstream also has historical `git-conversion-*` tags; prefer `csmith-X.Y.Z`.
- Linux binaries still need a recent glibc (GHA ubuntu-24.04).

## License

MIT for packaging. Upstream Csmith is BSD-style (see upstream `COPYING`).
