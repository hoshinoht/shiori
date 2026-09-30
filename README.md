# Shiori

**Shiori (しおり, “bookmark”)** is a proposed local-first work coordination
engine: preserve the plan, execution state, safety boundaries and the next
reliable action across workers and sessions.

This repository contains the specifications, the approved stage A contract
(machine schemas, a golden fixture corpus and a measured TypeScript baseline),
the stage B read-only Go core, the stage C transactional core with its
`shiori` CLI, the stage D native OpenCode adapter (`shiori serve --stdio`
plus `adapter/opencode/`), and the D.1, D.2, D.3, D.3.1 and D.4 approved
design changes
([contracts §11](docs/contracts.md#11-approved-design-changes-d1-approved-2026-09-30),
[§12](docs/contracts.md#12-approved-design-changes-d2-approved-2026-09-30),
[§13](docs/contracts.md#13-approved-design-changes-d3-approved-2026-09-30),
[§14](docs/contracts.md#14-approved-design-changes-d31-approved-2026-09-30),
[§15](docs/contracts.md#15-approved-design-changes-d4-approved-2026-09-30)).
The architecture is a Go core and CLI with a thin OpenCode JS/TS adapter.
Stages A–D, D.1, D.2, D.3, D.3.1 and D.4 are authorized. Automatic artifact migration, commits
and remote publication are not.

## Specifications

Read in this order:

1. [Core architecture and storage](docs/specs/01-core.md)
2. [Protocol and OpenCode adapter](docs/specs/02-protocol-and-adapter.md)
3. [Performance and data structures](docs/specs/03-performance.md)
4. [Worktree-aware execution](docs/specs/04-worktrees.md)
5. [Migration and acceptance](docs/specs/05-migration-and-acceptance.md)

Stage A contract:

- [Frozen contract v1 (APPROVED 2026-09-30)](docs/contracts.md), including the
  approved D.1, D.2, D.3, D.3.1 and D.4 design changes (§11–§15)
- [Machine schemas](schema/index.json) and [golden corpus](testdata/MANIFEST.json)
- [TypeScript reference baseline and Go stage B results](docs/baseline.md)

[Maintenance-safe status and handoff](docs/STATUS.md) records completed work,
validation limits and the next authorized decision without requiring OpenCode tools.

The baseline is the completed TypeScript workplan hardening in the local
OpenCode preset. Shiori should retain its tested behavior and existing
`.opencode/workplan/` V2 artifacts and `workplan_*` tool identities during
migration, rather than rewriting plans or changing agent names automatically.

Go module: `github.com/hoshinoht/shiori`. Binary: `shiori`. Stages B–D were
built and validated with Go 1.27.1 (and bun 1.4.0, OpenCode 2.0.20 for the
adapter) on darwin/arm64 only. That is not evidence of
cross-platform validation (contracts §5.6).

## Build and run

Requires Go 1.27.1 or newer. The binary has no runtime dependencies.

```sh
CGO_ENABLED=0 go build -trimpath -o shiori ./cmd/shiori
```

On macOS the binary still links the system `libSystem` (every Go binary
does); it has no other runtime dependency. On Linux `CGO_ENABLED=0` gives a
fully static binary.

### Read operations

Reads never prompt, write, lock, repair or change a file's mtime.

D.1 behaviour of the read operations (contracts §11):

- `resume` keeps text readable around a target page (8 items at budgets
  of 12000 or more, 4 at 6000 or more, 2 below). While the target fits it
  shortens text first, down to 240 code units for prose and 80 for titles
  (the current step, summary and next action keep at least 512), then to
  120. Below the target it returns fewer items (the cursor carries the
  rest). File paths and references are not shortened. Only when a single
  item cannot fit does text go below 120/80, and every cut is marked with
  `…` and listed in `truncatedFields`.
- `read --phase/--step` returns a slice: the plan header, the hashes and
  the selected phase/step. Findings and notes are added only with
  `--notes` (`includeNotes`), and the Markdown only with `--markdown`.
- `validate` and `doctor` warn, without failing, when the linked Markdown
  is not the generated rendering. `doctor` also lists orphaned sidecars
  and unclassified files in `.opencode/workplan/`. It never moves or
  deletes them.

D.2 behaviour when the plan has a valid dependency sidecar (contracts §12;
without one, output is the same as D.1):

- `resume` marks the current step and each open work item `ready` (with
  `unblocks`: how many open steps depend on it) or `blocked` (with
  `blockedBy`: the prerequisites that are not completed). Ready work comes
  first, ranked by `unblocks`, then blocked work. Open steps behind a
  cancelled prerequisite are flagged.
- Order checks only warn. An update that starts, reviews or completes a step before
  its prerequisites succeeds with `warnings`, and `validate`/`doctor`
  report order violations, cancelled prerequisites and backward links
  without changing `valid`.
- An update that replaces `phases` is refused if it would leave dependency
  links dangling. A dependency entry with an empty `dependsOn` is refused.
  Dependency refusals have the error class `invalid_structure`.
- `inspect` shows each step's prerequisites and dependents. `inspect` and
  `doctor` show the advisory critical path (the longest chain of open
  steps). The compaction preview lists steps that archived prerequisites
  still block.

D.3 behaviour of the read operations (contracts §13):

- A plan whose JSON cannot be loaded gets `planHash`/`stateHash` computed
  from its raw bytes in `doctor`, `validate` and `list`; pass that
  `stateHash` to `create --overwrite --expected-hash` to repair it.
- Every `list` entry has an `issues` array.
- `doctor` flags `<id>.md` left behind after a `planFile` move
  (`stale-markdown`) and names what changed since a stale checkpoint.
- `validate`/`doctor` warn when handwritten Markdown lacks a step's
  `<!-- workplan-step-id: ... -->` marker (it is never rewritten).

D.3.1 behaviour of the read operations (contracts §14):

- `resume` names what changed since a stale checkpoint in
  `checkpoint.diagnostic` (`changed: <path>[, …] [+N more]`, at most three
  paths). With a valid dependency sidecar it adds `criticalPath: {length,
  nextStep}` (dropped before any text would go below the D.1 minimums;
  the full path is in `inspect`/`doctor`).
- `doctor` reports `recoveredPlanFile` for an unreadable plan whose raw
  bytes still name a safe `planFile`, and notes an empty draft plan left by
  a wipe (with the archive path).
- Generated Markdown renders findings as `title (status)`; Markdown in the
  earlier `title(status)` rendering still counts as generated and is
  refreshed by the next write.

D.4 behaviour (contracts §15; plans without qualifying history give the
same output as D.3.1):

- `doctor` adds `compactionRecommended` to a plan entry when compaction
  could save at least 32 KiB of plan JSON and a threshold is crossed (50
  notes eligible for rollover, archivable completed phases ≥25% of the
  JSON, or a JSON of 192 KiB): the exact JSON (and generated-Markdown)
  saving, the counts, the reasons and a ready-to-preview selection.
  `resume` adds a compact `compactionRecommended` only when the packet
  with it keeps the same page items and text caps as without it (D.4.2;
  otherwise it is omitted and `doctor` keeps the full advice). Advice
  only.
  `--compaction-advice off|min-savings-kib=N,notes=N,terminal-percent=N,plan-kib=N,keep-notes=N`
  (resume, doctor, serve) sets the thresholds.
- `compact --rollover [--keep-notes N] [--pin-note I]...` (tool input
  `noteRollover`) selects every note older than the latest N (default
  20) except pinned notes (`[pinned]` or `--pin-note`), decision records
  (`decision`/`decided` or the uppercase `USER` marker), notes naming an
  open step or quoting an open finding, and the latest three archive
  pointer notes (D.4.2; older pointers are archived like other notes).
  It uses the unchanged preview → token → `ARCHIVE_SELECTED_HISTORY`
  flow; apply needs a fresh checkpoint, so every archived note predates
  it, and the archive keeps the complete originals.

```sh
shiori list     --root /path/to/project
shiori read     <id> [--phase ID] [--step ID] [--no-markdown | --markdown] [--notes]
shiori inspect  <id> [--phase ID] [--limit 1-500] [--cursor TOKEN]
shiori validate <id>                       # exit 1 when the plan is invalid
shiori resume   <id> [--max-chars 4096-64000] [--limit 1-100] [--cursor TOKEN] [--phase ID] [--step ID] [--compaction-advice SPEC]
shiori doctor   [id] [--limit 1-100] [--compaction-advice SPEC]
shiori compact  <id> --reason R [--archive-phase ID]... [--archive-note I]... [--archive-finding I]...   # preview only
shiori compact  <id> --reason R --rollover [--keep-notes N] [--pin-note I]...                          # D.4 rollover preview
```

### Mutations

Every mutation is prepared first (nothing is created: no file, lock or
directory), the exact intent is printed to stderr (operation, every target
with before/after sha256, journal, locks, staging count), and it is committed
only after confirmation. The commit takes the workspace lock then the plan
lock, rechecks every precondition, stages each file in the same directory
with fsync, publishes a durable journal, replaces each artifact atomically,
syncs directories and removes the journal. A failure after the journal
leaves it in place and reports `recovery_required`.

```sh
H=$(shiori read my-plan --json --no-markdown | jq -r .stateHash)
shiori create     my-plan --goal "Ship it" [--title T] [--plan-file .opencode/workplan/x.md] [--markdown-file F]
shiori update     my-plan --expected-hash "$H" --status in_progress --append-note "started"
shiori update     my-plan --expected-hash "$H" --input '{"updateSteps":[{"phaseId":"p","stepId":"s","status":"completed"}]}'
shiori patch      my-plan --expected-hash "$H" --patch-file change.patch [--validate]
shiori reset      my-plan --expected-hash "$H" [--mode draft|markdown-only] [--replace-markdown]
shiori reset      my-plan --expected-hash "$H" --mode wipe [--preserve-notes]                 # preview: prints previewToken
shiori reset      my-plan --expected-hash "$H" --mode wipe --preview-token TOKEN --confirm WIPE_PLAN_CONTENT
shiori checkpoint my-plan --expected-hash "$H" --summary S --next-action A [--phase ID --step ID] [--blocker B]...
shiori compact    my-plan --reason tidy --archive-phase done-phase                 # preview: prints previewToken
shiori compact    my-plan --reason tidy --archive-phase done-phase --apply \
                  --preview-token TOKEN --confirm ARCHIVE_SELECTED_HISTORY --expected-hash "$H"
shiori update     my-plan --recovery resume|rollback --expected-hash "$H"          # "$H" from doctor for a journal-only plan
```

- Confirmation: on a terminal, type `yes`; off a terminal, pass `--yes`.
  `--yes` is a flag only (never an environment variable or config file) and
  never bypasses the stale-hash, lock, journal or scope checks.
- Existing-state writes (everything except a fresh `create` and a compaction
  preview) require `--expected-hash` unless `--legacy-unhashed` is given;
  the state is still rechecked under the lock.
- `--input '<json>'` accepts any core-surface tool input; flags override its
  fields.
- `create`/`update` refuse to set the status to `in_progress`, `review` or
  `completed` while the executable-structure rules fail. The error lists
  every field path. `draft`, `blocked` and `cancelled` are always allowed.
  `patch --validate` returns the full issue list (D.1).
- D.3.1: `update` (and `create`, for the steps it creates) also refuses to move a step to `in_progress`, `review`
  or `completed` while that step lacks its own `action`, `validation`,
  `title` or a unique `id` (field paths in the error; completing them in
  the same call is fine).
- `reset` (draft) only resets statuses to draft and removes the checkpoint;
  phases, steps, notes, findings and dependencies are kept. `--mode wipe`
  clears the content after a preview and the exact token and confirmation,
  archiving the originals under `.opencode/workplan/archive/<id>/` first
  (D.3).
- New `specFiles` must exist, a new note is at most 16 KiB, generated ids
  are title slugs (`-2`... on collision), and new plan/Markdown files take
  the mode of the existing plans (else 0644 minus the umask) (D.3).
- `create --overwrite` of an unreadable plan archives the damaged bytes
  under `.opencode/workplan/archive/<id>/` in the same transaction and
  keeps the doctor's `recoveredPlanFile` unless `--plan-file` is given;
  other writers on an unreadable plan name that repair and its hash. New
  writes record whole-second UTC timestamps (D.3.1).

### Common

- `--root` defaults to the current directory.
- `--json` prints exactly the result text that the corresponding `workplan_*`
  tool returns (for `patch`: `{"output", "metadata"}`). Errors print
  `{"ok": false, "error": {class, message, issues?}}` with a protocol error
  class (`invalid_input`, `stale_state`, `lock_unavailable`,
  `ownership_conflict`, `recovery_required`, `external_edit_conflict`,
  `permission_denied`, `cancelled`, ...).
- Exit status: 0 success; 1 operation error, refusal, or an invalid plan for
  `validate`; 2 usage error.

### Native adapter protocol

```sh
shiori serve --stdio [--idle-timeout 10m] [--max-frame-bytes 16777216]
```

One private JSON-lines connection on stdin/stdout, started by the OpenCode
adapter (never a network listener or a daemon). stdout carries protocol frames
only; stderr carries redacted one-line diagnostics. The first request must be
`shiori.handshake`; it reports the core/protocol/contract versions, the
thirteen tool operations plus `shiori.commit`/`shiori.discard`, the platform
and write gate, observed durability facts and the frame limits. A mutating tool
request returns a `prepared` intent (exact resources, before/after digests,
expected state, a single-use capability) and touches nothing; only
`shiori.commit` of that unchanged intent, on the same connection and for the
same trusted invocation identity, writes. Cancel frames, disconnects and a
second commit expire it; nothing is replayed after a reconnect. The child exits
on stdin EOF, SIGTERM or after 10 minutes with no request and no prepared
intent. See [STATUS](docs/STATUS.md#stage-d--native-adapter-done-committed-4044539)
and `schema/v1/protocol-envelope-v1.schema.json`.

## OpenCode adapter

`adapter/opencode/` is an OpenCode V2 plugin package (verified on OpenCode
2.0.19/2.0.20 with plugin API 2.0.20). On another host version (D.3) the
read-only tools keep working, every mutating tool is refused with
`Shiori adapter not verified for OpenCode <v>; writes disabled — update
Shiori`, and `workplan_doctor` reports the host version and the verified
list. It registers the existing thirteen `workplan_*` tools with the
same descriptions, argument shapes, result text and role matrix as the
reference TypeScript plugin, and runs them through a lazily started
`shiori serve --stdio` child. It has no dependencies: it imports only `node:`
built-ins and types from `@opencode/plugin` (a peer; nothing is installed), and
there is no `node_modules`.

Install (not done automatically; enabling it in a real configuration is the
separately decided stage E):

1. Build the core: `CGO_ENABLED=0 go build -trimpath -o /abs/path/shiori ./cmd/shiori`.
2. Add the package to the OpenCode `plugins` list in place of the reference
   workplan plugin (never both: they register the same tools and plugin id):

   ```jsonc
   { "package": "/abs/path/to/shiori/adapter/opencode", "options": { "bin": "/abs/path/shiori" } }
   ```

   Instead of the `bin` option, `SHIORI_BIN=/abs/path/shiori` may be set in the
   environment OpenCode runs in. The path must be absolute; `PATH` is never
   searched and nothing is downloaded.

   Optional (D.4.2): `compactionAdvice` sets the compaction advisor
   thresholds of the spawned core (`shiori serve --stdio
   --compaction-advice …`): `"off"`, or an object with any of
   `minSavingsKiB` (default 32), `notes` (50), `terminalPercent` (25,
   at most 100), `planKiB` (192) and `keepNotes` (20, at most 10000), each
   a positive integer. Absent, the defaults apply. An invalid value fails
   plugin load with a message naming the option; it is operator
   configuration, never model input.

   ```jsonc
   {
     "package": "/abs/path/to/shiori/adapter/opencode",
     "options": {
       "bin": "/abs/path/shiori",
       "compactionAdvice": { "minSavingsKiB": 16, "notes": 30, "keepNotes": 30 }
     }
   }
   ```
3. Optional single-file build: `cd adapter/opencode && bun build ./index.ts
   --target=bun --format=esm --outfile /abs/path/shiori-opencode.js` (only
   `node:` imports remain).

Writes are authorized by the executing host's permission engine: the adapter
proves the host instance (HMAC challenge through its own RPC and the public
event stream), asks `edit` for the exact canonical resources of the prepared
intent with the trusted session/agent/message/tool-call from the native
`ToolContext`, and commits only after a definitive allow or a matching genuine
user reply. It never replies to permissions or edits rules, and the
invocation's `AbortSignal` cancels the protocol request and expires the intent.

## Test

```sh
go vet ./...
go test ./...
go test -race ./...
go test ./internal/ojson -run '^$' -fuzz '^FuzzParse$' -fuzztime 30s   # also FuzzQuote,
                                                                        # snapshot FuzzManifestJSON, engine FuzzCursorDecode
go test ./internal/engine -run '^$' -bench Ops -benchmem                 # perf fixtures, 100 KiB–10 MiB
SHIORI_BASELINE=/tmp/go-baseline.json go test ./internal/engine -run '^TestBaselineMatrix$' -v
(cd adapter/opencode && bun test)                                       # builds shiori unless SHIORI_BIN is set
```

- `internal/protocol` covers framing limits, malformed/duplicate/unknown
  keys, handshake and version refusal, cancellation before and during commit
  (before and after the journal), disconnect expiry, replay rejection, binding
  rejections, the write gate and idle exit; `internal/schematest` validates
  every frame of a real session against the envelope schema.
- The adapter tests run the real core with fakes of the OpenCode context and
  public API (P01–P07 as far as possible without a model). The opt-in runtime
  smoke starts a private OpenCode server with isolated `HOME`/XDG directories
  and its own port and never calls a model:

  ```sh
  cd adapter/opencode
  SHIORI_RUNTIME_SMOKE=1 SHIORI_SMOKE_FIXTURE=/path/to/a/project/to/copy \
    [SHIORI_REFERENCE_PLUGIN=/path/to/reference/workplan-tools] \
    [SHIORI_SMOKE_EVIDENCE=/tmp/evidence.json] bun test test/runtime-smoke.test.ts
  ```

  The fixture is copied (`cp -Rp` semantics) and verified unchanged; with
  `SHIORI_REFERENCE_PLUGIN` the same script also runs the reference plugin on
  its own server and records a parity report.

- `TestCorpusParity` runs every `tools/`, `resume/` and `paging/` vector at a
  root of the generation root's length, under `/private/tmp`, and fails on
  any byte, mode, mtime or file-set change. Vectors that D.1 changes on
  purpose are pinned in `testdata/d1/expectations.json`, and each one also
  passes a comparator against the unchanged oracle vector. Re-pin them
  after review with `SHIORI_D1_UPDATE=1 go test ./internal/engine -run TestCorpusParity`.
  Vectors that D.2 changes are pinned in `testdata/d2/expectations.json`,
  those D.3 changes in `testdata/d3/expectations.json`
  (`SHIORI_D3_UPDATE=1` re-pins; the D.3-off output must still pass every
  earlier check), and those D.3.1 changes, including mutation and Markdown
  render vectors, in `testdata/d3_1/expectations.json`
  (`SHIORI_D31_UPDATE=1`, per package; the D.3.1-off run must still pass
  every earlier check). D.4 changes would be pinned in
  `testdata/d4/expectations.json` (`SHIORI_D4_UPDATE=1`); it is empty
  because no corpus fixture qualifies for compaction advice, so every
  vector is byte-identical to D.3.1 (`TestD4ComparatorOnCorpus` checks
  the comparator with lowered thresholds; since D.4.2 every advised
  resume packet, small budgets included, is the advice-off packet plus
  the member). `testdata/d4_2/expectations.json` is empty for the same
  reason.
  The same engine with the graph additions off must still pass the
  earlier check, and the D.2 output must differ from it only by the
  approved members (`SHIORI_D2_UPDATE=1` re-pins).
- `TestMutationVectors` runs all 70 mutation vectors with a frozen clock and
  compares output, authorization count and the exact changed files;
  `TestMutationInputVectors` covers the 39 mutating-tool input vectors.
- Declared divergences (contracts §7, §10a) are checked by dedicated
  comparators; see [STATUS](docs/STATUS.md).
- `TestFaultInjection`, `TestNoSideEffectsBeforeCommit`, the `TestBarrier*`
  separate-process tests, `internal/storage` lock tests and
  `TestRecoveryRejectsForgedJournals` cover spec 05 S01–S11.
- The JSON Schema validator is a test-only dependency and is not linked into
  the binary.

`LICENSE` and `.gitignore` are retained from the repository's initial commit.

## Next decision

Review and commit D.4.2, then the queue in the
[STATUS resume point](docs/STATUS.md#resume-point-paused-after-d42): E1
(X2 evidence ledger, tool-surface proposal first), E2 (X3 worktree lanes,
proposal first), then a measured performance stage (X8–X10). Updating the
pinned copy the owner's OpenCode uses is a separate step for the owner.
