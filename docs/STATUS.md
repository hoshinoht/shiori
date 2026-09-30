# Maintenance-safe handoff — 2026-09-30

## Authorization

The owner has authorized implementation of stages A–D (spec 05 §2). Stages E–G
are still unauthorized. So are commits, pushes, automatic migration,
agent/tool renaming, and dependency or system installs made without asking.

Approved 2026-09-30 (recorded in [contracts.md §9](contracts.md)): every
stage A PROPOSED item, the D1–D12 resolutions (as narrow bug fixes that keep
the original workplan design), `go.mod` for `github.com/hoshinoht/shiori`, the
test-only validator `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3, and
keeping the compressed perf fixtures in git.

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

## Stage A — contract/fixture freeze: DONE (approved 2026-09-30, committed `fdadfd4`)

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

## Stage B — read-only Go core: DONE (uncommitted, for owner review)

Nothing was committed. All stage B changes are in the working tree:
`go.mod`, `go.sum`, `cmd/`, `internal/`, `testdata/perf/go-baseline-results.json`
and edits to `README.md`, `docs/contracts.md`, `docs/baseline.md` and this file.

### What exists

| Package | Role |
| --- | --- |
| `internal/ojson` | Order-preserving JSON values; strict parser (duplicate-member detection, raw number spelling, depth 128); JSON.stringify-exact compact/pretty encoder; UTF-16 length, comparison and truncation |
| `internal/model` | Compatible V2 decode with the reference's issue text and paths; structure rules; id normalization; checkpoint v1/v2, dependencies, journal and lock-owner decoders; Markdown renderer and markers |
| `internal/snapshot` | Read-only artifact reader with size limits; plan-file/spec path policy; manifests and `workplan-plan-v1`/`workplan-state-v1` hashes; D1 interrupted-state hash |
| `internal/index` | `phaseById`, composite `StepKey{PhaseID, StepID}` map, DAG adjacency (prerequisites/dependents) with cycle detection, ordered severity buckets, marker index with collision detection |
| `internal/engine` | `list`, `read`, `inspect`, `validate`, `resume`, `doctor`; tool-input validation for those six tools (core and native surfaces); checksummed cursors |
| `internal/cli`, `cmd/shiori` | The `shiori` CLI: human output by default, `--json` for the exact tool result |
| `internal/testutil`, `internal/schematest` | Corpus runner helpers; schema agreement tests (test-only validator) |

`storage` and `protocol` packages are not created; stage B does not need them.

### Evidence (darwin/arm64, Go 1.27.1)

- `go vet ./...`: clean. `go test ./...` and `go test -race ./...`: all
  packages pass.
- Corpus parity (`TestCorpusParity`, `TestInputVectors`, model/snapshot tests):

  | Category (vector files) | Pass | Intentional divergence | Deferred to stage C | Fail |
  | --- | --- | --- | --- | --- |
  | hash (31: 11 manifest cases, 29 fixture snapshots, 11-row invalidation table) | 31 | 0 | 0 | 0 |
  | markdown (14: 13 render cases, generated classification over 29 rows) | 13 | 1 | 0 | 0 |
  | validation/structure | 37 | 0 | 0 | 0 |
  | validation/input | 16 | 0 | 39 | 0 |
  | tools | 178 | 19 | 2 | 0 |
  | resume | 87 | 0 | 0 | 0 |
  | paging | 74 | 0 | 0 | 0 |

  - "Pass" is byte-exact: the SHA-256 of the output text equals
    `outputSha256`, and the raw UTF-16 length equals `rawOutputLength`. Runs
    use a root of the generation root's length.
  - Engine-specific JSON parser text (JavaScriptCore) is compared by its
    stable prefix (contracts §6.2 rule 3); everything around it is byte-exact.
  - The 20 intentional divergences each have a comparator test. It proves
    that only the approved fix differs:
    - D4, 2 vectors: the legacy read keeps the exact spelling of unknown
      numbers (`12345678901234567890`, `1.0`).
    - D6, 14 vectors: plans and sidecars are sorted by UTF-16 code units, not
      by ICU/readdir order. Lists are compared as sets, and
      `doctor-limit2` pages in the new order.
    - D1, 2 vectors: doctor reports a journal-only plan with its
      interrupted-state hash.
    - D5, 1 vector: validate reports the ambiguous backslash spec path.
    - Markdown, 1 vector: the reference's JavaScript TypeError when rendering
      a raw legacy document cannot occur, because Go renders only normalized
      documents.
  - Deferred items:
    - `tools/*--compact-preview` (2): compaction is stage C.
    - The input vectors for the 7 mutating tools (39).
- Read-only gate: every corpus, CLI and benchmark run fingerprints the whole
  root before and after (bytes, mode, mtime, file set) and fails on any
  change. `TestConcurrentReads` runs all six operations concurrently under
  `-race` with the same check.
- Schema agreement:
  - All 2020-12 schemas compile.
  - Every stored fixture artifact validates or is rejected as expected.
  - The tool schemas match the native parser's accept/reject on all 55 input
    vectors.
- R01 budget sweep: 32 budgets from 4096 to 64000, page limits 1/7/20/100,
  four fixtures, paged to exhaustion. Every page fits and the cursors cover
  every item.
- Fuzzing (20–30 s each): `FuzzParse`, `FuzzQuote`, `FuzzManifestJSON`,
  `FuzzCursorDecode`. The fuzzer found one real bug, now fixed and kept as a
  seed: Go's base64 decoder skipped CR/LF, so distinct cursor tokens could
  alias one position.
- Real plan (the owner's live 13-phase, 38-step, 3-spec draft; a temporary
  copy only, never committed to testdata):
  - All read-only CLI operations were run: list, read, read `--no-markdown`,
    inspect, validate, resume at 4096/12000/64000, and doctor with and
    without an id.
  - Zero byte, mtime or file-set changes.
  - An independent Python implementation of contracts §3 agrees with Go's
    `planHash`/`stateHash` on every operation.
  - The checkpoint in that plan was written by the TypeScript tool, and its
    recorded `planHash` equals Go's `planHash`, so freshness is `fresh` in both.
  - Resume fits every budget (3992/4096, 11458/12000, 35371/64000).
  - Executing the TS oracle for a side-by-side run was blocked by the
    session's permission policy and was not attempted another way.
- Benchmarks: [baseline.md](baseline.md) § Go stage B results.

### Approved D-fixes: disposition in stage B

| # | Stage B | Notes |
| --- | --- | --- |
| D1 | Partly | Doctor adds a plan entry for a journal-only plan, `{id, valid:false, issues, stateHash, recoveryRequired:true}`. This uses only existing plan-entry fields; the hash is `workplan-state-v1` with the primary JSON recorded as missing, plus the journal's targets and the id's sidecars. `read` is unchanged, because returning a packet instead of `Workplan file not found` would change the operation's design. `update {recovery}` on a journal-only plan is stage C. |
| D2 | N/A | The six read-only tools have no nested input objects. They reject unknown keys at every level. |
| D3 | Done at stage A | `schema/v1/tools` has the fixes; `internal/schematest` proves agreement. |
| D4 | Read side done | Reads preserve unknown members' raw bytes and number spelling. Duplicate member names are detected with field paths (`model.Plan.Duplicates`). The plan stays readable with last-wins values, as in the reference. Refusing mutations of such plans is stage C. |
| D5 | Read side done | The manifest keeps the `\` → `/` conversion for hash parity. `validate` reports the ambiguous path. Refusing new `\` links is stage C. |
| D6 | Done | UTF-16 order for plans and sidecars. |
| D7 | Stage C | Update-only. `inspect` keeps the reference refusal text. |
| D8 | Done (index) | `index.MarkerIndex` flags colliding markers as ambiguous. No stage B operation retrieves sections, so outputs are unchanged. |
| D9 | Done | A `read` result above 64 MiB fails with `unsupported_capability` and points to `includeMarkdown=false`, `inspect` or `resume`. |
| D10 | Stage C | Compaction. |
| D11 | Done | Contracts §6.2 prefix rule. The Go parser emits its own detail. |
| D12 | Stage C | Reset. |

### Open issues for stage C (and owner decisions)

1. The resume list-cap cut-over between `maxChars` 4097 and 11999 is not
   pinned by the corpus; Go uses 8192 (contracts §10). A vector at, say,
   6000 and 9000 would settle it.
2. Resume returns an error when even the smallest level cannot fit, for
   example with machine ids thousands of characters long. Machine ids are
   never truncated. A policy is needed: bound id length on write, or define
   an id-omission form.
3. D1 recovery: `update {recovery}` must accept a journal-only plan and
   validate the journal before authorization. Its `expectedHash` must be
   the doctor hash defined above.
4. Duplicate-key and backslash-path refusals (D4, D5) belong in the
   mutation preparation path.
5. The 39 mutating-tool input vectors and the 70 mutation vectors.
   Compact-preview token parity needs the token preimage, which the corpus
   does not expose. Compare the token as an opaque value bound to its inputs,
   or add vectors.
6. Before writes, decide the case-insensitive filesystem policy (spec 01 §4).
   Reads inherit APFS case folding (contracts §10).
7. `schema/index.json` still labels the schemas "PROPOSED — frozen for
   review". The owner approved them. Flip the label when committing if
   desired; stage B did not edit schema files.
8. Go performance (see baseline): Go is faster than the reference at 100 KiB
   and 1 MiB, and in every cold/process measurement. At 10 MiB, Go's warm
   in-process `inspect`/`validate`/`resume` are about 1.2–1.6× slower than
   bun's (≈36–41 ms against 25–29 ms). Stage F should set targets from this
   table before optimizing further. Profiling shows large-buffer page
   faulting and the parse as the dominant costs.

## Resume after maintenance

1. Read this file, `docs/contracts.md` and the specs. No workplan plugin is
   needed.
2. Inspect `git status`. Stage A and the specifications are committed
   (`fdadfd4`). Stage B exists only in the working tree until the owner
   commits it. Preserve any user edits.
3. If the oracle files change, the corpus is stale. Compare their sha256 against
   `testdata/MANIFEST.json` → `oracle.files`.

The Go toolchain observed is 1.27.1 darwin/arm64. Build and test commands are
in the [README](../README.md).
