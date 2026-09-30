# Frozen contract v1 (stage A)

Status: **PROPOSED for review**, 2026-09-30. Everything in this document is a
stage A output: the reviewed schemas, the golden corpus, and a concrete proposal
for each open item in [05 §6](specs/05-migration-and-acceptance.md). Items
marked **PROPOSED** need the owner's approval before stage B depends on them.
Items marked **OBSERVED** describe what the reference implementation actually
does, as recorded in `testdata/`. Where OBSERVED and the specifications
disagree, section 7 lists the difference and proposes a resolution.

## 1. Sources of truth

| Artifact | Location | Role |
| --- | --- | --- |
| Machine schemas | [`schema/`](../schema/index.json) (JSON Schema 2020-12) | Shape contract for stored artifacts, tool inputs and protocol frames |
| Golden corpus | [`testdata/`](../testdata/MANIFEST.json) | Behavioural contract: the fixtures plus the outputs the reference produced for them |
| This document | `docs/contracts.md` | Policies the corpus cannot express, and the stage B–D decisions |
| Specifications | `docs/specs/01..05` | Requirements. Where they conflict with this document, section 7 applies until the owner decides |

The reference implementation (the "oracle") is the local TypeScript workplan
engine that `testdata/MANIFEST.json` fingerprints: the sha256 of each source
file, bun 1.4.0, zod 4.1.8, macOS 27.0.1 arm64. It was only **executed**. No
reference source was copied, ported or paraphrased into this repository. The
harness scripts that drove it are kept outside the repository, and only their
sha256 fingerprints are recorded.

## 2. Schema layout (PROPOSED)

- The dialect is 2020-12. `$id` values use `https://shiori.invalid/schema/v1/…`.
  They are identifiers only and are never fetched. Relative `$ref`s resolve
  within the directory.
- `schema/v1/common.schema.json` holds shared definitions: hash, status,
  severity, `nonblank` (trimmed with the ECMAScript whitespace set), `workplanId`,
  the UTC datetime, the manifest entry and the composite step reference.
- Stored artifacts:
  - `plan-v2`
  - `checkpoint-v1`, `checkpoint-v2` and the union `checkpoint`
  - `dependencies-v1`
  - `transaction-journal-v1`
  - `lock-owner-v1`
  - `cursors-v1`
- Protocol: `protocol-envelope-v1` (section 5.4).
- Tools: `schema/v1/tools/<tool>.input.schema.json` for all 13 native identities,
  including `workplan_compact_preview`. These are the **native** surface: they
  have no `workspaceRoot` and are strict at the top level. The standalone core/CLI
  surface also accepts `workspaceRoot`, and keeps `expectedHash` optional for
  legacy direct calls.
- Rules that JSON Schema cannot express are listed under `x-shiori-*` keywords,
  documented in `schema/index.json`. Each one carries the exact reference message
  and field path. Where a native cross-field rule *can* be expressed (required
  `expectedHash`, recovery exclusivity, apply confirmation), it is also encoded as
  `allOf`/`if-then`. With those encodings, the schemas give the same accept/reject
  result as the oracle's native parser on all 55 input-validation vectors.
- The storage schemas accept every valid fixture and reject every invalid one
  (checked with a 2020-12 validator; see MANIFEST harness fingerprints).

**Semantic validation remains mandatory.** Schema validity does not cover id
normalization, dependency existence or cycles, artifact scope, hash/token
binding, or structure rules (`x-shiori-structure-rules`).

## 3. Hash contract (OBSERVED, frozen)

Covered by the vectors in `testdata/vectors/hash/`: 11 synthetic manifests, a
snapshot of every fixture plan, and an invalidation table.

```text
planHash  = SHA256("workplan-plan-v1\n"  + M(planEntries)  + "\n")
stateHash = SHA256("workplan-state-v1\n" + M(stateEntries) + "\n")
M(entries) = compact JSON array of entries sorted by path
entry = {"path":P,"sha256":H} | {"path":P,"missing":true}   (key order exactly so)
```

- **Membership.** The plan manifest contains the primary JSON, the linked
  Markdown and every linked spec. The state manifest contains the plan entries
  plus `<id>.checkpoint.json`, `<id>.dependencies.json` and
  `<id>.transaction.json`, whether each is present or missing. Lock, stage, temp,
  unrelated and archive files never contribute. A change to mtime only does not
  change either hash (`hash/invalidation`).
- **Ordering.** The sort key is the JavaScript comparison of UTF-16 code units.
  It is neither byte order nor code-point order. For example, `docs/𝒜.md`
  (surrogate pair D835) sorts **before** `docs/Ａ.md` (FF21) and `docs/.md`
  (E000). Go must compare `utf16.Encode([]rune(p))` or use an equivalent
  comparator. It must not use `sort.Strings`.
- **Escaping.** `M` uses ECMAScript `JSON.stringify` string escaping:
  - `"` and `\` are escaped.
  - U+0008, U+0009, U+000A, U+000C and U+000D are escaped as `\b \t \n \f \r`.
  - Every other code point below U+0020 is escaped as `\u00xx`, lowercase hex.
  - Everything else is emitted raw as UTF-8, including `< > &`, U+007F,
    U+2028/U+2029 and non-BMP characters.

  Go's `encoding/json` default escaping (`<>&`, U+2028/9) would break the hash.
  The proposed encoder is Go 1.27's `encoding/json/jsontext` string quoting,
  which was checked locally to leave `<>&`, U+2028 and U+007F raw. The vectors
  (`canonicalManifestJsonUtf8Hex`) remain the arbiter.
- **Paths.** Paths are project-relative and use `/` separators.
  **OBSERVED:** a backslash inside a POSIX filename is also rewritten to `/`
  (section 7, D5).
- The hash helpers hash whatever key order they are given. Only the canonical
  entry order above is contractual.
- A pending journal's bytes are part of `stateHash`, and a missing primary JSON
  is recorded as a missing entry.

Cursor checksums use the same style of preimage:
`SHA256("<domain>\n" + compact JSON)`, with the key order given in
`cursors-v1.schema.json`. The token is unpadded base64url.

## 4. Other frozen formats (OBSERVED)

- **Generated Markdown** must be byte-exact.
  - `testdata/vectors/markdown/` holds 11 renderings and 2 error cases.
  - `markdown/generated-classification` records which fixture Markdown counts as
    "generated": stored Markdown that byte-equals the rendering of the stored
    JSON.
  - The renderer treats absent `specFiles` as `[]`. The reference renderer
    throws on a raw legacy document; the tools only avoid this because the
    snapshot normalizes the document first.
- **Stored JSON** is `JSON.stringify(doc, null, 2) + "\n"`: pretty-printed with
  2 spaces, the same escaping as in section 3, and a single trailing newline.
- **Resume budget unit.** `maxChars` counts **UTF-16 code units** of the complete
  output text (JavaScript `string.length`). Go must count
  `len(utf16.Encode([]rune(s)))`, not bytes and not runes.
  - Every resume vector records `rawOutputLength` in this unit.
  - All 111 successful resume outputs (single-call and paged) fit their budget.
    The other 34 resume vectors are expected errors (invalid fixtures, cursor
    misuse).
  - The minimum-budget stress case produced 4025 of 4096 code units.
- **Datetimes.** The UTC form `YYYY-MM-DDTHH:MM[:SS[.frac]]Z` is accepted.
  Offsets are rejected and seconds are optional
  (`validation/structure/datetime-*`).
- **Journal content** fields hold exact bytes as padded standard base64. `mode`
  is the POSIX permission bits.

## 5. Resolutions for 05 §6 ("choose/freeze before implementation")

### 5.1 Schema and binding tooling: PROPOSED

1. The JSON Schema files in `schema/` are the single source of truth. There is
   no code generator in stages B–D. Go types are hand-written against the
   schemas, and a Go test loads every schema and every fixture/vector to prove
   agreement. Candidate validator for tests only:
   `github.com/santhosh-tekuri/jsonschema` (version pinned at stage B). It is a
   test dependency and is never linked into the shipped binary.
2. Decoding uses `encoding/json/jsontext` (standard library, Go 1.27, no
   experiment flag).
   - It rejects duplicate member names by default, which is required for
     protocol frames and native input.
   - It gives token-level access for preserving unknown members and number
     spellings.
   - `encoding/json` v1 struct decoding is not used for artifacts, because it is
     case-insensitive and silently accepts duplicates.
3. The adapter reads the tool schema files at build time. The adapter build
   copies them verbatim into its single file. It registers them unchanged with
   OpenCode, which is how it avoids hand-duplicated property maps. Input
   validation stays in Go, and its error text is returned unchanged.
4. Schema changes bump the directory version (`schema/v2/…`). Version v1 is
   frozen once the owner approves it.

### 5.2 Limits: PROPOSED (configurable, benchmark-revisited in stage F)

| Limit | Default | Notes |
| --- | --- | --- |
| Request frame | 16 MiB | Rejected before decoding into values |
| Response frame | 64 MiB | See D9. `workplan_read` output for a 10 MiB plan measured 30.1 M chars |
| Single artifact read | 64 MiB | Opt-in raise via CLI flag or config only, never via model input |
| Whole snapshot (JSON + MD + specs + sidecars) | 256 MiB | Fails closed as `unsupported_capability` |
| Linked spec files per plan | 1024 | |
| JSON nesting | 128 | Applies to frames and artifacts |
| Cursor token | 4096 bytes | Larger tokens fail with the reference "Invalid … cursor" text |
| Resume `maxChars` | 4096–64000, default 12000 | Unchanged |
| Resume / doctor / inspect `limit` | 1–100 default 20 / 1–100 default 50 / 1–500 default 100 | Unchanged |
| Lock wait / abandonment grace | 5 s / 5 min | Unchanged; age alone never reclaims |

### 5.3 Packaging and distribution: PROPOSED

- **Core.** A single static Go binary named `shiori`, built with `CGO_ENABLED=0`,
  `-trimpath`, reproducible flags, and the Go module
  `github.com/hoshinoht/shiori`. It has no runtime dependencies: no Bun, Node or
  libc requirement on Linux.
- **Adapter.** One dependency-free ES module (`adapter/shiori-opencode.js`, built
  from TS). It is npm-free: no `package.json` dependencies and no
  `node_modules`. It imports only OpenCode's plugin API and `node:` built-ins.
  - It locates the binary from an explicit plugin option or `SHIORI_BIN`. It
    never searches `PATH` implicitly, and never downloads or installs anything.
  - It refuses to start if the handshake's `contractVersion` or
    `protocolVersion` is unsupported.
- **Distribution.** Stages B–D use local build only (`go build`). Release
  artifacts (GitHub release, per-platform binaries plus `SHA256SUMS`) belong to
  stage E, and are neither published nor automated before that.

### 5.4 Process lifecycle: PROPOSED

- The adapter lazily spawns **one** `shiori serve --stdio` child per plugin
  instance on the first tool call. There is no daemon shared across projects or
  hosts, and no network listener.
- The first frame is `shiori.handshake`. Requests are multiplexed by
  `requestId`.
- A tool request that would mutate returns `prepared` and touches nothing. After
  host approval, `shiori.commit` commits exactly that intent (spec 02 §3).
- An `AbortSignal` sends a `cancel` frame. A late approval after cancellation is
  refused.
- **Child crash or transport loss.** All in-flight requests fail with
  `cancelled` or `outcome_uncertain`, plus the journal pointer when relevant.
  Nothing is replayed. The next call respawns the child and repeats the
  handshake, and prior approvals do not carry over.
- **Idle exit.** After 10 minutes with no request, the child exits.
- **Host unload.**
  1. Cancel in-flight requests.
  2. Close stdin.
  3. Wait 2 s, then send SIGTERM to the owned PID only.
  4. Wait 2 s more, then send SIGKILL.
  5. Keep any journal.
- **One-shot CLI.** `shiori <operation> …` runs the same engine in-process, with
  no child and no protocol.

### 5.5 Standalone CLI mutation confirmation: PROPOSED

The CLI acts under OS/local operator authority, not OpenCode policy (spec 02 §7).

- Read operations never prompt.
- Every mutation first prints the prepared intent: operation, exact resources,
  and before/after hashes.
  - On a TTY it requires typing `yes`.
  - Off a TTY it requires `--yes`.
- `--yes` cannot come from an environment variable or config file, and never
  bypasses stale-hash, lock, journal or scope checks.
- Existing-state writes require `--expected-hash` (native parity) unless
  `--legacy-unhashed` is given. That flag still re-checks state under the lock.
- `compact --apply` additionally requires `--preview-token` and
  `--confirm ARCHIVE_SELECTED_HISTORY`.
- Recovery takes `--recovery resume|rollback` and `--expected-hash`, and
  excludes every ordinary update flag.
- `--json` emits the protocol result object on stdout, and nothing else.

### 5.6 Platform and host matrix: PROPOSED

| Target | Read ops | Writes | Evidence required before claiming |
| --- | --- | --- | --- |
| darwin/arm64 (APFS, local) | yes | yes | Stage C fault, race and recovery suites on macOS |
| linux/amd64 (ext4/xfs, local) | yes | yes | Same suites on Linux; directory fsync verified |
| darwin/amd64, linux/arm64 | build-only | no | Unsupported until run through the gates |
| Windows, network/FUSE filesystems | no | no | Separate atomicity/locking design (spec 01 §6) |

- The minimum host is OpenCode runtime 2.0.19 with plugin/client 2.0.20. This is
  the verified baseline; other versions fail closed at registration.
- Handshake `durability` reports observed capabilities, never intent.
- On a system that is not a supported write target, the handshake reports
  `writeSupported: false`, and mutations fail with `unsupported_capability`.

## 6. Unknown-field preservation and error text

### 6.1 Unknown fields: PROPOSED policy, with OBSERVED reference behaviour

**Stored plan JSON (document, phase, step, finding).** Unknown members must
survive unrelated writes with their values and nesting intact. The reference
does this, but lossily (`mutations/update-legacy-preserve`):

- Known keys are re-emitted in schema order, with unknown keys after them, at
  every level.
- Duplicate keys collapse to the last value.
- Integers beyond 2^53 lose precision (`12345678901234567890` becomes
  `12345678901234567000`).
- `1.0` becomes `1`.

Proposal (see D4): Go preserves each unknown member's raw JSON bytes and its
number spelling, and keeps the reference's key order so ordinary documents stay
byte-identical. Duplicate keys in a stored plan are **diagnosed** instead of
collapsed: the plan stays readable and mutations fail closed with a field path.

**Sidecars** (checkpoint, dependencies, journal). Unknown members are accepted
and ignored on read, and are not carried forward, because sidecars are replaced
wholesale. This matches the reference.

**Native tool input.** Unknown keys are rejected at **every** level (spec 02
§7). The reference rejects them at top level only, and silently strips them in
nested phase/step/finding/patch objects (D2). The core/CLI surface follows the
same rule.

### 6.2 Error text policy: PROPOSED

1. The reference's own messages must match byte-for-byte after `$ROOT`
   substitution. These are the messages listed in the behavioural contract and in
   the corpus: patch grammar, plan-file policy, handwritten refusal, cursor,
   recovery, stale-hash and structure-rule texts.
2. Schema-library messages must also match byte-for-byte. The corpus contains
   this finite template set, and Go reproduces exactly this set with the same
   dot-joined paths:
   - `Invalid input: expected <T>, received <T>`
   - `Invalid input: expected <literal>`
   - `Invalid option: expected one of "a"|"b"`
   - `Invalid string: must match pattern /[a-z0-9]/i`
   - `Invalid string: must match pattern /^[a-f0-9]{64}$/`
   - `Too small: expected number to be >=N`
   - `Too small: expected string to have >=1 characters`
   - `Too big: expected number to be <=N`
   - `Unrecognized key: "k"`

   Multiple issues are joined with `"; "`, in schema property order, and wrapped
   as `Invalid <tool> input: …`.
3. JavaScript-engine text is **not** reproduced. Examples:
   `JSON Parse error: Expected '}'` and
   `Property name must be a string literal`. Go keeps the stable prefix
   (`Invalid workplan JSON at <path>: `,
   `Invalid workplan checkpoint JSON at <path>: `) and appends its own detail.
   Comparators match on the prefix only. This is a declared presentation
   difference.
4. Protocol errors add `class`, `issues[]`, `currentStateHash` and `retrieval`
   (see `protocol-envelope-v1`). The `message` field carries the text defined by
   rules 1–3.

## 7. Oracle behaviours that differ from the specifications

Each item cites the vectors that show it. "Proposal" is what stage B/C
would implement if approved.

| # | Observed reference behaviour | Spec / requirement | Proposal |
| --- | --- | --- | --- |
| D1 | **A journal left before the primary JSON exists cannot be recovered through any tool.** `read`, `validate`, `resume`, `checkpoint` and `update {recovery}` all fail with `Workplan file not found: $ROOT/.opencode/workplan/tx-new.json`. `create` prompts for permission and then fails with `Refusing to overwrite existing workplan artifact: …tx-new.transaction.json`. `doctor` lists the journal (`valid: true`, `targetCount: 2`), but gives **no stateHash**, only the issue `Primary workplan not found: tx-new`. (`mutations/update-recovery-precreate-resume`, `mutations/create-over-precreate-journal`, `tools/pending-journal-precreate/*`) | 01 §6 and S09 require a read-only interrupted-state hash and a recovery route | Go `doctor` (and `read` with `recoveryRequired`) returns `stateHash` computed with the primary recorded as missing. `update {recovery}` accepts a journal-only plan and validates the journal before authorization. New Go vectors are needed, because the oracle cannot produce them |
| D2 | Unknown keys in nested input objects are silently stripped (`validation/input/create--unknown-nested-phase-key`). Top-level unknown keys are rejected | 02 §7: unknown native input fields reject | Reject at every level with `Unrecognized key` at the nested path. Declared divergence |
| D3 | The registered tool JSON Schema is weaker than the validator. The id pattern loses the case-insensitive flag (`[a-z0-9]`, so `ABC` fails the registered schema but passes the validator: `create--id-uppercase-only`). Trimmed-nonempty becomes `minLength: 1`. Required `expectedHash`, recovery exclusivity and the apply rules are absent | 02 §7: schemas are the common contract | `schema/v1/tools` fixes all of these (`[A-Za-z0-9]`, `nonblank`, `allOf` conditions) |
| D4 | Unknown-metadata preservation is lossy: key reordering, duplicate collapse, loss of large-integer precision, and `1.0` rewritten as `1` (`mutations/update-legacy-preserve`) | 01 §3, C01: unknown metadata survives unrelated writes | Section 6.1. Divergence is limited to documents that the reference would already damage |
| D5 | A manifest path turns a literal `\` in a POSIX filename into `/`. `docs/quote"back\slash.md` is recorded as `docs/quote"back/slash.md` (`hash/fixtures/unicode--unicode-plan`), which could collide with a real `docs/quote"back/slash.md` | 01 §4/§5: manifest identity is the relative path | Keep the conversion for hash parity. Reject spec/plan paths containing `\` on POSIX with a field-path diagnostic, so no ambiguous manifest can be created. Existing plans with such paths stay readable, and Go reports an issue |
| D6 | `workplan_list` sorts with `localeCompare` (ICU, locale `en-US` here; the result depends on the locale). Sidecars come back in raw `readdir` order, which depends on the filesystem (`tools/list-mixed/list`) | 01 §5: no dependence on locale or map order | For canonical ids (`[a-z0-9-]`) UTF-16 order equals the observed order, checked on this corpus. Go sorts plans by UTF-16 code units and sidecars by name. The only visible difference is the position of non-canonical invalid entries (`Bad Name`, `UPPER`), which becomes a declared presentation difference; vectors compare invalid entries as a set |
| D7 | Any `update` of a plan whose phase/step id normalizes to empty fails with `Workplan id must contain at least one letter or number`, with no field path, even for unrelated fields. The generated-Markdown check renders markers before any other check (`mutations/update-duplicate-step-ids` on `invalid-structure`) | 01 §3: exact field paths, and drafts stay diagnosable | Go reports `phases.<i>.id: Must not be empty` (or the step path) and refuses the mutation. The message changes; the refusal is the same |
| D8 | Markdown markers are lossy for non-ASCII ids: `phase-𝒜-x` becomes `phase-x` and `step-été` becomes `step-t`, so distinct ids can share a marker (`tools/unicode/*--inspect`) | 03 §4: marker/heading index used for section retrieval | Keep the marker format (byte parity). The Go section index treats a marker collision as ambiguous and falls back to the heading plus ordinal, never "first match wins" |
| D9 | `workplan_read` output has no bound. For the 1 MiB plan it is 3.0 M chars; for 10 MiB it is 30.1 M chars (baseline) | 02 §2: bounded frames | Response frame 64 MiB (section 5.2). A larger read fails with `unsupported_capability` and a pointer to `includeMarkdown=false`, `inspect` or `resume`. Declared divergence only above the limit |
| D10 | The compact `previewToken` and `archivePath` depend on the absolute canonical root. The same preview at a different root path gives a different token (`mutations/compact-apply-valid`) | 01 §7 lists what the token binds; the root is not named | Accept and document: the token also binds the canonical root. No change |
| D11 | Engine- and library-specific message text leaks into diagnostics (the JSC JSON parser, zod) | 01 §3 exact paths; 02 §7 error classes | Section 6.2 |
| D12 | `reset {mode: "markdown-only"}` on already-generated Markdown asks for permission and "commits" identical bytes, so no file changes (`mutations/reset-markdown-only-generated`) | S03-adjacent: avoid needless authorization | Go returns an unchanged result without preparing an intent. Declared difference: zero prompts |

The remaining checks found no differences: hash algorithm, checkpoint
freshness classes (`missing`, `fresh`, `stale`, `legacy-unverified`, `invalid`),
cursor stale/tamper rejection, pre-authorization rejections with zero prompts,
handwritten-Markdown protection, and read-only operations leaving bytes and
mtimes unchanged (checked for every read vector).

## 8. Corpus layout and vector format

```text
testdata/
  MANIFEST.json            oracle fingerprints, runtime, normalization rules, per-file sha256
  fixtures/<name>/         a complete workspace root (.opencode/workplan/..., docs/...)
  vectors/hash/            manifest-hash cases, per-fixture snapshots, invalidation table
  vectors/markdown/        render inputs + <case>.expected.md bytes; generated classification
  vectors/validation/      structure/ (document -> issues) and input/ (tool input -> core/native parse)
  vectors/tools/<fixture>/ read, inspect, validate, doctor, list outputs (+ filtered/error cases)
  vectors/resume/<fixture>/ resume at maxChars 4096 / 12000 / 64000 (exact outputText)
  vectors/paging/          inspect and resume cursor chains, cursor misuse errors
  vectors/mutations/       pre-authorization rejections and frozen-clock successes (+ <case>.after/ bytes)
  perf/                    baseline fixtures (100 KiB raw; 1 MiB and 10 MiB as .tar.gz)
```

Every vector JSON has `id` and `fixture` (or an inline `input`), `call`,
`expect`, and `deterministic`. Read vectors also record
`readOnly.bytesAndMtimesUnchanged`. Mutation vectors record `changedFiles` and
`permissionPrompts`.

To run a vector, a Go test copies the fixture to a temporary root, invokes the
operation, and replaces the root with `$ROOT`. Budget-sensitive vectors (resume
and paging) must use a root of the same length as `generationRoot`, or check
invariants instead of bytes: the budget is met, IDs and hashes are present, and
the omission counts are truthful. Generated IDs, transaction IDs and stage-file
nonces are not contractual beyond their format.

Counts:

- 21 fixtures, 79 fixture files.
- 567 vector JSON files, 624 vector files including expected Markdown and
  after-bytes:
  - hash 31
  - markdown 14
  - validation 92 (structure 37, input 55)
  - tools 199
  - resume 87
  - paging 74
  - mutations 70
- 1 vector (`mutations/compact-apply-valid`) is deterministic only at a fixed
  root (D10).

To regenerate, rerun the out-of-repo harness against an oracle whose file
fingerprints equal `MANIFEST.oracle.files`. A fingerprint mismatch invalidates
the corpus, and requires review before replacement.

## 9. Open for the owner

1. Approve or amend the PROPOSED items in sections 2, 5 and 6, and the proposals
   D1–D12.
2. Confirm that shipping ~1 MB of compressed performance fixtures in git is
   acceptable. The alternative is generator parameters only, with the fixture
   sha256s recorded in `docs/baseline.md`.
3. Confirm the validator test dependency (5.1). Stage B adds `go.mod` and must
   not add it without approval.
