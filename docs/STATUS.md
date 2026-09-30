# Maintenance-safe handoff — 2026-09-30

## Resume point (paused after D.4.1)

The owner paused after stage D.4.1. Read this section first; the stage
sections below keep the full record.

**State.** Every stage is done; the owner commits each one after review.

| Stage | Status | Commit |
| --- | --- | --- |
| Specs 01–05 | done | `c78a177` |
| A contract and fixture freeze | done | `fdadfd4` |
| B read-only Go core and CLI | done | `7a899e5` |
| C transactional core | done | `9f66518` |
| D stdio protocol and OpenCode adapter | done | `4044539` |
| Spec 06 extensions (X1–X10, P2–P6; accepted X2, X3, X6, P2, P3, P6) | docs | `ca73894`, `6608561`, `7fe8915`, `44d5bb7` |
| D.1 readable resume, filtered read, drift, gate | done | `5c94cdb` |
| D.2 dependency graph drives work (X6) | done | `e47e667` |
| D.3 safe reset, corrupt-plan repair, host degradation | done | `83a3414` |
| D.3.1 step gate, repair archive, resume diagnostics | done | `df74f35` (pushed) |
| D.4 compaction advisor (P2) and note rollover (P3) | done | `02d2333` (pushed) |
| D.4.1 expectedHash guidance (error text, tool descriptions) | done | (owner adds id) |

**Live.** hoshi-opencode2 (`~/.config/opencode`, remote
`git@github.com:hoshinoht/hoshi-opencode2.git`) runs Shiori as its workplan
tools: git submodule `vendor/shiori` pinned to `02d2333` (D.4), plugin
entry `./vendor/shiori/adapter/opencode` with option `bin:
{env:HOME}/.config/opencode/vendor/shiori/shiori` (a locally built
binary, not tracked). The reference plugin `./packages/workplan-tools` is
the rollback (swap back once `workplan_doctor` shows no pending journal;
never register both). After each future stage: bump the submodule,
rebuild `vendor/shiori/shiori`, run hoshi-opencode2 `bun test` and restart
OpenCode. Nothing in
`~/.config/opencode` is changed by Shiori stages.

**Queue, in order** (each needs its own owner approval):

1. **E1 — X2 evidence ledger.** Start with a tool-surface proposal for
   owner approval (spec 06 §1 forbids new or renamed `workplan_*` tools
   without a decision: e.g. evidence through `workplan_update`/
   `workplan_checkpoint` inputs vs a new tool), then the
   `<id>.evidence.json` v1 sidecar, git tree binding and staleness.
2. **E2 — X3 worktree lanes** (needs X2). Proposal first (path-claim
   trie, lane state machine, baseline fingerprint, merge train; spec 04).
3. **Measured performance stage** (all still to-review in spec 06): X8
   warm snapshot cache in `serve`, X9 derived plan index (open choice:
   SQLite vs a small custom format, decide by measurement), X10 structural
   index and slice parsing. Measure first (spec 03), record targets.

**Open owner decisions.**

- D.4 review items (below, "Owner decisions to review (D.4)"), chiefly the
  decision-record rule of note rollover and the resume budget cost of the
  advice.
- Still to-review in spec 06: X1, X4, X5, X7, X8, X9, X10, P4, P5.
- Stage E–G items beyond what the owner already enabled (release
  artifacts, cross-platform evidence, migration automation) remain
  unauthorized; so are commits, pushes and installs by an agent.

**Fixtures and evidence.**

- In the repository: the frozen oracle corpus and fixtures (`testdata/`,
  `testdata/MANIFEST.json` pins the oracle hashes), the stage pins
  (`testdata/d1`, `d2`, `d3`, `d3_1`, `d4`, `d4_1/expectations.json`), perf
  fixtures (`testdata/perf/*.tar.gz`, unpacked by the benchmarks into
  `$TMPDIR/shiori-perf-<go version>/`).
- Owner-plan evidence (never copied into the repository): read-only
  copies `kanade-fixture` (13-phase roadmap, `kanade-v5-roadmap`) and
  `kanade-hardening` (257 KB plan, `kanade-v5-beta-hardening`) in the
  session scratchpad `/private/tmp/claude-501/-Users-cantabile--config-opencode/cd0126d5-9456-499c-8c8d-47d08ad247e9/scratchpad/`
  (temporary; recreate with `cp -Rp` from the originals under
  `~/projects/personal/maplestory/kanade-bot/.opencode/workplan/` and its
  `docs/v5/` specs, never modifying the originals). Always work on a
  further `cp -Rp` copy with `chmod -R u+w`.
- Reference TypeScript plugin (for `adapter/opencode/scripts/snapshot-registration.ts`):
  `~/.config/opencode/packages/workplan-tools/src/core`.

## Authorization

The owner has authorized implementation of stages A–D (spec 05 §2).
Stage C owner decisions (contracts §9): D1 recovery accepted; D10 changed
to a root-independent preview token. Stage D owner decision (contracts §9
item 7): the resume display-cap residual is accepted as a divergence
(since superseded by D.1). D.1 owner decision (contracts §11, 2026-09-30):
five design changes to behaviour inherited from the reference (resume
budgeting, filtered read slice, Markdown drift warning, status gate and
patch issue list, doctor stray artifacts) are approved and implemented.
D.2 owner decision (contracts §12, 2026-09-30): the dependency graph drives
work (resume readiness and ranking, warn-only order checks, cancelled
prerequisites, phase replacement against the graph, dependency write and
view fixes, critical path X6); order checks never refuse.
D.3 owner decision (contracts §13, 2026-09-30): safety and robustness fixes
(status-only draft reset and a previewed, archived wipe; repair of
unreadable plans; stale Markdown copies; missing step markers; stale
checkpoint detail, readable ids, spec existence, list issues, note limit,
file modes; SIGKILL recovery; unverified-host degradation).
D.3.1 owner decision (contracts §14, 2026-09-30): step status gate, repair
archive and recovered planFile for unparseable plans with a repair hint on
writers, compact stale diagnostic and critical path in resume, wiped-plan
doctor note, finding rendering `title (status)` and whole-second
timestamps (both renderings are detected as generated).
D.4 owner decision (contracts §15, 2026-09-30): the compaction advisor
(P2) in resume and doctor and note rollover (P3) as a compaction selection
mode; advice only, the compaction flow is unchanged.
Stages E–G are still unauthorized (enabling the adapter in a real OpenCode
configuration is stage E, decided separately). So are commits, pushes, automatic migration,
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

## Stage C — transactional Go core: DONE (committed `9f66518`)

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

### Remaining issues after stage C (disposition at stage D)

1. Native authorizer and `shiori serve --stdio`: done at stage D (below).
2. Resume display-cap selection for plans with very many truncated strings
   (contracts §10a item 1): **accepted by the owner as a divergence**
   (contracts §9 item 7); resume behaviour is unchanged.
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

## Stage D — native adapter: DONE (committed `4044539`)

Stage D added: new
`internal/protocol/`, `internal/cli/serve.go`, `internal/engine/errclass.go`
(the error-class mapping moved from the CLI so the CLI and the protocol share
it), `adapter/opencode/`, and small edits (doctor runtime-fact injection,
`serve` in the CLI usage, protocol-envelope schema additions). The owner's
`~/.config/opencode` was not modified; registration there is stage E.

### Protocol surface (`shiori serve --stdio`)

| Aspect | Behaviour |
| --- | --- |
| Transport | JSON lines on stdin/stdout of a child started by the adapter; stdout carries frames only; stderr one-line diagnostics with redaction (issued capabilities, `Basic`/`Bearer` credentials, `password=`/`token=`-style values). No listener, no daemon |
| Framing | Request frames above 16 MiB are rejected before they are buffered or decoded; nesting above 128, duplicate member names anywhere in a frame, unknown envelope or `hostContext` keys, an unknown operation, a bad `requestId`, a request before the handshake or a second handshake are `invalid_frame`/`unsupported_protocol`; the error is answered (with requestId `_` when it cannot be correlated) and the connection closes, expiring every prepared intent. Unknown keys inside `input` are ordinary `invalid_input` errors (native surface, every nesting level) and the connection continues. Response frames above 64 MiB become `unsupported_capability` |
| Handshake | `shiori.handshake {protocolVersions}` → core/protocol/contract versions, the 13 tool operations plus `shiori.commit`/`shiori.discard`, hash algorithms, platform and `writeSupported` (darwin/arm64, linux/amd64 per contracts §5.6), durability facts observed by a probe in a private temporary directory (never the project), limits and idle timeout |
| Requests | Multiplexed by `requestId`; each runs concurrently. `hostContext` (mode `native`, canonical root, sessionID, agent, messageID, callID; `runtimeFacts` only for `workplan_doctor`) is required; the connection binds to the first canonical root and rejects symlinked or different roots |
| Results | `result.text` is the exact native tool result text (the adapter returns it unchanged; patch is `{output, metadata}`); `hashes` carries planHash/stateHash when present |
| Prepare → commit | A mutating tool returns `prepared` {intentId, intentDigest, 64-hex capability, operation, tool, workplanId, canonicalRoot, expectedStateHash, resources, targets} and touches nothing. `shiori.commit {intentId, intentDigest, capability}` commits that unchanged intent only if the digest, capability and the trusted invocation identity match; any mismatch is `permission_rejected` and burns the intent. Single use; `shiori.discard` releases it. A mutation that would change no byte (D12) returns its result without an intent |
| Expiry | Cancel of the preparing request, stdin EOF/transport loss, SIGTERM, process exit, a mismatched commit, or a commit (used). State changes are rejected by the locked recheck (`stale_state`/`external_edit_conflict`). Nothing is persisted, so a reconnect cannot replay |
| Cancellation | A cancel frame answers an in-flight read or prepare with `cancelled` at once; a cancelled commit reports its own truthful outcome: `cancelled` before the journal, `outcome_uncertain` with `recoveryJournal` and `retrieval` after it |
| Errors | `{class, message, issues?, currentStateHash?, recoveryJournal?, retrieval?}` with the envelope classes; `message` is the reference text |
| Lifecycle | Exits on stdin EOF, SIGTERM/SIGINT (in-flight commits are cancelled and reach a truthful outcome first), a fatal frame error, or after 10 minutes with no live request and no prepared intent (`--idle-timeout`) |

Schema: `schema/v1/protocol-envelope-v1.schema.json` gained only optional,
documented stage D members (`hostContext.runtimeFacts`,
`prepared.capability/tool/canonicalRoot`, `limits.maxResponseBytes/idleTimeoutMs`,
`durability.observedAt`, `toolResult`, `discardInput`) and requires the commit
capability. `internal/schematest` validates every frame of a real session
against it.

### Adapter design (`adapter/opencode/`)

- **Package**: `index.ts` → `src/plugin.ts`; plugin id `workplan-tools` (the
  reference id, so only one of the two can be active); no dependencies, no
  `node_modules` — runtime imports are `node:` built-ins only, and
  `@opencode/plugin` 2.0.20 is a type-only peer. The runtime smoke confirmed
  OpenCode loads it from source without installing anything. OpenCode loads a
  package plugin from its root entry file, hence `index.ts` at the package
  root. A single-file bundle builds with `bun build` (README).
- **Registration**: the thirteen tools in the reference order with the
  reference descriptions and input JSON Schemas (`src/registration.json`,
  generated from the reference plugin by `scripts/snapshot-registration.ts`),
  `options: {codemode: true}`. A test checks the property sets against
  `schema/v1/tools`. Input validation and its error text stay in Go.
- **Trust**: sessionID/agent/messageID/tool-call ID and the AbortSignal come
  only from the native `ToolContext`; the root is `realpath` of the plugin
  location. Model input carrying `workspaceRoot`, identity or approval fields
  is rejected by the core's strict native parser. The role matrix (spec 02
  §5) runs first with the reference messages.
- **Host authorization**: the reference permission bridge, ported to a
  dependency-free public client (`src/host-client.ts`: service discovery by
  registration file only, `/api/info`, the SSE event stream, session lookup,
  `permission.create`, the bridge's own RPC). Fresh HMAC instance proof via
  the bridge RPC plus the correlated event-stream echo, session-location
  check, `edit` request for the exact canonical resources (every write,
  delete, lock, lock-protocol, staging, archive and journal path of the
  intent, plus their parent directories inside the project — the same class
  of resources the reference requested), filtered ask/reply waiters, receipt
  verification. Deny, rejection, ambiguity, lost stream, unknown results and
  abort fail closed. No reply/rule API is called (checked statically).
- **Scope check** before asking: the prepared intent must be bound to this
  root and tool; every write-class resource must be an exact absolute path
  under `<root>/.opencode`, every read path inside the project.
- **Core client** (`src/core-client.ts`): binary from plugin option `bin`,
  else `SHIORI_BIN`; absolute path required, never `PATH`. Lazy spawn on the
  first call, handshake validated (protocol 1, contract v1, all operations,
  capability facts) or the child is terminated. Frames above the negotiated
  limit or nested deeper than 128 are refused locally. AbortSignal → cancel
  frame; an abort after preparation also cancels the preparing request so the
  intent expires. Transport loss fails every in-flight request (`cancelled`,
  or `outcome_uncertain` for a commit with a pointer to `workplan_doctor`); the
  next call respawns and handshakes. Unload: cancel in-flight requests, close
  stdin, 2 s, SIGTERM to the owned PID, 2 s, SIGKILL.
- **Doctor**: the adapter gathers the same host facts as the reference
  (effective tools, plugin state, agent/session rules, bridge diagnostics)
  and passes them as `hostContext.runtimeFacts`; Go sanitizes them exactly as
  the reference does.

### P01–P07 coverage

| ID | Where proven |
| --- | --- |
| P01 | `plugin.test.ts`: forged `workspaceRoot`/`sessionID`/`agent`/`messageID`/`callID`/`approved` rejected; role checks use the trusted agent; the permission request carries the ToolContext session/agent/message/call; a write without an AbortSignal is refused with no change. `core-client.test.ts`: the same through the core. Runtime smoke: `err-root-override` on a real host |
| P02 | `permission-bridge.test.ts` (ported reference cases): proof precedes any request; altered RPC proof, wrong-location echo, missing echo, closed stream and missing RPC fail before any `permission.create`. Runtime smoke: every mutation passed the real host proof (`host verified, event stream ready` in doctor) |
| P03 | Runtime smoke on OpenCode 2.0.20: absolute agent deny → denied; absolute session deny → denied; a relative session deny does not match the absolute resources and the host default (allow) applies — disclosed, not translated. Aliased roots/resources and nested sessions fail closed (unit) |
| P04 | Runtime smoke: allow (default policy), ask → genuine user reply once (7 mutations), ask → reject (no change), deny, session override, cancel then late "once" reply (no change, tool never continued). Unit: the same plus unrelated replies |
| P05 | Unit: unrelated proof events, asks with other source/marker/resources/action/location, and unrelated replies neither authorize nor consume the request; a correlated ask with a different request ID is rejected |
| P06 | Static test: no reply/rule/saved-permission/`Service.ensure` call and no `@opencode/client` import in `src/`; core: single-use intents, replay after reconnect rejected, burned intents; no permission grant is cached or journaled |
| P07 | Missing/relative/non-executable binary and unsupported handshakes (contract, protocol, operations, facts) fail with actionable messages; unverified host versions register nothing; child crash mid-authorization and mid-commit fail closed without replay; lost event stream fails closed; malformed core frames close the connection |

### Evidence (darwin/arm64, Go 1.27.1, bun 1.4.0, OpenCode 2.0.20)

- `go vet ./...` (darwin and `GOOS=linux`): clean. `go test -race -count=1`
  per package with timeouts: every package passes (protocol ×3 under race).
- `bun test` in `adapter/opencode`: 47 pass, 1 skipped (the opt-in runtime
  smoke). Typecheck against the real `@opencode/plugin` 2.0.20 declarations
  (read-only, via a scratch tsconfig): clean.
- `CGO_ENABLED=0 go build -trimpath ./cmd/shiori`: builds.
- Runtime smoke (`test/runtime-smoke.test.ts`, opt-in): a private
  `opencode serve --service` (OpenCode 2.0.20) with its own HOME and
  XDG_CONFIG/DATA/STATE/CACHE directories and TMPDIR in a scratch directory,
  its own loopback port, a project that is a copy of the pristine owner-plan
  fixture (13 phases, 38 steps, 3 specs; kept outside the repository), the
  adapter package copied from the working tree, a test-only relay plugin, and
  no model call. Results: all 13 tools registered and effective; 13 read
  calls (list, read ±Markdown, inspect ±limit, validate, resume
  4096/12000/64000, doctor ±id, compact preview both ways) changed no file
  and asked nothing; 6 error cases returned the reference messages; one
  authorized mutation of the real plan (`workplan_update` appendNotes) went
  through the real host proof and a real `permission.asked` event with 22
  exact canonical resources, was granted by a user reply through the public
  API, and changed exactly `<plan>.json`; create, update, patch, checkpoint,
  compact apply and reset each went through a real ask and grant; a user
  rejection, an absolute agent deny and an absolute session deny changed
  nothing; a relative session deny did not match (default allow applied;
  disclosed); a cancel followed by a late "once" reply changed nothing and
  the tool never continued; doctor reported `host verified, event stream
  ready`. OpenCode created nothing in the adapter package directory. The
  pristine fixture's tree hash was identical before and after; the owner's
  `~/.config/opencode` git status was identical before and after; only the
  servers the test started were stopped (SIGTERM).
- Parity (reference plugin vs adapter, same scripted inputs on two private
  servers, fresh copies of the same fixture at equal-length roots, no model):
  36 calls covering all 13 tools (every mutating tool through the real host
  permission path):
  - 19 byte-identical after substituting the root: list, read ±Markdown,
    inspect ±limit, validate, resume at 4096/12000/64000 on the owner-plan
    fixture, the 6 error messages, the rejected, denied, session-denied and
    cancelled mutations.
  - 11 identical after also normalizing timestamps and the hashes/tokens
    derived from them (update of the real plan, create, update, patch,
    checkpoint, resume after checkpoint, compact apply, read, reset, the
    relative-deny create, read before cancel).
  - 6 different, all expected: 3 doctor results differ only in the session
    ID and the per-server scratch HOME paths inside host permission rules,
    plus (final doctor) D6 sidecar order; 3 compact previews differ only in
    the lock-protocol auxiliary paths of `writeIntent.resources` (§10a,
    known since stage C).
  - The resulting `.opencode` trees (9 files, including the archive) are
    identical after the same normalization.

### Stage E prerequisites (not done; separately decided)

1. Build the core at a fixed absolute path, e.g.
   `CGO_ENABLED=0 go build -trimpath -o /Users/cantabile/projects/personal/shiori/shiori ./cmd/shiori`.
2. With no OpenCode session active and no pending workplan transaction (run
   `workplan_doctor`; resolve any journal first), replace in
   `~/.config/opencode/opencode.json` the entry

   ```jsonc
   { "package": "./packages/workplan-tools" }
   ```

   with

   ```jsonc
   { "package": "/Users/cantabile/projects/personal/shiori/adapter/opencode", "options": { "bin": "/Users/cantabile/projects/personal/shiori/shiori" } }
   ```

   (or keep `options` out and export `SHIORI_BIN` with the same absolute path
   in the environment OpenCode starts in). Never list both entries: they
   register the same tools and the same plugin id.
3. Restart the OpenCode service so the plugin reloads; check with
   `workplan_doctor` that `runtimeFacts.plugin.effective` is true and the
   registrations list the thirteen tools.
4. Rollback: stop admissions (no session running), run `workplan_doctor` and
   resolve any pending journal, restore the original
   `{ "package": "./packages/workplan-tools" }` entry, restart OpenCode. The
   artifacts are format-compatible (V2 plans, checkpoints, dependencies,
   journals); lock auxiliary paths differ, so never run both writers on one
   root at the same time (contracts §10a).
5. Open for the owner before stage E: the items below.

### Remaining issues (stage D)

1. **Registration schemas.** Contracts §5.1.3 says the adapter registers
   `schema/v1/tools` unchanged; the adapter instead registers the reference
   plugin's own generated schemas and descriptions, so the model-facing tool
   surface is byte-identical to today's (design rule). `schema/v1/tools`
   (with the D3 fixes) remains the validation contract in Go; a test keeps the
   property sets equal. Needs owner confirmation.
2. **Read authority for linked files.** Like the reference, only mutations
   ask the host; reads of linked Markdown/specs do not request read
   permission (spec 02 §3 last paragraph).
3. **Go read cancellation.** A cancelled read is answered immediately, but
   the engine's read functions do not observe the context internally; the
   abandoned goroutine finishes its (side-effect-free) work.
4. **Doctor strings from the host.** Go bounds host-supplied strings by
   UTF-16 code units like the reference, but drops a surrogate pair cut at
   the boundary where JavaScript would keep a lone surrogate.
5. **Durability facts** are probed in a private temporary directory, which
   may be a different filesystem from the project; per-commit
   `directorySync` remains the project-filesystem fact.
6. **Linux**: `writeSupported` is true on linux/amd64 per the approved matrix,
   but the stage C/D suites have only run on darwin/arm64 (contracts §5.6).
7. **Host versions**: the adapter allow-lists OpenCode 2.0.19 and 2.0.20
   (contracts §5.6); a host upgrade disables the workplan tools until the
   list is extended after re-verification.
8. **Adapter layout.** Contracts §5.3 names a single committed
   `adapter/shiori-opencode.js`; stage D ships the TypeScript package
   `adapter/opencode/` (loaded from source by OpenCode) and documents a
   `bun build` single-file bundle instead of committing generated code.
   `adapter/opencode/tsconfig.json` is for editor/typecheck use only; the
   typecheck in this stage mapped `@opencode/plugin` to the read-only
   2.0.20 declarations through a scratch tsconfig.
9. The compact-preview `writeIntent.resources` lock-protocol paths differ
   from the reference (known since stage C, §10a); the adapter's permission
   request therefore names Shiori's lock auxiliary paths.

## Stage D.1 — approved design changes: DONE (committed `5c94cdb`)

Base `4044539`. Nothing was committed. The pinned copy of `4044539` that
the owner's OpenCode uses and everything under `~/.config/opencode` were
not touched. The decisions and exact behaviour are in
[contracts §11](contracts.md#11-approved-design-changes-d1-approved-2026-09-30).

### Changes

| Item | What changed | Where |
| --- | --- | --- |
| A resume | New degradation order around a target page of min(limit, 8/4/2) items (budget ≥12000 / ≥6000 / below): while the target fits, text shrinks first (uncapped → prose 240 / titles 80 → floor 120/80; current work keeps ≥512); below the target, fewer items at the floor, then current work at the floor, then fewer pinned-list entries; only then emergency caps below the minimums, then paths, then an empty page. Paths, references and the instruction are protected; kinds and severities are never truncated. (Rebalanced after coordinator review: the first version dropped items before any shortening and returned 2/44 at 12000) | `internal/engine/resume.go` (`chooseResume`, `resumeTargetPage`, `textClass`) |
| B filtered read | `phaseId`/`stepId` return a slice: header without phases (and without findings/notes unless `includeNotes`), the selection, `plan` without content unless `includeMarkdown:true`, dependency entries touching the selection, hashes, and a `slice` descriptor. New input `includeNotes` (both surfaces; CLI `--notes`, plus `--markdown`) | `internal/engine/read.go`, `engine.go` (`sliceValue`), `input.go`, `internal/cli/cli.go`, `schema/v1/tools/workplan_read.input.schema.json`, `adapter/opencode/src/registration.json` (+ `scripts/snapshot-registration.ts`) |
| C drift | `warnings` (only when nonempty) in validate, doctor plan entries and patch validation when the linked Markdown is not the generated rendering; `valid` unchanged | `internal/engine/validate.go` (`validationWarnings`), `doctor.go`, `patch.go`, CLI human output |
| D gate | create/update refuse `in_progress`/`review`/`completed` while the structure rules fail on the resulting plan (`StatusGateError`, class `invalid_structure`, field-path `issues` in the CLI `--json` and protocol errors, before authorization); patch `validate` returns `validation.issues` | `internal/engine/planedit.go`, `create.go`, `update.go`, `errclass.go`, `patch.go`, `internal/protocol/server.go`, `internal/cli/cli.go` |
| F strays | doctor `strayArtifacts`/`strayArtifactCount`/`omittedStrayArtifacts`/`warnings` (only when present) for orphaned checkpoint/dependency sidecars and unclassified root files (e.g. `*.patch`, unlinked `*.md`); suggests `archive/`, never moves or deletes | `internal/engine/doctor.go`, `list.go` (`dirListing.other`) |

### Vector expectations

The oracle corpus is unchanged. `testdata/d1/expectations.json` pins the
new Go output (SHA-256 of the root-normalized text and UTF-16 length) of
every vector that differs on purpose. Each one also passes a D.1
comparator against the unchanged oracle vector
(`internal/engine/d1_vectors_test.go`). A listed vector that becomes
identical to the oracle, or an unlisted vector that diverges, fails.
Re-pin after review with `SHIORI_D1_UPDATE=1 go test ./internal/engine -run TestCorpusParity`.

| Category | Pass (oracle-exact) | Earlier approved divergence | D.1 divergence | Fail |
| --- | --- | --- | --- | --- |
| tools (199) | 168 | 10 | 21 (3 B filtered read, 18 C/F additive; 11 of these also carry D1/D6 and are checked by those comparators after the D.1 members are removed) | 0 |
| resume (87) | 83 | 0 | 4 (A) | 0 |
| paging (74) | 43 | 0 | 31 (A) | 0 |
| mutations (70) | 62 | 7 | 1 (`patch-validate`, D/C) | 0 |

The A comparator checks the following against the oracle packet. Every
machine field matches: ids, hashes, counts, totals, freshness, retrieval.
Every display string equals the oracle's or is a truncation of the same
text. Page items are the same items in order. Page arithmetic and
`omittedDangerCounts` are consistent, and every page progresses. With
more than one item, no string is shortened below the minimums and no
path is truncated.

The B comparator checks that the slice equals the oracle document minus
phases/findings/notes, the same selection, no Markdown, the oracle
dependency view restricted to the selection, and a correct descriptor.

The C/F comparator checks that removing the additive members yields the
oracle bytes, or the earlier D1/D6 comparator result.

### New tests

- `d1_test.go`:
  - `TestD1ResumeReadability`: a synthetic 13-phase/38-step roadmap at
    64000/20000/12000/8000/6000, paged to the end. Minimums hold;
    ≥20000 truncates nothing; current work keeps ≥512 at 12000; a page
    below the target only has floor-level item prose; the default budget
    returns at least 4 items. `TestD1ResumeTargetPage` pins the targets.
  - `TestD1ResumeMinimumBudget`
  - `TestD1FilteredRead`
  - `TestD1MarkdownDrift`
  - `TestD1StatusGate`: create and update, allowed statuses, same-call
    completion, patch issue list.
  - `TestD1DoctorStrays`
- `TestResumeBudgetSweep`: extended with 6000, the synthetic roadmap and
  the readability/protection/target-order invariants.
- `TestResumeCapsBetweenCorpusBudgets`: now checks the unchanged caps
  directly and on resume-stress.
- CLI: `TestReadSliceFlags`, `TestStatusGateCLI`; `--json` vector checks
  use the D.1 pins.
- Protocol: `TestStatusGateAndPatchValidateIssues`.
- Adapter: registration differs from the reference only by the `d1`
  additions (`referenceSha256`).

### Evidence (darwin/arm64, Go 1.27.1, bun 1.4.0)

- `gofmt -l`: clean. `go vet ./...` (darwin and `GOOS=linux`): clean.
- `go test -race -count=1` per package with timeouts: all pass
  (engine 78 s). The protocol package also passes `-race -count=3`.
- `bun test` in `adapter/opencode`: 48 pass, 1 skip (opt-in runtime smoke).
- `CGO_ENABLED=0 go build -trimpath ./cmd/shiori`: builds.
- Resume benchmark (`BenchmarkOps/*/resume`): 0.7 / 4.7 / 39 ms at
  100 KiB / 1 MiB / 10 MiB, in line with the stage B–D figures.

Owner-plan evidence ran on fresh `cp -Rp` copies of the read-only fixture;
nothing from it is in the repository. Before is the `4044539` binary,
after is the working tree. The fixture tree hash (bytes, modes, mtimes) was
identical before and after, and the read-only operations left the copy
unchanged.

| Resume | Before (`4044539`) | After (D.1) |
| --- | --- | --- |
| 12000 | 11470 units, 20/44 items, 144 truncated fields; summary, next action, titles and actions all cut to 32 units; path truncated | 11802 units, 5/44 items, 36 truncated fields; summary 512 (current-work cap), next action 236 and current action 414 (full), item titles complete (median 51), item actions 120; all 8 constraints; paths intact |
| 6000 | 5952 units, 17 items, every display string cut to **1** unit | 5814 units, 1 item; summary, next action and actions at 120, titles complete; 2 of 8 constraints shown (`omittedDangerCounts` 6), paths intact |
| 4096 | 4020 units, 7 items, strings cut to 1 unit | 3843 units, 1 item, emergency cap 32 with every cut listed; paths intact |

The 8-item target at 12000 is not reached on this plan: its pinned header
(8 constraints, 6 scope, 5 non-goals, 8 relevant files, 3 guardrails, 3
recent validations, current work at 512) leaves room for 5 items at the
120 floor. A measured variant that also lets the pinned lists shrink to
hold the target (before the page shrinks) gives 8 items at 12000 (summary
120, 4 of 8 constraints) and 4 at 8000, but still 1 at 6000; it was not
adopted because it trades safety-list breadth and current-work text for
page size, which the review did not ask for.

| Read of the 38-step plan | Before | After |
| --- | --- | --- |
| unfiltered | 227236 bytes | identical bytes |
| `--phase core-bot` | 192517 bytes (whole document + Markdown) | 13714 bytes (33441 with `--notes`) |
| `--phase core-bot --step context` | 185476 bytes | 6673 bytes |

C, on the unmodified copy: `validate` returns `valid: true` plus the
drift warning for `.opencode/workplan/kanade-v5-roadmap.md`, because the
owner's Markdown is hand-edited. `doctor` repeats the warning on the plan
entry. D, on a scratch copy: an update that sets `in_progress` and adds a
step without validation is refused with
`issues[{path: "phases.6.steps.7.validation"}]`, and `blocked` is
accepted. F, on a scratch copy: with an orphan
`kanade-v5-next.checkpoint.json` and a `rename.patch` added, doctor lists
both with the `archive/` suggestion and changes nothing.

### Remaining issues (D.1)

1. **Page size at small budgets.** The target page is a preference, not a
   guarantee: with a large pinned header the roadmap gets 5 items at 12000
   and 1 at 6000/8000 (the synthetic plan about 4 at 12000). Reaching the
   target there would need shrinking the pinned safety lists first (see
   the variant above). Tiers, minimums and targets are constants in
   `resume.go`.
2. **Markdown default in filtered reads.** A filtered read omits the
   Markdown unless `includeMarkdown` is explicitly true. The registered
   `includeMarkdown` description ("defaults to true") is the reference
   text and was left unchanged, because only the `includeNotes` addition
   was approved. The `includeNotes` description states the filtered
   behaviour.
3. **Status gate scope.** The gate covers the plan status and the spec 01
   §3 structure rules. It does not cover missing linked spec files, which
   `validate` still reports, or phase/step statuses. It applies only when
   the input sets a gated status. An existing invalid in-progress plan can
   still be edited otherwise, so it stays repairable.
4. Mixed writers: the D.1 Go core and the reference TypeScript plugin now
   give different resume, filtered-read and doctor output. Stage E's
   one-writer-per-root rule is unchanged.

## Stage D.2 — dependency graph drives work: DONE (committed `e47e667`)

Base `5c94cdb`. Nothing was committed. The pinned `vendor/shiori` copy
(`5c94cdb`) that the owner's OpenCode uses and everything under
`~/.config/opencode` were not touched. The decisions and exact behaviour are
in [contracts §12](contracts.md#12-approved-design-changes-d2-approved-2026-09-30).
Graph additions appear only with a valid dependency sidecar. Without one,
every output is byte-identical to D.1.

### Changes

| Item | What changed | Where |
| --- | --- | --- |
| G1 readiness | `checkpoint.current` and each `active-work` item: `readiness`, plus `unblocks` (ready) or `blockedBy[{phaseId, stepId, status}]` capped at the pinned-list cap, with `blockedByOmitted` (blocked) | `internal/engine/resume.go` (`readinessView`), `internal/index/graph.go` |
| G2 order warnings | update that sets `in_progress`/`review`/`completed` with unmet prerequisites succeeds with `warnings`; validate, doctor plan entries and patch validation add non-failing `dependencies: Order warning: ...` | `internal/engine/graph.go` (`graphWarnings`, `statusChangeWarnings`), `update.go`, `validate.go` (`validationWarnings` is now an engine method), `doctor.go`, `patch.go` |
| G3 cancelled prerequisites | open dependents of a cancelled step (plan or archived terminal summary) are flagged in validate/doctor/patch validation and in resume `safety.unverifiedWarnings` (placed before the unverified checkpoint lines) and `blockedBy` status; cancelling a step with open dependents warns in the update result | `graph.go`, `resume.go`, `index.Graph.Status` |
| G4 phase replacement | `update {phases}` without `dependencies` re-validates the stored sidecar against the result during preparation; new dangling links are refused before authorization (`Invalid dependency metadata: ...; the phase replacement would leave these dependency links dangling...`) | `internal/engine/update.go` |
| G5 small | empty `dependsOn` refused on write (warning on read); backward links warn (write result, validate, doctor); compaction preview `archivedPrerequisites`; inspect steps `prerequisites`/`dependents` (+ `readiness`/`unblocks`/`slack` for open steps) | `update.go`, `graph.go`, `compact.go`, `inspect.go` |
| G6 critical path (X6) | transitive open-dependent counts; resume ranks ready work by `unblocks` (ties in plan order), then blocked work; `criticalPath{length, estimate?, steps, recommendation}` in inspect and doctor plan entries (≥2 open steps); optional numeric `estimate` step member weights it | `internal/index/graph.go` (`Graph`, `Critical`, `Slack`, `Unblocks`), `inspect.go`, `doctor.go` |
| CLI | human output shows `[ready, unblocks N]` / `[blocked by ...]` and the critical path | `internal/cli/human.go` |

### Vector expectations

The oracle corpus is unchanged. `testdata/d2/expectations.json` pins the 11
vectors whose output gains graph members: 6 resume (`full-valid`,
`list-mixed/b-plan` at 4096/12000/64000), `tools/full-valid/{doctor,
doctor-limit1, full-plan--doctor, full-plan--inspect}` and
`tools/list-mixed/b-plan--inspect`. None of them overlaps a D.1 pin.
`TestCorpusParity` runs every vector twice. The output with the graph
additions turned off (`Engine.noGraph`, test-only) must pass exactly the
check it passed at D.1: the oracle bytes, a D.1 pin or a declared
divergence. The D.2 output must differ from it only by the approved
members, which the comparator restates from the raw fixture JSON:
readiness, `blockedBy` prefix and omitted count, `unblocks`, ranking,
prerequisites/dependents, and a critical path of the restated maximum
length along stored edges. A listed vector that stops differing, or an
unlisted one that starts, fails. Re-pin after review with
`SHIORI_D2_UPDATE=1 go test ./internal/engine -run TestCorpusParity`.

| Category | Pass (oracle-exact) | Earlier approved divergence | D.1 divergence | D.2 divergence | Fail |
| --- | --- | --- | --- | --- | --- |
| tools (199) | 163 | 10 | 21 | 5 (3 doctor critical path, 2 inspect graph view) | 0 |
| resume (87) | 77 | 0 | 4 | 6 (readiness) | 0 |
| paging (74) | 43 | 0 | 31 | 0 | 0 |
| mutations (70) | 62 | 7 | 1 | 0 | 0 |

The CLI `--json` vector checks use the D.2 pins before the D.1 pins.

### New tests

- `internal/index/graph_test.go`: estimate weighting (including ignored
  non-positive estimates), unblocks, slack, readiness through completed and
  cancelled terminal summaries.
- `internal/engine/d2_test.go`, on the D.1 synthetic 13-phase/38-step
  roadmap with a 12-entry cross-phase graph (M1 p2–p5 → M2 p6–p9 → M3
  p10–p12, invented content):
  - `TestD2ResumeReadiness`: exact ranked order, `unblocks` 12/11, `blockedBy`.
  - `TestD2ResumeBudgets`: 6 budgets × 2 limits paged to the end with the
    D.1 invariants and the D.2 order. It logs D.2 against D.2-off page sizes.
  - `TestD2OrderWarnings`: G2 (in_progress, review, completed) on
    update/validate/doctor; `valid` unchanged.
  - `TestD2CancelledPrerequisite`: G3, including cancelled and completed
    terminal summaries.
  - `TestD2PhaseReplacement`: G4 refusals (class `invalid_structure`)
    before authorization (no bytes written) and the accepted cases.
  - `TestD2DependencyWrites`: G5 (empty entry, backward link, inspect view,
    compaction preview differs from D.2-off only by `archivedPrerequisites`).
  - `TestD2CriticalPath`: path, slack, inspect/doctor agreement, no members
    without a sidecar.
- `TestResumeBudgetSweep` now also sweeps the graph roadmap and checks the
  D.2 order on every page (33 budgets × 4 limits × 6 plans).

### Evidence (darwin/arm64, Go 1.27.1, bun 1.4.0)

- `gofmt -l`: clean. `go vet ./...` (darwin and `GOOS=linux`): clean.
- `go test -race -count=1` per package with timeouts: all pass (engine
  99 s).
- `bun test` in `adapter/opencode`: 48 pass, 1 skip (opt-in runtime smoke).
  The tool surface and registration are unchanged.
- `CGO_ENABLED=0 go build -trimpath ./cmd/shiori`: builds.

Budget on the synthetic graph roadmap (first page, D.2 against D.2-off): at
64000/20000 the counts are the same (7 or 20, and 9); at 12000 it is 5
against 6; at 8000/6000/4096 it is 1 against 1. Every page fits and keeps
the D.1 minimums.

Owner-plan evidence ran on fresh `cp -Rp` copies of the read-only fixture;
nothing from it is in the repository. The pristine fixture's tree hash
(paths, modes, mtimes, sizes and file bytes) was identical before and
after. `5c94cdb` (D.1) and the working tree (D.2) were compared on the
same copies:

- **Unchanged without a sidecar.** On an unmodified copy, `resume` (12000,
  6000, 4096), `validate`, `doctor`, `inspect --limit 500` and
  `read --phase core-bot` are byte-identical between D.1 and D.2.
- **12-entry graph** (synthetic, M1 core-bot/core-evidence → M2
  workspace/authoring-ops/member-portal → M3 later-extension → release-ops),
  written with `update --input '{"dependencies":[...]}'`: no warnings.
  Resume page sizes D.1/D.2 are 6/6 at 12000, 1/1 at 6000 and 4096, and
  20/20 at 64000. The current step `core-bot/context` is `ready, unblocks
  11`. At 64000 the ranked page starts with `core-bot/reliability` and
  `core-bot/staging` (unblocks 11 each), then ready steps with 0, then
  blocked steps (`core-evidence/offline-proof` blocked by reliability and
  staging). Inspect's critical path is `core-bot/reliability →
  core-evidence/offline-proof → m1-stability → workspace/visual-contract →
  m3e-pass → later-extension/nexon-feasibility → guide-publisher →
  release-ops/roadmap-completion` (8 steps).
- **offline-proof completed early:** the update succeeds with `Order
  warning: step core-evidence/offline-proof was set to completed while its
  prerequisites are not completed: core-bot/reliability (draft),
  core-bot/staging (draft)...`. Validate stays `valid: true` with the
  matching `dependencies: Order warning`, and doctor repeats it. The doctor
  plan is `valid: false` only because the owner's checkpoint went stale with
  the update, as in D.1.
- **nexon-feasibility cancelled:** the update warns that
  `later-extension/nexon-enrichment` and `guide-publisher` are now blocked
  by a cancelled prerequisite. Validate and doctor flag both, and resume
  lists both `Dependency order warning`s in the shown safety warnings. The
  items carry `blockedBy[{... nexon-feasibility, status: "cancelled"}]`
  (guide-publisher also `member-portal/member-reads (draft)`).
- **Dropping later-extension via phase replacement:** refused (exit 1,
  before the prompt) with the six dangling links listed
  (`dependencies.8: Source step later-extension/nexon-feasibility does not
  exist; ...`). The plan bytes are unchanged.
- **Empty dependsOn:** refused with `Invalid dependency metadata:
  dependencies.12.dependsOn: Dependency entry must list at least one
  prerequisite`.
- **Backward link** (`core-bot/context` depends on
  `release-ops/metadata-docs`): accepted. The result and validate carry
  `dependencies.12.dependsOn.0: Backward link: core-bot/context depends on
  release-ops/metadata-docs, which comes later in plan order.`

### Owner decisions on the D.2 review (2026-09-30)

1. All dependency refusals (existing and G4/G5) have the error class
   `invalid_structure` (was `internal`); message text unchanged
   (`internal/engine/errclass.go`, contracts §12).
2. G2 also warns when a step moves to `review` with unmet prerequisites
   (update result, validate, doctor, patch validation).
3. Current-step selection stays as in D.1 (a blocked current step is shown
   as `blocked`).
4. `estimate` stays an optional hand-edited step member (documented in
   contracts §12 G6); no writer sets it.
5. `workplan_reset` (draft) clearing phases without touching the sidecar is
   left for D.3.

After these, the validation above was re-run: gofmt, go vet (darwin and
`GOOS=linux`), `go test -race -count=1` per package, adapter `bun test` and
the static build all pass. No pinned vector changed (the oracle records
error messages, not classes).

### Remaining issues (D.2)

1. **Budget cost.** On the synthetic graph roadmap the 12000 budget gives
   5 items instead of 6 (the readiness members). All D.1 rules still hold.
2. Mixed writers: the D.2 core and the reference TypeScript plugin now also
   differ in resume/inspect/doctor/validate output and in the error class
   of dependency refusals. Stage E's one-writer-per-root rule is unchanged.

## Stage D.3 — safety and robustness: DONE (committed `83a3414`)

Base `e47e667`. Nothing was committed. The pinned `vendor/shiori` copy
(`e47e667`) that the owner's OpenCode uses and everything under
`~/.config/opencode` were not touched. The decisions and exact behaviour
are in [contracts §13](contracts.md#13-approved-design-changes-d3-approved-2026-09-30).
D.1 resume budgeting and D.2 graph behaviour are unchanged (no resume or
paging vector changed).

### Changes

| Item | What changed | Where |
| --- | --- | --- |
| 1 reset | `draft` resets statuses only (plan/phases/steps → draft, checkpoint removed, content and dependency graph kept; handwritten Markdown kept unless `replaceMarkdown`). New `wipe`: read-only preview → exact `previewToken` + `confirmation: WIPE_PLAN_CONTENT`; archive of the complete originals first, then plan/Markdown, checkpoint and dependency sidecars deleted in the same transaction (`reset:wipe`). Schema, registration (`d3` key), CLI `--preview-token/--confirm` | `internal/engine/reset.go`, `mutinput.go`, `recovery.go`, `schema/v1/tools/workplan_reset.input.schema.json`, `adapter/opencode/src/registration.json`, `scripts/snapshot-registration.ts`, `internal/cli` |
| 2 unreadable plans | raw-byte `planHash`/`stateHash` in doctor/validate/list; `create --overwrite` accepts that hash (Markdown kept unless `replaceMarkdown`); recovery handles the undecodable before-image | `internal/snapshot/snapshot.go` (`LoadUnreadable`), `create.go`, `validate.go`, `doctor.go`, `list.go`, `recovery.go` |
| 3 stale Markdown | doctor stray kind `stale-markdown` for `<id>.md` of a plan that links elsewhere | `doctor.go` |
| 4 markers | non-failing warning for steps whose marker is missing from handwritten Markdown | `validate.go` (`missingStepMarkers`) |
| 5 polish | stale-checkpoint detail (doctor); title-slug ids with `-N` on collision; missing `specFiles` refused (`invalid_input`); list `issues` array on every entry; 16 KiB note limit; new plan/Markdown mode from existing plans (else 0644 minus umask), archives/sidecars 0600 | `engine.go` (`staleDetail`), `planedit.go`, `update.go`, `list.go`, `mutate.go` (`fileMode`), `umask_*.go`, `errclass.go` |
| 6 recovery | separate-process SIGKILL test; recovery also deletes the interrupted transaction's staging files | `d3_test.go` (`TestD3KillRecovery`), `recovery.go` |
| 7 host versions | unverified OpenCode: read-only tools work, mutating tools refused (`unsupported_capability`), bridge not started; doctor `runtimeFacts.host` | `adapter/opencode/src/plugin.ts`, `doctor.go` (`runtimeFacts`) |

### Vector expectations

The oracle corpus is unchanged. `testdata/d3/expectations.json` pins 24
vectors: 20 tools vectors (items 2/4/5: raw-byte hashes, marker warnings,
stale-checkpoint detail, list shape) and four mutation vectors. Read
vectors are judged against the same engine with the D.3 additions off
(`Engine.noD3`), which must pass every earlier check; removing the approved
members must give that output byte-for-byte, and the additions are restated
from raw fixture bytes. `SHIORI_D3_UPDATE=1` re-pins.

| Category | Pass (oracle-exact) | Earlier approved | D.1 | D.2 | D.3 | Fail |
| --- | --- | --- | --- | --- | --- | --- |
| tools (199) | 149 | 9 | 16 | 5 | 20 | 0 |
| resume (87) | 77 | 0 | 4 | 6 | 0 | 0 |
| paging (74) | 43 | 0 | 31 | 0 | 0 | 0 |
| mutations (70) | 56 | 7 | 1 | 0 | 6 | 0 |
| mutating input (39) | 37 | 1 (D2) | 0 | 0 | 1 | 0 |

Six of the 20 D.3 tools vectors were earlier D.1 pins or D6 divergences;
their D.3-off output still passes those checks. D.3 mutation vectors:
`reset-draft`, `reset-draft-preserve-notes` (status-only reset restated
from the stored plan), `create-generated-ids` (oracle files after
substituting the ids), `create-new-full` (spec refused; with the spec
present the files equal the oracle's), `update-recovery-resume/-rollback`
(only extra change: the leftover staging file is removed; output
unchanged, so not pinned). Input vector `reset--bad-mode` lists `"wipe"`.

### New tests

- `d3_test.go`: `TestD3DraftResetKeepsStructure`, `TestD3WipePreviewConfirm`,
  `TestD3UnreadablePlanRepair` (truncated plan → doctor hash → create
  overwrite), `TestD3StaleMarkdownCopy`, `TestD3StepMarkers`,
  `TestD3StaleCheckpointDetail`, `TestD3GeneratedIDs`,
  `TestD3SpecFilesMustExist`, `TestD3NoteLimit`, `TestD3FileModes`,
  `TestD3KillRecovery` (6 child processes: wipe killed after the journal
  and mid-publication, update mid-publication; each resumed and rolled
  back).
- `TestFaultInjection` adds `reset-wipe`, `reset-draft-checkpoint` and
  `create-overwrite-unreadable`, and fault points `publish:3`/`publish:4`.
- Protocol: `TestResetWipePreviewThenPreparedApply`; host facts in
  `TestDoctorRuntimeFactsAreSanitized`.
- Adapter: registration differs from the reference only by the `d1`/`d3`
  changes; unverified host (2.1.0) degradation; verified host facts.

### Evidence (darwin/arm64, Go 1.27.1, bun 1.4.0)

- `gofmt -l`: clean. `go vet ./...` (darwin and `GOOS=linux`): clean.
- `go test -race -count=1` per package: all pass (engine 108 s).
- `bun test` in `adapter/opencode`: 49 pass, 1 skip (opt-in runtime smoke).
- `CGO_ENABLED=0 go build -trimpath ./cmd/shiori`: builds.

Owner-plan evidence ran on fresh `cp -Rp` copies of the read-only fixture
(`chmod -R u+w` on the copy); nothing from it is in the repository. The
pristine fixture's paths, modes, mtimes and bytes were identical before and
after. Before is `e47e667`, after is the working tree.

- **Draft reset** (after adding a 5-entry dependency graph and setting the
  plan and `core-bot/context` in progress): `e47e667` refuses the
  handwritten Markdown; with `--replace-markdown` it leaves 0 phases, 0
  steps, 0 notes, 0 findings, overwrites the Markdown, keeps the checkpoint
  and leaves 12 dangling dependency issues. D.3 without flags keeps 13
  phases, 38 steps, 35 notes and 4 findings with every status `draft`,
  removes the checkpoint, leaves the Markdown and the dependency sidecar
  byte-identical, and validate is `valid: true` with no dependency issue.
- **Wipe:** without `--replace-markdown` the preview is refused
  (handwritten Markdown); a token without `--confirm` and a wrong
  confirmation are input errors; a wrong token is refused. The preview
  changed nothing and listed 13/38/4/35 removals and the two sidecar
  deletions. Apply wrote
  `archive/kanade-v5-roadmap/state-0947ec8328a2-51963ef4dab8.json` (0600)
  holding the exact original JSON, Markdown, checkpoint and dependency
  bytes, then left an empty draft plan with no sidecars.
- **Truncated plan** (JSON cut in half): `e47e667` doctor/validate/list
  give no hash and `create --overwrite` fails (`Invalid workplan JSON`). D.3
  doctor, validate and list report the same raw `stateHash`
  (`bbf72afd…`); a wrong hash is `stale_state`; `create --overwrite
  --expected-hash bbf72afd…` succeeds and keeps the handwritten Markdown.
  Doctor afterwards names the changed JSON and the three spec links the
  new plan no longer has.
- **SIGKILL** (engine test binary as the writer; update of status, a note
  and the dependency sidecar; killed after the journal was published and
  between the two publications): the copies held the journal, both locks
  (dead PID) and the staging files. Doctor reported `recoveryRequired` and
  a state hash. CLI recovery right away failed with `lock_unavailable`
  (dead owner within the 5-minute grace). After 5 minutes, CLI `update
  --recovery resume` put both targets at their after images (identical
  state hash in both kill cases) and `rollback` restored the exact
  original files; no lock, staging file or journal remained and validate
  was `valid: true` in all four copies.
- **Unverified host** (adapter with the test fakes of the host, OpenCode
  version 2.1.0, on a copy): 13 tools registered, bridge not started;
  read (filtered), list, inspect, validate, resume and compact_preview
  answered; doctor reported `host {opencodeVersion 2.1.0, verified false,
  writes disabled}`; update, reset and checkpoint were refused with
  `unsupported_capability` and no permission request. On 2.0.20 the same
  update reached the permission prompt and committed.

### Owner decisions to review (D.3)

1. The draft reset removes the checkpoint without archiving it (the brief
   asks for archives only on wipe; the journal holds it until commit).
2. The draft reset keeps handwritten Markdown by default (the earlier draft
   reset refused it; regenerating would destroy it).
3. `workplan_compact` is refused as a whole on an unverified host
   (including its preview mode, which `compact_preview` covers).
4. New sidecars (checkpoint, dependencies) stay 0600; only plan JSON and
   Markdown follow the plan mode. A read-only plan mode (0444) gives
   owner-writable new files (0644).
5. The list entries lose the single `issue` string (replaced by `issues`).
6. Recovery after a real crash waits out the 5-minute lock grace (S11,
   unchanged); `TestD3KillRecovery` simulates the elapsed grace with the
   lock clock.

### Remaining issues (D.3)

1. Resume keeps the D.1 stale-checkpoint text (its budget is unchanged);
   the detail is in doctor only.
2. The plan-wide step-id uniqueness applies to generated ids only;
   explicit duplicate step ids across phases stay allowed (reference
   behaviour), and the owner's roadmap has six `intake` steps.
3. Mixed writers: the D.3 core and the reference TypeScript plugin now
   also differ in reset semantics, list shape and doctor output. Stage E's
   one-writer-per-root rule is unchanged.

## Stage D.3.1 — third live-testing round: DONE (committed `df74f35`)

Base `83a3414`. Nothing was committed. The pinned `vendor/shiori` copy
(`83a3414`) that the owner's OpenCode uses and everything under
`~/.config/opencode` were not touched. The decisions and exact behaviour
are in [contracts §14](contracts.md#14-approved-design-changes-d31-approved-2026-09-30).
No tool input schema or registration changed.

### Changes

| Item | What changed | Where |
| --- | --- | --- |
| 1 step gate | update (and create, for the steps it creates) refuses to move a step to `in_progress`/`review`/`completed` (new step, or status changed by any edit) while that step fails its spec 01 §3 rules; before authorization, class `invalid_structure`, field-path `issues` (CLI `--json`, protocol); same-call completion accepted; unchanged gated steps not re-checked | `internal/engine/planedit.go` (`stepStatusGate`, `StepStatusGateError`), `update.go`, `errclass.go`, `internal/cli/cli.go`, `internal/protocol/server.go` |
| 2a repair archive | `create --overwrite` of an unreadable plan publishes `archive/<id>/state-<stateHash:12>-<rawSha:12>.json` (0600) first in the same `create:overwrite` transaction: exact damaged bytes (string, or base64 when not UTF-8) plus Markdown and sidecars; result `archivePath`; recovery accepts the archive target | `internal/engine/repair.go`, `create.go`, `recovery.go`, journal schema description |
| 2b recovered link | doctor `recoveredPlanFile` from a tolerant raw scan (single value, plan-file policy, not another plan's link); the repair keeps that Markdown path unless `planFile` is given | `repair.go` (`recoverPlanFile`), `doctor.go`, `create.go`, CLI human output |
| 2c repair hint | update/patch/reset/checkpoint/compact apply on an unreadable plan name `workplan_create overwrite=true and expectedHash=<stateHash>`; class unchanged | `repair.go` (`UnreadablePlanError`), `mutate.go`, `compact.go` |
| 3 stale diagnostic | resume `checkpoint.diagnostic` = `changed: <path>[, …] [+N more]` (≤3 paths, ≤240 units) for a stale v2 checkpoint | `engine.go` (`staleDiagnostic`), `resume.go` |
| 4 critical path | resume `criticalPath {length, nextStep}` with a valid sidecar and ≥2 open steps; dropped before any text would go below the D.1 minimums | `resume.go` (`criticalView`, `chooseResume` split into levels/emergency) |
| 5a wiped note | doctor warning for an empty draft plan with a `reset:wipe` archive; validate unchanged | `doctor.go` (`wipedNote`) |
| 5b cosmetics | findings render `title (status)`; generated detection accepts old and new renderings everywhere; new writes whole-second UTC | `internal/model/markdown.go` (`RenderMarkdownLegacy`, `IsGeneratedMarkdown`), `engine/mutate.go` (`nowISO`, `render`, `isGenerated`), `create.go`, `update.go`, `reset.go`, `compact.go`, `validate.go`, `read.go` |

### Vector expectations

The oracle corpus is unchanged. Every vector runs with the D.3.1 changes
off (`Engine.noD31`), which passes every earlier check unchanged, and on;
a difference must be pinned in `testdata/d3_1/expectations.json` and pass
the D.3.1 comparator (contracts §14). `SHIORI_D31_UPDATE=1 go test
./internal/engine ./internal/model` re-pins (run the packages one at a
time; each rewrites only its own prefixes).

| Category | Pass (oracle-exact) | Earlier approved | D.1 | D.2 | D.3 | D.3.1 | Fail |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tools (199) | 149 | 9 | 16 | 5 | 20 (3 also D.3.1) | 3 (2b recovered planFile, doctor) | 0 |
| resume (87) | 74 | 0 | 4 | 6 (3 also D.3.1) | 0 | 6 (3 stale diagnostic, 3 critical path) | 0 |
| paging (74) | 43 | 0 | 31 | 0 | 0 | 0 | 0 |
| mutations (70) | 56 (D.3.1-off) | 7 | 1 | 0 | 6 | 17 (5b; 7 also refresh generated Markdown) | 0 |
| mutating input (39) | 37 | 1 | 0 | 0 | 1 | 0 | 0 |
| Markdown render (13) | 12 | 0 | 0 | 0 | 0 | 1 (`finding-permutations`) | 0 |

The D.3.1 columns overlap the earlier ones: a D.3.1 vector's D.3.1-off
output is still judged by its earlier check (for example the three doctor
vectors are D.3 raw-hash vectors, the critical-path resume vectors are D.2
readiness vectors). `mutations/update-dependencies-empty-replace` now also
writes the plan JSON (its stored `updatedAt` was the frozen clock in the
millisecond form, so the D.3.1-off write left the bytes unchanged) and
refreshes the old-rendering Markdown. No vector exercises item 1 or 2c.

### New tests

- `d3_1_test.go`: `TestD31StepStatusGate` (all three gated statuses,
  paths, no write, allowed statuses, same-call completion, addSteps /
  addPhases / phases replacement, unchanged gated step editable),
  `TestD31UnreadableRepair` (recovered link, hint on update/reset/
  checkpoint, archive bytes/mode/layout, base64 for non-UTF-8, explicit
  `planFile` wins, ambiguous/unsafe/missing members and another plan's
  link give nothing),
  `TestD31ResumeStaleDiagnostic` (one path, `+N more`, cap, legacy null),
  `TestD31ResumeCriticalPath` (agrees with inspect; byte-identical without
  a sidecar on three fixtures), `TestD31ResumeBudget` (4096–16000, three
  limits: fits, diagnostic present, critical path never kept below the
  minimums; fails without the drop), `TestD31WipedNote`,
  `TestD31LegacyGeneratedMarkdown` (old rendering: generated, no drift
  warning, update and markdown-only refresh it; a real hand edit is still
  handwritten), `TestD31Timestamps`.
- `internal/model`: `TestMarkdownVectors` checks the legacy rendering
  against the oracle and the D.3.1 rendering against it, and that both
  are detected as generated.
- CLI `TestStepStatusGateCLI`, `TestCreateStepStatusGateCLI`; protocol
  `TestStepStatusGateIssues`. `TestD31StepStatusGate` also covers create.
- `TestFaultInjection` `create-overwrite-unreadable` now includes the
  archive target at every fault point.

### Evidence (darwin/arm64, Go 1.27.1, bun 1.4.0)

- `gofmt -l`: clean. `go vet ./...` (darwin and `GOOS=linux`): clean.
- `go test -race -count=1` per package: all pass (see the run log in the
  handoff).
- `bun test` in `adapter/opencode`: 49 pass, 1 skip (opt-in runtime
  smoke).
- `CGO_ENABLED=0 go build -trimpath ./cmd/shiori`: builds.

Owner-plan evidence ran on fresh `cp -Rp` copies of the read-only fixture
(`chmod -R u+w` on the copy); nothing from it is in the repository. The
pristine fixture's paths, modes, mtimes, sizes and bytes were identical
before and after. Before is `83a3414` (D.3), after is the working tree.

- **Unmodified copy:** resume (12000/6000/4096), validate, doctor,
  `inspect --limit 500` and `read --phase core-bot` are byte-identical
  between D.3 and D.3.1 (fresh checkpoint, no sidecar, handwritten
  Markdown).
- **Step gate:** after adding a draft step with an action but no
  validation, `updateSteps … in_progress` is refused (exit 1,
  `invalid_structure`, `issues[{path: "phases.6.steps.7.validation"}]`,
  plan bytes unchanged); D.3 accepts the same update. `blocked` is
  accepted, and `in_progress` plus `validation` in the same call is
  accepted.
- **Unreadable plan with a moved link** (`planFile` moved to
  `.opencode/workplan/roadmap/kanade-v5.md`, JSON cut in half): D.3.1
  doctor reports `recoveredPlanFile: .opencode/workplan/roadmap/kanade-v5.md`;
  update is refused with the repair hint naming the doctor state hash;
  `create --overwrite --expected-hash <that hash>` relinks the handwritten
  `roadmap/kanade-v5.md` and writes
  `archive/kanade-v5-roadmap/state-c8ce6f258585-1d43c6e33c63.json` (0600,
  `workplanJson` byte-identical to the damaged file, sha256 matches, 115 KB
  of original Markdown). D.3 has no `recoveredPlanFile`, no hint and no
  archive, and relinks the stale `kanade-v5-roadmap.md` copy left by the
  move (the wrong file).
- **Stale checkpoint** (a line appended to the Markdown): D.3 resume
  diagnostic `null`; D.3.1 `changed: .opencode/workplan/kanade-v5-roadmap.md`
  at 12000/6000/4096, with the same page sizes (6/1/1) and truncated-field
  sets; +46 units.
- **Critical path** (3-entry graph `core-bot/reliability,staging →
  core-evidence/offline-proof → m1-stability → workspace/visual-contract`,
  which also makes the checkpoint stale): D.3.1 resume adds
  `criticalPath {length 4, nextStep core-bot/reliability}` at
  64000/12000/6000 (page sizes 20/6/1, same as D.3; truncated fields
  unchanged), matching inspect. At 4096 the critical path is dropped; the
  D.3 packet was already in the emergency tier there (display cap 64), and
  the stale diagnostic moves it one step lower (48).
- **Wipe:** doctor adds `phases: Plan was wiped …; the removed content is
  archived at .opencode/workplan/archive/kanade-v5-roadmap/state-8ed35bcbe9c6-3725f90fc967.json. Add phases …`;
  validate keeps only `phases: At least one phase is required`; D.3 doctor
  has no note.
- **Old-rendering generated Markdown** (regenerated with the D.3 binary,
  4 findings `…(resolved)`/`…(open)`): D.3.1 validate gives no drift
  warning; a D.3.1 update refreshes it to `… finalized (resolved)— …` and
  records `updatedAt 2026-09-30T15:07:56Z` (D.3: `…:56.329Z`, rendering
  unchanged); `createdAt` stays as stored; validate stays `valid: true`
  without warnings.

### Owner decisions to review (D.3.1)

1. The step gate also applies to `workplan_create` (coordinator review,
   2026-09-30): every step created in `in_progress`/`review`/`completed`
   must pass the per-step rules (same class and `issues`).
2. "Set by this call" means a new step or a changed status; a step already
   in a gated status is not re-checked by unrelated edits (keeps repairs
   possible; a `phases` replacement that restates an existing in-progress
   step unchanged is not gated).
3. The repair archive name is `state-<stateHash:12>-<sha256(raw):12>.json`
   (compaction/wipe layout; recovery validation already requires the
   `state-` prefix), and it also stores the Markdown and sidecars, not only
   the damaged JSON.
4. `recoveredPlanFile` is reported by doctor only (not validate or list);
   members that disagree, or a link another readable plan owns, yield
   nothing.
5. The resume critical path is dropped before any text would go below the
   D.1 minimums; the stale diagnostic is never dropped (it can only
   shrink).
6. Lock owner files keep millisecond `startedAt`; journals, archives,
   plans and sidecars use whole seconds. Two writes in the same second can
   now produce the same `updatedAt`; a write that changes nothing else is
   then a no-op (no intent, D12 behaviour).
7. `reset --mode markdown-only` on old-rendering generated Markdown now
   rewrites it to the new rendering (one authorization) instead of the D12
   no-op.

### Remaining issues (D.3.1)

1. At the minimum budget the roadmap packet was already in the emergency
   tier; with a stale checkpoint the diagnostic costs about 50 units and
   takes the emergency cap one step lower (64 → 48) there.
2. Mixed writers: the reference plugin and D.3 write `title(status)` and
   millisecond timestamps; D.3.1 treats both renderings as generated, but
   a reference write after a D.3.1 write flips the rendering back (still
   classified as generated either way). Stage E's one-writer-per-root rule
   is unchanged.

## Stage D.4 — compaction advisor and note rollover: DONE (committed 02d2333)

Base `df74f35`. Nothing was committed. The pinned `vendor/shiori` copy
(`df74f35`) that the owner's OpenCode uses and everything under
`~/.config/opencode` were not touched. The decisions and exact behaviour
are in [contracts §15](contracts.md#15-approved-design-changes-d4-approved-2026-09-30);
spec 06 marks P2 and P3 implemented. No V2 field changed; tool identities
are unchanged; the only input change is the optional `noteRollover` on
`workplan_compact`/`workplan_compact_preview`.

### Changes

| Item | What changed | Where |
| --- | --- | --- |
| P2 advisor | exact plan JSON saving of archiving archivable completed phases, rollover notes and resolved findings (pretty-encoding footprints, archive pointer and `updatedAt` included); recommended at ≥32 KiB saving plus one threshold (50 eligible notes, archivable phases ≥25% of the JSON, JSON ≥192 KiB); doctor plan entry `compactionRecommended` (estimate incl. generated-Markdown render, counts, kept reasons, reasons, selection, thresholds, instruction); resume compact `compactionRecommended {savedJsonBytes, savedJsonPercent, notes, terminalSteps, resolvedFindings}`, dropped (before the D.3.1 critical path) before any text would go below the D.1 minimums | `internal/engine/advisor.go`, `resume.go` (`chooseResume`), `doctor.go`, `engine.go` (`Compaction`, `noD4`) |
| thresholds | `--compaction-advice off\|min-savings-kib=N,notes=N,terminal-percent=N,plan-kib=N,keep-notes=N` on `resume`, `doctor` and `serve` (protocol `Options.Compaction`); never model input | `advisor.go` (`ParseCompactionThresholds`), `internal/cli/cli.go`, `serve.go`, `internal/protocol/server.go` |
| P3 rollover | `noteRollover {keepLatest 1–10000 (20), pinNoteIndexes}`: selects notes older than the latest N except pinned (`[pinned]`/indexes), decision records, open-step/open-finding references and archive pointers; token binds `canonicalSelection.noteRollover`; combination with `noteIndexes` is an input error; rollover-mode preview adds `noteRollover` and `estimatedSavings`, apply adds `savings`; archive and apply unchanged | `advisor.go` (`rolloverSelect`), `compact.go`, `mutinput.go` (`kInt`, `rolloverSpec`, refine), schemas, `registration.json` (`d4`), `scripts/snapshot-registration.ts`, CLI `--rollover --keep-notes --pin-note` |
| CLI output | human `resume`/`doctor` show the advice | `internal/cli/human.go` |

### Vector expectations

The oracle corpus is unchanged. Every tools/resume/paging vector runs with
the advice off (`Engine.noD4`), which passes every earlier check, and on;
`testdata/d4/expectations.json` lists **no** vector: with the default
thresholds no corpus fixture qualifies (`large-paging`, the largest
eligible history, could save 13 080 of 45 232 JSON bytes), so all 360
read vectors are byte-identical to D.3.1. Mutation (70) and mutating-input
(39) vectors are unchanged. Parity: tools 149 pass / 50 approved
divergences, resume 74 / 13, paging 43 / 31, 0 fail (same as D.3.1).
`TestD4ComparatorOnCorpus` runs the comparator (output minus the advice =
D.4-off text; counts restated from the raw plan by an independent
implementation) on 10 doctor/resume vectors with lowered thresholds; one
4096-budget resume packet pays for the advice with a page item and is
skipped there.

### New tests

- `d4_test.go`: `TestD4AdvisorReport` (counts per keep reason, restated;
  resume = doctor figures; off/unmet thresholds byte-identical to
  D.4-off), `TestD4AdviceMatchesApply` (applying the advised selection
  writes exactly the estimated JSON and Markdown sizes; advice then gone),
  `TestD4RolloverPreviewApply` (selection, token binds keepLatest/pins,
  wrong parameters and stale checkpoint refused before authorization,
  archive holds the complete original notes and plan JSON, remaining
  notes = kept + pointer), `TestD4RolloverInput` (both surfaces, both
  tools), `TestD4ResumeBudget` (4096–16000 × 3 limits on a heavy header:
  fits; advice never kept below the minimums; a packet without it equals
  D.4-off; shown 860, dropped 91), `TestD4NoEligibleHistoryIdentical`,
  `TestParseCompactionThresholds`.
- `d4_vectors_test.go`: the D.4 split in `TestCorpusParity`, the
  comparator and `TestD4ComparatorOnCorpus`.
- CLI `TestCompactRolloverCLI`, `TestCompactionAdviceFlag`; protocol
  `TestNoteRolloverPreviewThenPreparedApply`; adapter registration differs
  from the reference only by the `d1`/`d3`/`d4` changes.

### Evidence (darwin/arm64, Go 1.27.1, bun 1.4.0)

- `gofmt -l`: clean. `go vet ./...` (darwin and `GOOS=linux`): clean.
- `go test -race -count=1 -timeout 20m` per package: all pass (engine
  160 s).
- `bun test` in `adapter/opencode`: 49 pass, 1 skip (opt-in runtime smoke).
- `CGO_ENABLED=0 go build -trimpath ./cmd/shiori`: builds.
- Cost (CLI, median of 7, D.3.1 → D.4): resume 4.9 → 4.9 / 8.8 → 9.3 /
  47.1 → 49.9 ms and doctor 4.8 → 5.4 / 14.6 → 18.4 / 95.5 → 132.0 ms at
  100 KiB / 1 MiB / 10 MiB (the perf plans qualify; doctor renders the
  generated Markdown twice for the Markdown figures). A first version that
  re-encoded the plan per component and matched notes against every open
  step cost 391 ms for the 10 MiB resume; it was replaced by footprint
  arithmetic and linear-time note classification.

Owner-plan evidence ran on fresh `cp -Rp` copies (`chmod -R u+w`) of the
read-only fixtures; nothing from them is in the repository. The pristine
copies' paths, modes, mtimes, sizes and bytes were identical before and
after. Before is `df74f35` (D.3.1), after is the working tree.

- **Roadmap copy** (`kanade-v5-roadmap`, 65 939-byte JSON, 35 notes): no
  advice. `resume` (12000/6000/4096), `doctor`, `validate`, `inspect
  --limit 500`, `read --phase core-bot` and the full `read` are
  byte-identical between D.3.1 and D.4.
- **Hardening copy** (`kanade-v5-beta-hardening`: 257 151-byte JSON,
  203 367-byte handwritten Markdown, 12 phases, 62 steps, 214 notes, 32
  resolved findings, fresh checkpoint). `validate` is identical. Doctor
  advice: reasons `notes: 105 notes are eligible for rollover (threshold
  50)` and `size: the plan JSON is 257151 bytes (threshold 196608)`;
  estimate JSON 257 151 → 176 182 (−80 969, 31%), Markdown preserved
  (handwritten), total 17%; terminal steps 28, of which 2 archivable (only
  phase `integration` qualifies; `privacy` has a cancelled step and the
  rest are unfinished) = 2 140 bytes; notes kept: latest 20, decision 85,
  open reference 4, eligible 105 = 60 459 bytes; resolved findings 32 =
  18 486 bytes. Resume at 12000 adds `{savedJsonBytes 80969,
  savedJsonPercent 31, notes 105, terminalSteps 2, resolvedFindings 32}`
  and returns 5 instead of 6 page items (truncated fields 30 vs 32).
- **Rollover** (`compact --rollover`): preview selected 105 of 194 older
  notes with `estimatedSavings` JSON 257 151 → 196 808 (−60 343, 23%);
  apply wrote exactly 196 808 bytes (`savings` identical), left the
  handwritten Markdown byte-identical and refreshed the checkpoint. The
  archive `archive/kanade-v5-beta-hardening/state-538980bf42a0-5ffce10e9ccc.json`
  (0600, 535 KB) holds all 105 removed notes byte-identical to the
  originals (59 616 bytes of note text), `source.workplanJson`
  byte-identical to the original JSON (sha256 `d4ea2174534a…`), the
  original Markdown and checkpoint. Remaining notes = the 109 kept notes
  in order + the archive pointer; validate `valid: true`; doctor no
  longer recommends (the remaining history saves under 32 KiB).
- **Advised selection** (doctor's `selection` passed to preview/apply on
  a second copy): archived 1 phase, 105 notes and 32 findings; JSON
  257 151 → 176 182 bytes, exactly the doctor estimate.
- **Sizes after** (bytes; before / after rollover / after advised
  selection): plan JSON 257 151 / 196 808 / 176 182; full `read` 576 412 /
  515 861 / 492 553; `read --no-markdown` 372 286 / 311 735 / 288 427;
  `resume` 12000: 11 595 / 11 563 / 11 562 (5 items; notes are not part of
  resume); `resume --max-chars 64000`: 50 937 / 50 875 / 50 874.

### Owner decisions to review (D.4)

1. **Decision-record rule.** A note is a decision record (pinned) when it
   contains `decision`/`decisions`/`decided` (any case) or the uppercase
   word `USER`. On the hardening plan this keeps 85 of 194 older notes.
   Decision words only would archive 131 notes (72 KB). Matching the
   Markdown `## Decision register` lines to notes was not feasible (the
   texts differ), so the rule does not look at the register itself.
2. **Resume budget cost.** Like D.3.1's critical path, the advice is
   dropped only before text would go below the D.1 minimums, so it can
   cost a page item (hardening plan at 12000: 6 → 5). Alternative: drop it
   whenever it would shrink the page.
3. Resume reports the plan JSON saving only (no Markdown render on the
   hot path); doctor adds the Markdown and total figures.
4. "Older than the newest checkpoint" is enforced by apply's existing
   fresh-checkpoint rule; notes carry no timestamps.
5. Earlier archive pointer notes are kept (one per compaction).
6. Thresholds are CLI/serve flags only; the adapter passes none, so the
   live setup uses the defaults.
7. Terminal steps in unfinished phases are never archivable (compaction
   rule unchanged), so most of the measured "terminal steps ≈48%" stays;
   step-level archival would be a new design.

### Remaining issues (D.4)

1. Doctor at 10 MiB costs +37 ms (two Markdown renders for the Markdown
   figures).
2. Mixed writers: the reference plugin has no advice and no
   `noteRollover`; stage E's one-writer-per-root rule is unchanged.

## Stage D.4.1 — expectedHash guidance: DONE (owner adds id)

Base `470e52d` (D.4 = `02d2333`). Nothing was committed. The pinned
`vendor/shiori` copy (`02d2333`) that the owner's OpenCode uses and
everything under `~/.config/opencode` were not touched; no OpenCode or
`shiori serve` process was started, stopped or restarted (the adapter
`bun test` runs its own temporary stdio children). Found in real use on
2026-10-01: an agent called `workplan_patch` right after a
`workplan_update` without `expectedHash`. The refusal is correct and
stays; only the guidance changed. Decisions and exact texts are in
[contracts §16](contracts.md#16-approved-design-changes-d41-approved-2026-10-01).
No input shape, error class or output member changed.

### Changes

| Item | What changed | Where |
| --- | --- | --- |
| missing hash | native refusal (update, patch, reset, checkpoint, compact apply, create overwrite) appends ` — pass expectedHash set to the stateHash from your last successful write, or re-read with workplan_resume or workplan_inspect first`; leading sentence, path and class unchanged; no hash echoed; core surface unchanged | `internal/engine/mutinput.go` (`nativeHashGuidance`, `msgNativeHash`) |
| stale hash | `stale_state` refusal keeps its reference text and appends ` Use workplan_resume or workplan_inspect for that re-read before retrying, so the retry is based on the current plan.` (the reference text already names the current hash; the addition names none) | `internal/engine/mutate.go` (`staleHashGuidance`) |
| descriptions | one `expectedHash` sentence on the six mutating tools (create: `With overwrite=true, …`; compact: `In apply mode, …`; others `Pass expectedHash = the stateHash from your latest read or successful write.`) | `adapter/opencode/src/registration.json` (key `d4_1`), `scripts/snapshot-registration.ts`, `schema/v1/tools/workplan_{create,update,patch,reset,checkpoint,compact}.input.schema.json` (`description`, `x-shiori-rules` native text) |

`registration.json` was updated by applying the new D.4.1 block of the
snapshot script to the committed D.4 snapshot (a JSON round trip checked
byte-identical first), not by re-running the script against the
reference plugin in `~/.config/opencode` (off limits during this stage;
its working tree also has uncommitted changes). The script now produces
the same `d4_1` key when it is next run.

### Vector expectations

The oracle corpus is unchanged. `testdata/d4_1/expectations.json` pins 6
vectors (SHA-256 and UTF-16 length of the error text): 5 mutating-input
vectors on the native surface (`compact--apply-no-token`,
`create--overwrite-without-hash`, `update--no-hash`,
`update--recovery-with-replaceMarkdown`, `update--recovery-with-title`)
and 1 mutation vector (`mutations/create-overwrite-stale-hash`). The
comparator (`internal/engine/d4_1_vectors_test.go`) requires each
guidance to follow its reference sentence, to contain no hash, and the
text without the guidance to equal the oracle message byte-for-byte.
`SHIORI_D41_UPDATE=1 go test ./internal/engine -run
'TestMutationInputVectors|TestMutationVectors'` re-pins. Everything else
is byte-identical to D.4: tools 149 pass / 50 approved, resume 74 / 13,
paging 43 / 31, mutations 56 pass (one of them the D.4.1 pin) / 14
declared divergences, mutating input 39 (2 earlier divergences, 5 D.4.1
pins on the native surface), 0 fail.

### New tests

- `d4_1_vectors_test.go`: the pin/comparator used by
  `TestMutationInputVectors` and `TestMutationVectors`, and
  `TestD41Guidance` (all six native writers carry the guidance with class
  `invalid_input` and no hash; the core surface never does; the stale
  message keeps its prefix, names the hash once and has class
  `stale_state`).
- Adapter `plugin.test.ts`: the registration test checks the `d4_1`
  changes (six tools, description-only, exact sentence) and restores
  them before the D.1/D.3/D.4 reversal; `referenceSha256` unchanged.

### Evidence (darwin/arm64, Go 1.27.1, bun 1.4.0)

- `gofmt -l`: clean. `go vet ./...` (darwin and `GOOS=linux`): clean.
- `go test -race -count=1 -timeout 20m` per package: all pass (engine
  164 s).
- `bun test` in `adapter/opencode`: 49 pass, 1 skip (opt-in runtime smoke).
- `CGO_ENABLED=0 go build -trimpath ./cmd/shiori`: builds.

## Resume after maintenance

See [Resume point](#resume-point-paused-after-d41) at the top. The Go
toolchain observed is 1.27.1 darwin/arm64; build and test commands are in
the [README](../README.md). If the oracle files change, the corpus is
stale: compare their sha256 against `testdata/MANIFEST.json` →
`oracle.files`.
