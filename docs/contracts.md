# Frozen contract v1 (stage A)

Status: **APPROVED 2026-09-30** by the owner (section 9 records the approval).
Everything in this document is a stage A output: the reviewed schemas, the
golden corpus, and a resolution for each open item in
[05 §6](specs/05-migration-and-acceptance.md). Items marked **APPROVED
2026-09-30** were proposals at stage A and are now binding. Items marked
**OBSERVED** describe what the reference implementation actually does, as
recorded in `testdata/`. Where OBSERVED and the specifications disagree,
section 7 lists the difference and the approved resolution. Section 10 records
stage B findings (reference behaviour the corpus pins down that stage A did not
spell out). Sections 11 and 12 record the approved D.1 and D.2 design
changes, which deliberately depart from the reference.

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

## 2. Schema layout (APPROVED 2026-09-30)

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

### 5.1 Schema and binding tooling: APPROVED 2026-09-30

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

### 5.2 Limits: APPROVED 2026-09-30 (configurable, benchmark-revisited in stage F)

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

### 5.3 Packaging and distribution: APPROVED 2026-09-30

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

### 5.4 Process lifecycle: APPROVED 2026-09-30

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

### 5.5 Standalone CLI mutation confirmation: APPROVED 2026-09-30

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

### 5.6 Platform and host matrix: APPROVED 2026-09-30

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

### 6.1 Unknown fields: APPROVED 2026-09-30 policy, with OBSERVED reference behaviour

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

### 6.2 Error text policy: APPROVED 2026-09-30

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
| D10 | The compact `previewToken` and `archivePath` depend on the absolute canonical root. The same preview at a different root path gives a different token (`mutations/compact-apply-valid`) | 01 §7 lists what the token binds; the root is not named | **Superseded at stage C (owner):** the token and archive name must not depend on the absolute root. Shiori's token binds workplan id, stateHash, reason, exact selection, removals digest and Markdown treatment (§10 stage C) |
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

## 9. Owner decisions (APPROVED 2026-09-30)

The owner approved, on 2026-09-30:

1. Every item previously marked PROPOSED in sections 2, 5 and 6, and the
   proposed resolutions D1–D12 in section 7. They are bug fixes to the
   original workplan design, applied in the narrowest way: the V2
   plan/checkpoint/dependencies/journal formats, the `.opencode/workplan/`
   layout, the thirteen `workplan_*` identities, their argument shapes, the
   hash algorithm and the generated Markdown stay unchanged. A fix that would
   need a design change is not implemented; it is listed in
   [STATUS.md](STATUS.md) instead.
2. Keeping the compressed performance fixtures (`testdata/perf/*.tar.gz`,
   ~1 MB) in git.
3. Adding `go.mod` (module `github.com/hoshinoht/shiori`) and the test-only
   JSON Schema validator `github.com/santhosh-tekuri/jsonschema/v6` (pinned
   v6.0.3; its `golang.org/x/text` requirement comes with it). Only
   `_test.go` files import it, so it is never linked into the `shiori`
   binary. Its regexp engine is Go RE2 with `\uXXXX` escapes translated;
   no further dependency.

### Stage C owner decisions (2026-09-30)

4. D1's doctor addition (`{id, valid:false, issues, stateHash,
   recoveryRequired:true}` for a journal-only plan) is accepted, and
   `update {recovery}` accepts that `stateHash` as its `expectedHash`.
5. D10 is changed from "accept and document" to a fix: the compact preview
   token and the archive path must not depend on the absolute project root.
6. The native OpenCode host authorizer is stage D; stage C provides the
   `Authorizer` interface and the standalone CLI implementation (§5.5).

### Stage D owner decision (2026-09-30)

7. **Accepted divergence — resume display-cap residual.** For plans with
   very many truncated strings (the `resume-stress` fixture), the Go resume
   packet chooses a different display-string cap than the reference on some
   budgets (88 of 154 sampled; §10a item 1). Every packet still fits its
   budget and keeps all machine ids, hashes, counts and retrieval pointers
   (R01/R02). This is accepted as-is; resume behaviour is not changed.
   *Superseded by D.1 (§11 item A), which replaces the display-cap ladder.*

## 10. Stage B findings (OBSERVED, pinned by the corpus)

These reference behaviours were not written down at stage A. The Go core
reproduces them byte-for-byte; the vectors named are the evidence.

- **Tool output text** is `JSON.stringify(result, null, 2)` for every tool
  except `workplan_resume`, whose packet chooses its own format (below). Every
  `outputSha256` in `tools/`, `resume/` and `paging/` is the SHA-256 of that
  text after `$ROOT` substitution.
- **Pending journal short-circuit.** `read`, `inspect` and `resume` on a plan
  with `<id>.transaction.json` return only
  `{recoveryRequired, journalPath, planHash, stateHash}`; `read` adds
  `"workplan": null` and `resume` adds `"planFresh": false`
  (`tools/pending-journal/*`, `resume/pending-journal/*`).
- **Resume cursor filters.** The resume cursor stores `phaseId`/`stepId` as
  `"sha256:" + hex(SHA-256(id))`, not the raw id; the inspect cursor stores the
  raw `phaseId` (`paging/resume-big-phase2-step20`, `paging/inspect-big-*`).
- **Resume budget policy.** A packet is built with a pinned-list cap *L*, a
  display-string cap *C* (UTF-16 code units, the last one being `…`, never
  splitting a surrogate pair) and a page size, and the first candidate whose
  complete text fits `maxChars` is returned:
  1. *L* = 4 when `maxChars` is 4096 and 8 at 12000 and 64000. The cut-over
     between those budgets is not covered by the corpus; Go uses 8192.
  2. For *C* in 512, 256, 128, 64, 32, 21, 10, 5, 2, 1: pretty
     (`JSON.stringify(v, null, 2)`), then compact, with the full page.
  3. Then, at *C* = 1, the page shrinks one item at a time (pretty, then
     compact).

  The corpus forbids a cap in 11–20, 22–31 or 33–58 (a packet at such a cap
  would have fit and been chosen), and requires 32, 21, 10, 2 and 1; 256, 128,
  64 and 5 are unconstrained and follow the halving pattern. `truncatedFields`
  lists danger fields first, then the rest, in packet order, capped at 4·*L*.
  All 87 `resume/` and 74 `paging/` vectors reproduce exactly.
  *D.1 (§11 item A) replaces this policy; 35 of those vectors now differ on
  purpose and are pinned in `testdata/d1/`.*
- **Checkpoint diagnostics.** A checkpoint that is JSON but matches neither
  version is `checkpoint: : Invalid input` in doctor and
  `Invalid workplan checkpoint document at <path>: : Invalid input` in resume
  (`tools/list-mixed/a-plan--doctor`, `resume/list-mixed/a-plan--*`).
- **Doctor filtering.** `doctor {id}` keeps `planCount` for the whole
  directory, filters plans by exact file name against the normalized id, and
  lists every sidecar, lock and journal (`tools/list-mixed/UPPER--doctor`).
- **Case-insensitive lookup is inherited from the filesystem.** On APFS,
  `read UPPER` normalizes to `upper` and opens `UPPER.json`; the vectors
  record that. The same call on a case-sensitive Linux filesystem reports
  `Workplan file not found`.

## 10a. Stage B open-issue decisions and stage C findings

Recorded at stage C. Everything here keeps the original workplan design
(formats, layout, tool identities, argument shapes, hash algorithm and
generated Markdown). "Measured" means the reference was executed as a black
box on copies of the corpus fixtures; its source was not read.

**Stage B open issues.**

1. *Resume caps between the corpus budgets* (measured). The pinned-list cap
   is `clamp(floor(maxChars/900), 4, 8)`; the listed `truncatedFields` cap
   is `min(floor(maxChars/256), 32)`; the display-string cap ladder is
   512, 256, 128, 64, 32, 21, 10, **4**, 2, 1 (the reference uses 4, not 5).
   Also measured and fixed: `safety.overflow` is true when any display
   field was truncated (not only danger fields); `checkpoint.recentValidation`
   truncations count as danger fields; `checkpoint.summary` is truncated
   before the current position. With these, every sampled budget
   (4096–12000, step 37/53, plus 15000–64000) of the large-paging and
   full-valid fixtures and the owner's real plan is byte-identical, and the
   corpus still passes. **Residual:** for a plan with very many truncated
   strings (resume-stress) the reference chooses display caps that are not
   a fixed ladder (3, 7, 8, 16 observed); 88 of 154 sampled budgets pick a
   different display cap. Every packet still fits its budget with all
   machine ids, hashes, counts and pointers intact (R01/R02 hold).
   **Accepted as a divergence by the owner on 2026-09-30 (§9 item 7).**
2. *Resume cannot fit (very long machine ids).* Kept as a fail-closed error:
   machine ids are never truncated. Writers cannot create such ids (every
   create/update id is normalized to at most 80 code units), so the case is
   limited to hand-edited plans. No format change.
3. *D1 recovery.* Done (owner decision 4). The expectedHash of a journal-only
   plan is the doctor hash: `workplan-state-v1` over the missing primary,
   the journal's in-root targets and the id's sidecars. Recovery reports
   `planHash`/`stateHash` of the recovered state (for a rolled-back create:
   the same interrupted-plan definition, primary missing).
4. *D4/D5 in mutation preparation.* Done. A stored plan with repeated
   member names is readable, but every writer refuses it before
   authorization: `Workplan <id> has duplicate JSON member names: <paths>.
   Remove the duplicates before mutating the plan.` New `planFile`,
   `specFiles` or `addSpecFiles` links containing `\` are refused with
   `<field>.<i>: Linked path contains a backslash and has an ambiguous
   manifest identity: <raw>`.
5. *Mutating vectors and compact token parity.* Done: the 39 mutating input
   vectors, 70 mutation vectors and 2 compact-preview vectors run in CI
   (STATUS has the table). The token is compared as an opaque value bound to
   its inputs (D10 fix). The reference's `removals.digest` preimage could
   not be recovered by black-box probing (it is root-independent and
   depends only on the removed content); Shiori defines
   `SHA256("workplan-compact-removals-v1\n" + compact JSON of the removed
   object + "\n")`, and the token is `"v1-" + SHA256("workplan-compact-token-v1\n" +
   compact JSON {workplanId, stateHash, archiveReason, canonicalSelection,
   removalsDigest, linkedMarkdownTreatment} + "\n")`. The `v1-` prefix and
   64-hex shape, the archive name `state-<stateHash:12>-<token:12>.json` and
   the transaction id `<token:16>` are unchanged.
6. *Case-insensitive filesystems.* Ids are normalized to lowercase, so plan
   and sidecar names never differ by case. Linked Markdown ownership and
   pending-journal claims compare case-folded paths, and journal validation
   rejects case-folded duplicate targets, so aliasing links fail closed on
   every platform (stricter than necessary on case-sensitive Linux, never
   weaker on APFS). Reads still inherit the filesystem's lookup (§10).

**Stage C findings.**

- *Pre-authorization rejection.* The reference detects several refusals
  only under the lock, after prompting: an existing sidecar or pending
  pre-create journal on create, a move onto an existing file, destinations
  owned by another plan or claimed by a pending journal, and recovery
  third-state edits. Shiori checks them during preparation (same message,
  no authorization request, S06/S07) and again under the lock.
- *Unchanged results.* When a mutation would not change any byte (D12, and
  the same principle for any writer), no intent is prepared and no
  authorization is requested; the result is identical.
- *Plan-file extension.* The linked Markdown must end in lowercase `.md`
  on read and write (the reference rejects `.MD`; stage B accepted it).
- *Dependency validation text* (measured): `dependencies.<i>: Source step
  <p>/<s> does not exist`, `dependencies.<i>: Duplicate dependency source
  <p>/<s>`, `dependencies.<i>.dependsOn.<j>: Duplicate dependency`; a
  replacement drops existing `terminalSummaries`.
- *Compaction* (measured): only generated Markdown is refreshed and listed
  in the intent; preserved Markdown is not a target. Apply prunes dependency
  entries whose source was archived and records terminal summaries for
  archived prerequisites still referenced; the refreshed checkpoint keeps
  its fields, appends the archive path to `references` (last 10 kept) and
  writes `planHash`/`manifest`/`evidenceStatus` last.
- *Lock protocol.* Lock files keep the reference names and lock-owner-v1
  content. Shiori publishes owner metadata atomically (stage file + link),
  reclaims only proven-dead same-host owners past the 5-minute grace under a
  reclaim mutex, and releases by rename + nonce check (a replacement owner's
  lock is restored, never unlinked). Its auxiliary paths differ from the
  reference's (`<lock>.<tx>.stage`, `<lock>.reclaim.*`), so compact-preview
  `writeIntent.resources` lists different lock-protocol paths; all other
  resources match. Mixed TS/Go writers on one root are not proven
  interoperable and must not be run together (stage E, spec 05 §3).

## 11. Approved design changes D.1 (APPROVED 2026-09-30)

A real-use round on the owner's 13-phase/38-step roadmap found five
problems that every earlier stage had reproduced faithfully from the
reference design. The owner approved changing that design on 2026-09-30.
Everything not listed here stays as in sections 1–10: the V2
plan/checkpoint/dependencies/journal formats, the `.opencode/workplan/`
layout, the thirteen `workplan_*` identities, the hash algorithm, the
generated Markdown bytes and every other argument shape. The only schema
change is the new optional `includeNotes` boolean on `workplan_read`.

The oracle corpus is not edited. Vectors whose output changes on purpose
are listed in [`testdata/d1/expectations.json`](../testdata/d1/expectations.json)
with the SHA-256 and UTF-16 length of the pinned Go output, and each one
passes a comparator that proves only the approved change differs
(`internal/engine/d1_vectors_test.go`). There are 56 such tool vectors
(A: 35 resume/paging, B: 3 filtered reads, C/F: 18 validate/doctor) and
one mutation vector (`mutations/patch-validate`, item D).

**A. Resume budgeting prefers fewer items to shorter text.** The reference
shortened every display string (down to one code unit) before it
returned fewer page items. On the roadmap, the default 12000 budget gave
20 items whose every string was cut to 32 code units, and 6000 gave one
code unit. A first D.1 version returned fewer items before any shortening,
which gave 2 of 44 items at 12000 and was too few for surveying work; the
coordinator's review on 2026-09-30 rebalanced it to a target page. The
order (see `chooseResume` in `internal/engine/resume.go`):

1. **Target page** of min(limit, *T*) items: *T* = 8 at `maxChars` ≥ 12000,
   4 at ≥ 6000, 2 below. While the target still fits, text shrinks first
   through the tiers `{prose cap, title cap}`: uncapped, {2048, 512},
   {1024, 256}, {512, 200}, {240, 80}, then the floor {120, 80}. Each tier
   returns the largest page (at least the target) that fits. Current work
   (checkpoint summary, next action, current step
   target/action/validation) keeps a cap of at least 512 at these levels.
   Pretty output is preferred unless compact output carries more items.
2. **Below the target**, at the floor the page shrinks to one item (the
   rest stays reachable through `nextCursor`). Then current work drops to
   the floor. Then the pinned lists (scope, non-goals, constraints,
   blockers, guardrails, references, recent validation, relevant files,
   warnings, high findings, dependencies) show fewer entries, down to one
   each. Every total and `omittedDangerCounts` entry that the packet has
   stays exact, and `safety.overflow` is set. Recent validation and
   relevant files have no total in the packet (shape unchanged); the
   relevant files stay reachable as page references.
3. Only when one page item (or the pinned packet alone, if nothing is left
   to page) still does not fit, text goes below the minimums (caps 100,
   80, 64, 48, 32, 24, 16, 10, 4, 2, 1). Then file paths and references
   are shortened too, and last of all the page is dropped. Every shortened
   string ends in `…` and is listed in `truncatedFields`.

The minimums are 120 code units for prose (summary, next action, goal,
target/action/validation, list entries, finding detail) and 80 for titles.
At 120, a summary or next action still holds a full clause (about 20
words). At 80, a step title keeps its distinguishing words: the roadmap's
longest phase or step title is 69 code units. The 240 prose floor while
the target page fits keeps a typical action or validation sentence whole
(two to three clauses). A surrogate pair is never split, so a cut string
can be one unit shorter.

Some values are never shortened while one item fits: the path, planFile,
relevant files, checkpoint and page references, and the instruction.
Ids, hashes, enums (kind, severity, status), counts and retrieval
pointers are never shortened at all.

The output shape, field names, list caps
(`clamp(floor(maxChars/900), 4, 8)`), the `truncatedFields` cap and the
cursor format are unchanged. Every accepted budget still yields a packet
within `maxChars`, and every page makes progress. A page smaller than the
target only carries page-item prose at the 120 floor (text shrank first).
`TestResumeBudgetSweep` checks this over 33 budgets × 4 page limits ×
5 plans (including a synthetic 13-phase/38-step roadmap), paged to the
end. It also checks that no packet with more than one item goes below the
minimums or shortens a protected path, and that no page below the target
carries item prose above the floor. The only failure left is still the
§10a item 2 case, machine ids longer than the budget.

This supersedes the display-cap ladder in §10 ("Resume budget policy")
and the §9 item 7 residual. Resume output is no longer
reference-identical whenever the reference would have truncated. On
large plans the default budget pages with fewer, readable items per call.

**B. A filtered `workplan_read` returns a slice.** With `phaseId` and/or
`stepId`, the result is:

- `path`;
- `workplan`: the document without `phases`, and without `reviewFindings`
  and `notes` unless `includeNotes: true`. Unknown top-level metadata is
  kept;
- `selection` (unchanged);
- `plan` with `path`/`exists`. Its `content` is included only on an
  explicit `includeMarkdown: true`; with a filter the default becomes
  false;
- `dependencies`: entries whose source is selected or that depend on a
  selected step, the terminal summaries they reference, and all issues;
- `planHash`, `stateHash`;
- a new `slice` descriptor:
  `{filtered, phaseCount, stepCount, findingCount, noteCount,
  notesIncluded, markdownIncluded, dependenciesFiltered, full}`.

An unfiltered read, with or without `includeNotes`, is byte-identical to
the reference.

`includeNotes` is added to `schema/v1/tools/workplan_read.input.schema.json`
and to the adapter's registration snapshot
(`adapter/opencode/src/registration.json`, key `d1`). Removing the
addition and the `d1` key reproduces the reference snapshot byte-for-byte
(`referenceSha256`, checked by `plugin.test.ts`). This is the only
difference between the model-facing tool surface and the reference.

**C. Markdown drift warning.** `workplan_validate`, each `workplan_doctor`
plan entry and `workplan_patch {validate:true}` report a non-failing
warning when the linked Markdown exists, is nonblank and is not
byte-identical to the generated rendering of the stored JSON. In that
case later JSON changes (status, notes, findings, compaction) do not reach
the Markdown. The warning names the file and the explicit regeneration
path (`workplan_reset` `markdown-only` with `replaceMarkdown=true`).
The new `warnings` member appears only when nonempty, so outputs without
warnings are unchanged. `valid` and `issues` are unchanged.

**D. Status gate and patch issue list.** `workplan_create` and
`workplan_update` refuse to set the plan status to `in_progress`,
`review` or `completed` while the executable-structure rules (spec 01
§3, `x-shiori-structure-rules`, applied to the resulting plan) fail. The
refusal comes before authorization, so nothing is prepared, prompted or
written. The error class is `invalid_structure`. The message lists every
`path: message`, and the CLI `--json` error and the protocol error carry
`issues[{path, message}]`. `draft`, `blocked` and `cancelled` stay
allowed with incomplete structure. An update that does not set a gated
status is not gated (the `"draft"` placeholder is still a no-op). An
update that sets a gated status and completes the structure in the same
call is accepted.

`workplan_patch {validate:true}` (CLI and native) now returns
`metadata.validation.issues`, the full ordered issue list of
`workplan_validate`, next to `valid`/`issueCount`, plus `warnings`
when present.

**F. Stray artifacts in doctor.** `workplan_doctor` reports root-level
files of `.opencode/workplan/` that belong to no plan:

- `orphaned-sidecar`: `<id>.checkpoint.json` or `<id>.dependencies.json`
  with no `<id>.json`;
- `unclassified`: any other file that is not a plan, sidecar, lock,
  temporary or stage file and not a plan's linked Markdown, for example
  `*.patch`.

Journals without a primary JSON remain D1 recovery state, not strays.
Each entry suggests moving the file under `.opencode/workplan/archive/`.
Doctor never moves or deletes anything. The new members
`strayArtifacts[{name, kind, suggestion}]` (bounded by `limit`),
`strayArtifactCount`, `omittedStrayArtifacts` and `warnings` appear only
when there is at least one stray. `workplan_list` is unchanged.

## 12. Approved design changes D.2 (APPROVED 2026-09-30)

The dependency graph now drives work. The owner approved the brief on
2026-09-30, with two decisions: order checks **warn only** and never
refuse a write, and the critical path (extension X6,
[06](specs/06-extensions.md)) is included. Everything not listed here stays
as in sections 1–11: the V2 plan/checkpoint/dependencies/journal formats,
the `.opencode/workplan/` layout, the thirteen `workplan_*` identities
and argument shapes, the hash algorithm and the generated Markdown bytes.
The graph is never written into generated Markdown. There is no schema
change.

**Scope rule.** Every graph addition appears only when the plan has a
dependency sidecar that decodes and passes `ValidateDependencies` (no
issues). Without a sidecar, or with an invalid one (whose issues are
already reported), outputs are byte-identical to D.1. A prerequisite is
met only when it is `completed`, either as a plan step or as an archived
`terminalSummaries` entry. A step is open when it is neither `completed`
nor `cancelled`.

The oracle corpus is not edited. Vectors whose output changes on purpose
are listed in [`testdata/d2/expectations.json`](../testdata/d2/expectations.json)
(SHA-256 and UTF-16 length of the pinned Go output). Each one is judged
twice (`internal/engine/d2_vectors_test.go`). The same engine with the
graph additions turned off must still pass the oracle, D.1 pin or
divergence check it passed before, and the D.2 output must pass a
comparator against that output which restates readiness, downstream counts
and the critical path from the fixture's raw JSON. There are 11 such
vectors: 6 resume (`full-valid`, `list-mixed/b-plan`), 3 doctor and 2
inspect. No mutation vector changes.

**G1. Resume readiness.** The current step (`checkpoint.current`) and
every `active-work` page item carry `readiness: "ready" | "blocked"`,
right after their status. A ready step also carries `unblocks: N`. A
blocked step carries `blockedBy: [{phaseId, stepId, status}]`, its
prerequisites that are not completed, in stored order. The list shows at
most the pinned-list cap (`clamp(floor(maxChars/900), 4, 8)`, shrinking
with the D.1 list levels), and `blockedByOmitted` gives the rest when
there is any. Ids and statuses are never truncated, so the members cost
a fixed, small budget. The D.1 budgeting (target page 8/4/2, readable
floors, current work ≥512, pinned safety lists kept whole before the page
shrinks) is unchanged, and `TestResumeBudgetSweep` covers a 12-entry
cross-phase graph in addition to the D.1 plans. On the synthetic roadmap
with that graph, the 12000 budget gives 5 items where D.2-off gives 6, and
every other sampled budget gives the same count.

**G2. Order warnings (warn only).** `workplan_update` that sets a step to
`in_progress`, `review` or `completed` while a prerequisite is not completed succeeds
and returns `warnings` (only when nonempty), listing the unmet
prerequisites with their statuses. `workplan_validate`, each
`workplan_doctor` plan entry and `workplan_patch {validate:true}` report
every such step as a non-failing `warnings` entry
(`dependencies: Order warning: step <p>/<s> is <status> but its
prerequisites are not completed: ...`). `valid`, `issues` and every
refusal are unchanged. `review` counts as started (owner decision,
2026-09-30).

**G3. Cancelled prerequisites.** An open step whose unmet prerequisites
include a cancelled one, in the plan or as an archived terminal summary,
is reported as `dependencies: Step <p>/<s> is blocked by cancelled
prerequisite <p>/<s>. Replace or remove the dependency, or cancel the
step.` in validate, doctor and patch validation. Resume adds `Dependency
order warning: Step ...` to `safety.unverifiedWarnings`, after the
checkpoint freshness issue (when there is one) and before the unverified
checkpoint lines, and the item's `blockedBy` shows `status:
"cancelled"`. An update that cancels a step with open dependents returns a
warning naming them.

**G4. Phase replacement against the graph.** An update with `phases` (a
full replacement) and without `dependencies` re-validates the stored
sidecar against the resulting plan in the same prepared write. If the
replacement adds dependency issues, for example a removed prerequisite or
source step, it is refused before authorization:
`Invalid dependency metadata: <issues>; the phase replacement would leave
these dependency links dangling. Replace the dependencies in the same
update.` Issues the sidecar already had do not block the update. An update
that also supplies `dependencies` is validated against the result as
before. `workplan_reset` (draft) is not covered; it is revisited in D.3
(owner decision).

**G5. Dependency writes and views.**

- An entry with an empty `dependsOn` is refused:
  `Invalid dependency metadata: dependencies.<i>.dependsOn: Dependency
  entry must list at least one prerequisite`. A stored sidecar with such an
  entry is still read. Validate reports it as a warning
  (`...: Dependency entry lists no prerequisites.`), not an issue.
- A backward link (a step that depends on a step later in plan order) is
  accepted with a warning: `dependencies.<i>.dependsOn.<j>: Backward link:
  <p>/<s> depends on <p>/<s>, which comes later in plan order.` A
  dependency write returns all graph warnings of the result. Validate and
  doctor report them too.
- The compaction preview adds `archivedPrerequisites[{phaseId, stepId,
  status, dependents[{phaseId, stepId}]}]` (only when nonempty) for
  selected steps that remaining steps depend on. Apply keeps them as
  terminal summaries (unchanged).
- `workplan_inspect` step entries add `prerequisites[{phaseId, stepId,
  status}]` and `dependents[{phaseId, stepId}]`. Open steps also get
  `readiness`, `unblocks` and `slack`.

**G6. Downstream counts and critical path (X6).** `unblocks` is the
number of distinct open plan steps that depend on the step, directly or
transitively. Resume ranks `active-work` items as follows: ready items
first, by `unblocks` descending with ties in plan order, then blocked items
in plan order, then findings and references as before. The page total and
cursor are unchanged; the cursor offset indexes the ranked list. The
critical path is the heaviest chain of open steps. A step weighs its
optional numeric `estimate` member (kept as unknown step metadata, a
positive finite number), else 1. V2 has no estimate field and no
writer sets one: `estimate` is an optional member that stays hand-edited,
preserved like any unknown step member (owner decision, 2026-09-30). Ties go to more steps, then to earlier
plan order. `workplan_inspect` (top level) and each `workplan_doctor` plan
entry add `criticalPath: {length, estimate?, steps[{phaseId, stepId,
status}], recommendation}` when it chains at least two open steps.
`estimate` appears only when a step on the path used one. `slack` is the
critical weight minus the heaviest chain through the step. These are
recommendations only: nothing is executed, reordered on disk or
refused because of them. Resume does not include the critical path, to
keep its budget. The current-step selection is unchanged from D.1 (a
fresh checkpoint position, else the first in-progress, else the first
unfinished step, shown as `blocked` when it is; owner decision).

**Error class of dependency refusals.** Every refusal whose message starts
with `Invalid dependency metadata` (the existing dependency-write
refusals, the G4 phase-replacement and G5 empty-entry refusals, and a
compaction apply over an invalid sidecar) now has the protocol and CLI
`--json` error class `invalid_structure` instead of `internal` (owner
decision, 2026-09-30). The message text is unchanged, so the oracle
mutation vectors (which record messages only) still pass unchanged; the
class is pinned by `TestD2PhaseReplacement` and `TestD2DependencyWrites`.
