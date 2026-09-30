# Maintenance-safe handoff — 2026-09-30

## Authorization

The owner has authorized implementation of stages A–D (spec 05 §2). Stages E–G
are still unauthorized. So are commits, pushes, automatic migration,
agent/tool renaming, and dependency or system installs made without asking.

## Background (unchanged)

- The TypeScript/OpenCode workplan hardening is complete and was independently
  reviewed. Its terminal artifacts are
  `~/.config/opencode/.opencode/workplan/workplan-tools-hardening.{json,md}`.
  Do not reopen it.
- Its recorded validation: 56 workplan tests passed with two opt-in skips, and
  the opt-in OpenCode 2.0.19/client 2.0.20 smokes passed.
- A broad comparison kept 24 unrelated failures and 1 error, so it was not a
  full-repository green run.
- Shiori was cloned from `git@github.com:hoshinoht/shiori.git` at `89c73ed`
  (`master`).

## Stage A — contract/fixture freeze: DONE (pending owner review)

Produced (nothing has been committed):

- `schema/`: versioned JSON Schema 2020-12 contract v1 ([index](../schema/index.json)).
  - Plan V2, checkpoint v1/v2 plus the union, dependencies v1, transaction
    journal v1, lock owner, and cursors.
  - The protocol envelope, PROPOSED.
  - Input schemas for all 13 `workplan_*` native tools, including
    `workplan_compact_preview`.
  - Unknown-field preservation notes and exact rule messages (`x-shiori-*`).
- `testdata/`: sanitized corpus. [MANIFEST](../testdata/MANIFEST.json) records
  the sha256 of every oracle file, bun 1.4.0, zod 4.1.8, the platform, the
  harness fingerprints, the normalization rules, and the sha256 of every file.
  - 21 fixtures (79 files).
  - 567 vector JSON files (624 files including expected Markdown and after-bytes):
    - hash 31
    - markdown 14
    - validation 92
    - tools 199
    - resume 87 (budgets 4096/12000/64000)
    - paging 74
    - mutations 70
  - Performance fixtures of 100 KiB, 1 MiB and 10 MiB, plus raw baseline results.
- [contracts.md](contracts.md): the frozen contract. It resolves every 05 §6
  item as PROPOSED, and lists 12 oracle-vs-spec differences (D1–D12) with
  proposals.
- [baseline.md](baseline.md): measured TypeScript latency (median/p95) and peak
  RSS for read, inspect, resume, validate and update at 100 KiB, 1 MiB and
  10 MiB on darwin/arm64.

Validation performed:

- The schemas are valid 2020-12. The storage schemas accept every valid fixture
  and reject every invalid one.
- The tool schemas give the same accept/reject result as the oracle's native
  parser on all 55 input vectors.
- Every read vector left bytes and mtimes unchanged.
- Each vector ran twice. All are deterministic except
  `mutations/compact-apply-valid`, which is root-dependent by design (D10).

Licensing: the oracle (partly GPL-2.0, by other authors) was executed only. Its
outputs are data. No reference source was copied or paraphrased. The harness
scripts live outside this repository, and only their fingerprints are recorded.

## Next: stage B — read-only Go core

This needs the owner's review first:

1. Approve or amend the PROPOSED items in `contracts.md` §2, §5 and §6, and
   D1–D12.
2. Approve adding `go.mod`, and the test-only JSON Schema validator dependency
   (contracts §5.1).

Then implement:

- decode, snapshot and hash (with the UTF-16 ordering and JSON.stringify
  escaping vectors)
- list, read, inspect, validate, resume and doctor
- the markdown renderer and indexes

Stage B must not write artifacts or change mtimes. The gates are:

- `testdata/vectors/{hash,markdown,validation,tools,resume,paging}` parity
- the D1 read-only interrupted-state hash
- the same baseline table re-measured for Go

## Resume after maintenance

1. Read this file, `docs/contracts.md` and the specs. No workplan plugin is
   needed.
2. Inspect `git status`. As of this handoff, `README.md`, `docs/`, `schema/` and
   `testdata/` are untracked; only `LICENSE` and `.gitignore` are committed
   (`89c73ed`). Preserve any user edits.
3. If the oracle files change, the corpus is stale. Compare their sha256 against
   `testdata/MANIFEST.json` → `oracle.files`.

The Go toolchain observed is 1.27.1 darwin/arm64. No `.go` files and no `go.mod`
exist yet.
