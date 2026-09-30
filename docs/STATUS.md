# Maintenance-safe handoff — 2026-09-30

## Authorization

The owner has authorized implementation of stages A–D (spec 05 §2).
Stage C owner decisions (contracts §9): D1 recovery accepted; D10 changed
to a root-independent preview token. Stages E–G
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

## Stage B — read-only Go core: DONE (committed `7a899e5`)

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

### Open issues from stage B (resolved at stage C; see contracts §10a)

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

## Stage C — transactional Go core: DONE (uncommitted, for owner review)

Nothing was committed. All stage C changes are in the working tree (new
`internal/storage/`, new engine/cli files, and edits listed by `git status`).

### What exists

| Package / file | Role |
| --- | --- |
| `internal/storage` | Prepared `Intent` (exact read preconditions, write/delete/archive targets with before/after digests and modes, staging, journal, lock-protocol paths, directories; `Digest()` binds an approval); locks with lock-owner-v1 metadata; `Commit` (workspace lock → plan lock → locked recheck → exclusive same-directory staging with fsync → durable journal → atomic publication → directory sync → journal removal → release); explicit recovery reuses the same engine; fault-injection points |
| `internal/engine/mutinput.go` | Strict input parsing for the 7 mutating tools on both surfaces, including every `x-shiori-rules` refinement; unknown keys reject at every level (D2) |
| `internal/engine/{create,update,patch,reset,checkpoint,compact,recovery}.go` | Preparation for each writer (no filesystem side effects); `Execute` = authorize exactly that intent, then commit |
| `internal/engine/mutate.go` | `Authorizer` interface, `Prepared`, workspace linkage (pending-journal claims, Markdown ownership), D4 refusal, stale-hash check |
| `internal/cli/mutate.go` | CLI mutations: prints the prepared intent, TTY `yes` prompt or `--yes`, `--expected-hash`/`--legacy-unhashed`, compact `--apply --preview-token --confirm`, `update --recovery` |

Every writer — create, update (including dependencies and recovery),
patch, reset, checkpoint and compaction apply — goes through `Prepare` →
`Authorizer` → `storage.Commit`; there is no other write path (S01).
Compaction preview is read-only (checked by the corpus read-only gate).

### Evidence (darwin/arm64, Go 1.27.1)

- `go vet ./...` (darwin and `GOOS=linux`): clean. `go test ./...` and
  `go test -race ./...`: all packages pass.
- Vector parity:

  | Category (vector files) | Pass | Approved divergence | Fail |
  | --- | --- | --- | --- |
  | validation/input, mutating tools (39) | 38 | 1 (D2) | 0 |
  | mutations (70) | 63 | 7 | 0 |
  | tools (199, incl. 2 compact-preview) | 178 | 21 (19 stage B + 2 D10) | 0 |
  | resume (87), paging (74) | 161 | 0 | 0 |

  The 7 mutation divergences, each checked by a comparator proving only
  the approved difference: `update-legacy-preserve` and
  `reset-legacy-no-specfiles` (D4 refusal, no write, no prompt);
  `update-duplicate-step-ids` (D7 path text); `reset-markdown-only-generated`
  (D12, zero authorizations); `update-recovery-precreate-resume` (D1:
  recovers with the doctor hash); `create-over-precreate-journal` (same
  refusal, raised before authorization); `compact-apply-valid` (D10: every
  written byte identical after substituting token, archive name and
  removals digest). The two compact-preview vectors are identical except
  the token, digest and lock-protocol auxiliary paths.
- Fault injection (S04/S08): 8 writers (create, create-overwrite, update
  with move, update with dependencies, patch, reset, checkpoint, compaction)
  × 12 fault points (directories, locked, each staging, journal staged,
  journal published, journal synced, each publication, directory sync,
  cleanup) × resume/rollback: 172 runs pass, 20 skip (point not reached).
  Before the journal the artifacts are the old state; after it the error is
  `recovery_required`, doctor and read report it, and both `resume` and
  `rollback` restore exactly the new or the old state with no lock or
  staging file left.
- Separate-process barriers (S02): create/create on one absent destination
  (same plan, and three different plans sharing one Markdown path),
  move/move of three plans onto one destination, four same-plan updates
  with the same expectedHash: exactly one winner each, losers refused, no
  duplicate ownership, no lost update. Pending-journal claims are enforced
  at preparation and under the lock; an unreadable journal fails closed.
- Locks (S11): live, reused-PID, foreign-host, ambiguous and empty owners
  are never reclaimed however old; a dead owner is not reclaimed within the
  grace or while the file is fresh; a proven-dead owner past the grace is
  reclaimed (also with a real dead PID); release never unlinks a replacement
  owner's lock; reclaim verifies the nonce; waiting honours cancellation.
- Cancellation/denial (S03): for all 8 writers, preparation alone, denial,
  cancellation before authorization and a late approval after cancellation
  leave the root byte-, mode- and mtime-identical; prepared intents are
  single-use. Cancellation after the durable journal stops publication and
  reports an uncertain, recovery-required outcome.
- Recovery validation (S05–S07): foreign plan/arbitrary/escaping targets,
  workplanId mismatch, duplicate targets, operation-kind mismatch, unknown
  operation, invented Markdown link, deleted Markdown, overwritten or
  foreign archive, move without its new Markdown, move overwriting an
  existing file, move keeping its old Markdown, and third-state edits are
  all rejected before authorization in both modes, with nothing written.
- C01/D4 positive case: an unrelated update keeps unknown members with
  exact number spelling (`12345678901234567890`, `1.0`, `1e+21`) and leaves
  an absent legacy `specFiles` absent. D5 refusals and symlink refusal
  tested.
- Real-plan comparison (owner's 13-phase plan; two fresh `cp -Rp` copies of
  the read-only fixture per operation; the reference run as a black box on
  one, Shiori on the other, both with the same frozen clock):
  - Byte-identical output and resulting files: list, read, read without
    Markdown, read of one step, inspect, validate, doctor, doctor with id,
    update (step status change), update (appended note), patch (one
    localized Markdown insertion), checkpoint.
  - Resume at 4096/12000/64000: initially different; this found three
    stage B bugs (overflow flag, recentValidation danger class, summary
    truncation order), now fixed and byte-identical.
  - Compact preview: identical except the D10 token/digest and the
    lock-protocol auxiliary paths (after one fix: preserved Markdown is not
    a compaction target).
  - The fixture's bytes were unchanged afterwards; all temporary copies and
    captured outputs were deleted. Nothing from it is in the repository.
- Stage B fixes found during stage C: `.MD` plan files are rejected (as in
  the reference); dependency validation texts; resume caps (contracts §10a).

### D-fixes: disposition after stage C

| # | Stage C |
| --- | --- |
| D1 | Done: journal-only plans are recoverable with the doctor hash as expectedHash; create over a pre-create journal is refused before authorization |
| D2 | Done: unknown nested input keys reject with their path (`phases.0.steps.0: Unrecognized key: "extra"`) |
| D4 | Done: plans with repeated member names are refused by every writer; unknown members keep raw bytes and number spelling on writes |
| D5 | Done: new links containing `\` are refused with a field path |
| D7 | Done: unrenderable ids are reported as `phases.<i>.id: Must not be empty` (or the step path) by update and reset |
| D10 | Done (owner change): root-independent token and archive name |
| D12 | Done: unchanged Markdown-only reset (and any byte-identical mutation) prepares no intent and asks for no authorization |

### Remaining issues for stage D (and owner decisions)

1. The native OpenCode authorizer (actual-host permission requests for the
   intent's exact resources, identity/signal binding, P01–P07) and the
   `shiori serve --stdio` protocol are stage D. The intent's `Resources()`
   and `Digest()` are the inputs it needs.
2. Resume display-cap selection for plans with very many truncated strings
   (contracts §10a item 1) differs from the reference on some budgets; the
   packet invariants hold. Needs an owner decision or stage F work.
3. The compaction removals digest and token are Shiori-defined (D10 and
   §10a item 5); a token from the TypeScript engine is not accepted by Go
   and vice versa.
4. Mixed TypeScript/Go writers on one root are not proven interoperable
   (lock auxiliary paths differ; journals are format-compatible). Stage E
   must select one writer per root.
5. Linux/amd64 write validation has not been run (only `GOOS=linux go vet`);
   contracts §5.6 requires the same suites on Linux before claiming writes
   there.
6. `schema/index.json` still says "PROPOSED — frozen for review" (unchanged).

## Resume after maintenance

1. Read this file, `docs/contracts.md` and the specs. No workplan plugin is
   needed.
2. Inspect `git status`. Stages A and B are committed (`fdadfd4`, `7a899e5`).
   Stage C exists only in the working tree until the owner commits it.
   Preserve any user edits.
3. If the oracle files change, the corpus is stale. Compare their sha256 against
   `testdata/MANIFEST.json` → `oracle.files`.

The Go toolchain observed is 1.27.1 darwin/arm64. Build and test commands are
in the [README](../README.md).
