# Shiori

**Shiori (しおり, “bookmark”)** is a proposed local-first work coordination
engine: preserve the plan, execution state, safety boundaries and the next
reliable action across workers and sessions.

This repository contains the specifications, the approved stage A contract
(machine schemas, a golden fixture corpus and a measured TypeScript baseline)
and the stage B read-only Go core with its `shiori` CLI. The architecture is a
Go core and CLI with a thin OpenCode JS/TS adapter (stage D). Stages A–D are
authorized; automatic artifact migration, commits and remote publication are
not. Nothing in stage B writes to a workspace.

## Specifications

Read in this order:

1. [Core architecture and storage](docs/specs/01-core.md)
2. [Protocol and OpenCode adapter](docs/specs/02-protocol-and-adapter.md)
3. [Performance and data structures](docs/specs/03-performance.md)
4. [Worktree-aware execution](docs/specs/04-worktrees.md)
5. [Migration and acceptance](docs/specs/05-migration-and-acceptance.md)

Stage A contract:

- [Frozen contract v1 (APPROVED 2026-09-30)](docs/contracts.md)
- [Machine schemas](schema/index.json) and [golden corpus](testdata/MANIFEST.json)
- [TypeScript reference baseline and Go stage B results](docs/baseline.md)

[Maintenance-safe status and handoff](docs/STATUS.md) records completed work,
validation limits and the next authorized decision without requiring OpenCode tools.

The baseline is the completed TypeScript workplan hardening in the local
OpenCode preset. Shiori should retain its tested behavior and existing
`.opencode/workplan/` V2 artifacts and `workplan_*` tool identities during
migration, rather than rewriting plans or changing agent names automatically.

Go module: `github.com/hoshinoht/shiori`. Binary: `shiori`. Stage B was built
and validated with Go 1.27.1 on darwin/arm64 only. That is not evidence of
cross-platform validation (contracts §5.6).

## Build and run

Requires Go 1.27.1 or newer. The binary has no runtime dependencies.

```sh
CGO_ENABLED=0 go build -trimpath -o shiori ./cmd/shiori
```

Stage B operations are all read-only. They never prompt, write, lock, repair
or change a file's mtime.

```sh
shiori list     --root /path/to/project
shiori read     <id> [--phase ID] [--step ID] [--no-markdown]
shiori inspect  <id> [--phase ID] [--limit 1-500] [--cursor TOKEN]
shiori validate <id>                       # exit 1 when the plan is invalid
shiori resume   <id> [--max-chars 4096-64000] [--limit 1-100] [--cursor TOKEN] [--phase ID] [--step ID]
shiori doctor   [id] [--limit 1-100]
```

- `--root` defaults to the current directory.
- `--json` prints exactly the result text that the corresponding `workplan_*`
  tool returns. Errors print `{"ok": false, "error": {class, message, issues?}}`
  with a protocol error class.
- `--input '<json>'` passes a raw core-surface tool input object instead of
  flags.
- Exit status: 0 success; 1 operation error, or an invalid plan for
  `validate`; 2 usage error.

## Test

```sh
go vet ./...
go test ./...
go test -race ./...
go test ./internal/ojson -run '^$' -fuzz '^FuzzParse$' -fuzztime 30s   # also FuzzQuote,
                                                                        # snapshot FuzzManifestJSON, engine FuzzCursorDecode
go test ./internal/engine -run '^$' -bench Ops -benchmem                 # perf fixtures, 100 KiB–10 MiB
SHIORI_BASELINE=/tmp/go-baseline.json go test ./internal/engine -run '^TestBaselineMatrix$' -v
```

- `TestCorpusParity` runs every `tools/`, `resume/` and `paging/` vector at a
  root of the generation root's length, under `/private/tmp`, and fails on
  any byte, mode, mtime or file-set change.
- The declared divergences (contracts §7 fixes D1, D4, D5, D6) are checked by
  dedicated comparators; see [STATUS](docs/STATUS.md).
- The JSON Schema validator is a test-only dependency and is not linked into
  the binary.

`LICENSE` and `.gitignore` are retained from the repository's initial commit.

## Next decision

Review and commit stage B (see [STATUS](docs/STATUS.md)). The next stage is C,
the transactional core: prepared intents, locks, journals, recovery and
compaction. Worktree orchestration and alternative storage remain later,
explicitly gated stages.
