# Shiori

**Shiori (しおり, “bookmark”)** is a proposed local-first work coordination
engine: preserve the plan, execution state, safety boundaries and the next
reliable action across workers and sessions.

This repository contains specifications, the stage A contract (machine
schemas, a golden fixture corpus and a measured TypeScript baseline), and no
Go implementation yet. The proposed architecture is a Go core and CLI with a
thin OpenCode JS/TS adapter. Stages A–D are authorized; automatic artifact
migration, commits and remote publication are not.

## Specifications

Read in this order:

1. [Core architecture and storage](docs/specs/01-core.md)
2. [Protocol and OpenCode adapter](docs/specs/02-protocol-and-adapter.md)
3. [Performance and data structures](docs/specs/03-performance.md)
4. [Worktree-aware execution](docs/specs/04-worktrees.md)
5. [Migration and acceptance](docs/specs/05-migration-and-acceptance.md)

Stage A contract, for review:

- [Frozen contract v1 (PROPOSED)](docs/contracts.md)
- [Machine schemas](schema/index.json) and [golden corpus](testdata/MANIFEST.json)
- [TypeScript reference baseline](docs/baseline.md)

[Maintenance-safe status and handoff](docs/STATUS.md) records completed work,
validation limits and the next authorized decision without requiring OpenCode tools.

The baseline is the completed TypeScript workplan hardening in the local
OpenCode preset. Shiori should retain its tested behavior and existing
`.opencode/workplan/` V2 artifacts and `workplan_*` tool identities during
migration, rather than rewriting plans or changing agent names automatically.

Proposed Go module: `github.com/hoshinoht/shiori`.
Proposed binary: `shiori`. Local specification authoring observed
Go `1.27.1` on Darwin/arm64; that is not evidence of Go implementation or
cross-platform validation.

`LICENSE` and `.gitignore` are retained from the repository's initial commit.

## Next decision

Review the PROPOSED items and oracle differences in
[docs/contracts.md](docs/contracts.md), then start stage B, the read-only Go
core. Worktree orchestration and alternative storage remain later, explicitly
gated stages.
