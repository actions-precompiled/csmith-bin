# actions-precompiled / csmith-bin

Relocatable **[Csmith](https://github.com/csmith-project/csmith)** builds with
[`foundation`](https://github.com/actions-precompiled/foundation) (Cobra CLI).

Csmith is a random generator of C programs (compiler differential testing).

**Tagged releases only** for publish — use upstream tags like `csmith-2.3.0`.

**Self-contained Linux trees** — post-install `patchelf` sets `$ORIGIN` RPATH;
smoke runs with a clean loader env (no `LD_LIBRARY_PATH`).

**Windows / macOS** build natively in a conda-forge env. `mise` installs
`micromamba`; PrepHost creates `.cache/conda-env` with cmake, ninja, m4, and
a compiler (MinGW on Windows, clang on macOS). Go stays on mise — that is the
org-wide package CLI toolchain.

**Linux stays Docker.** Artifacts are Ubuntu 24.04 glibc + `$ORIGIN` RPATH.
A conda compiler would vendor conda `libstdc++` and change that contract.

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
mise exec -- go run . build csmith-2.3.0      # host injects binary into Docker
mise exec -- go run . smoke csmith-2.3.0
mise exec -- go run . generate workflow --force
```

Bare versions work too: `go run . build 2.3.0` clones tag `csmith-2.3.0`.

### Architecture

| Layer | What runs |
|-------|-----------|
| Host | `go run . build <tag>` — plan, docker image (linux), native Work (windows/darwin), smoke, publish |
| Container | `/apc work` — Linux only |

Dockerfile is **deps only** (no shell `ENTRYPOINT`).

### CI

- `build.yml` — `go run . plan` then matrix `go run . build <tag>`
- `dispatch-missing.yml` — `go run . list` → one Build per **tag**

## Targets

- `linux-amd64`, `linux-aarch64` — Docker on GHA ubuntu runners
- `windows-amd64` — native on `windows-latest` (conda MinGW, statically linked)
- `darwin-amd64` — native on `macos-13` (conda clang; non-system dylibs vendored)
- `darwin-aarch64` — native on `macos-latest` (same)

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
- glibc is from Ubuntu 24.04 — older distros may not run the binary.

## License

MIT for packaging. Upstream Csmith is BSD-style (see upstream `COPYING`).
