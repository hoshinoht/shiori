# Shiori

**Shiori (しおり, “bookmark”)** is a proposed local-first work coordination
engine: preserve the plan, execution state, safety boundaries and the next
reliable action across workers and sessions.

This repository contains the specifications, the approved stage A contract
(machine schemas, a golden fixture corpus and a measured TypeScript baseline),
the stage B read-only Go core, the stage C transactional core with its
`shiori` CLI, and the stage D native OpenCode adapter (`shiori serve --stdio`
plus `adapter/opencode/`). The architecture is a Go core and CLI with a thin
OpenCode JS/TS adapter. Stages A–D are authorized; enabling the adapter in a
real OpenCode configuration (stage E), automatic artifact migration, commits
and remote publication are not.

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

```sh
shiori list     --root /path/to/project
shiori read     <id> [--phase ID] [--step ID] [--no-markdown]
shiori inspect  <id> [--phase ID] [--limit 1-500] [--cursor TOKEN]
shiori validate <id>                       # exit 1 when the plan is invalid
shiori resume   <id> [--max-chars 4096-64000] [--limit 1-100] [--cursor TOKEN] [--phase ID] [--step ID]
shiori doctor   [id] [--limit 1-100]
shiori compact  <id> --reason R [--archive-phase ID]... [--archive-note I]... [--archive-finding I]...   # preview only
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
shiori reset      my-plan --expected-hash "$H" [--mode draft|markdown-only] [--preserve-notes] [--replace-markdown]
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
intent. See [STATUS](docs/STATUS.md#stage-d--native-adapter-done-uncommitted-for-owner-review)
and `schema/v1/protocol-envelope-v1.schema.json`.

## OpenCode adapter

`adapter/opencode/` is an OpenCode V2 plugin package (verified on OpenCode
2.0.19/2.0.20 with plugin API 2.0.20; other host versions fail closed at
registration). It registers the existing thirteen `workplan_*` tools with the
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
  any byte, mode, mtime or file-set change.
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

Review and commit stage D (see [STATUS](docs/STATUS.md)). The next stage is
E, the opt-in switch of one real OpenCode configuration from the reference
plugin to this adapter, with rollback; its exact change and prerequisites are
in STATUS. Worktree orchestration and alternative storage remain later,
explicitly gated stages.
