# 06 — Extensions (to-review)

Status: **to-review, optional.** Proposed 2026-09-30. None of these is required
for stages A–G or for any acceptance gate in [05](05-migration-and-acceptance.md).
Each needs owner review before it is scheduled, and each ships only after
stage D.

## 1. Ground rules

- Keep the original workplan design: no V2 plan field changes, no new or renamed
  `workplan_*` tools, same hashes and generated Markdown.
- Every extension is a separately versioned, parent-owned sidecar classified per
  [01 §4](01-core.md) before it is exposed, and written only through the
  transaction engine ([01 §6](01-core.md)).
- A missing or corrupt extension sidecar degrades that feature only; it never
  hides plan state or blocks core operations.
- Performance items follow [03](03-performance.md): measure, record a target,
  then change.

## 2. Candidates

Ordered by expected value for effort.

| ID | Extension | Value | Depends on |
| --- | --- | --- | --- |
| X1 | Per-file hash cache + Merkle-style manifest | Skip rehashing unchanged artifacts; state-hash recompute proportional to what changed. Targets the measured 10 MiB warm regression | 03 baseline |
| X2 | Evidence ledger | Makes completion checkable instead of asserted | stage C |
| X3 | Worktree lanes: path-claim trie, lane state machine, baseline tree fingerprint, merge train | Safe parallel execution | [04](04-worktrees.md), X2 |
| X4 | Hash-chained event log | Audit trail, cheap "changed since checkpoint", basis for undo | stage C |
| X5 | Cross-plan workspace graph | Portfolio view of related plans in one repo | X4 optional |
| X6 | Critical path and slack | Surface the steps that block the most work | stage F ready queue |
| X7 | Session ↔ step/lane links | Precise handoffs across sessions | X3 optional |
| P2 | Compaction advisor | Keeps long-running plans small without manual bookkeeping | stage C compaction |
| P3 | Note rollover | Keeps the plan roughly constant in size over hours of iteration | P2, stage C compaction |
| P4 | Journal v2 by reference | About 3x fewer bytes written per mutation on large plans | stage C journal |
| P5 | Markdown section index for resume | Smaller resume packets on Markdown-heavy plans | stage B resume |
| P6 | Stray-file classification in doctor | Tidy workplan roots; no unknown files silently ignored | stage B doctor |

## 3. Sketches

### X1 — Hash cache and Merkle manifest

Cache `sha256` per artifact keyed by `(device, inode, size, mtime_ns)`; any key
mismatch rehashes. The cache is a hint, never mutation authority: mutations
still reread and rehash under locks. The `*-v1` hash output must stay
byte-identical; the Merkle structure is internal only.

### X2 — Evidence ledger

Candidate `<id>.evidence.json` v1: per `(phaseId, stepId)` records of
`{command, exitCode, treeOid, outputDigest, recordedAt, source}`. `treeOid` is
the git tree actually tested, including uncommitted changes via a temporary
index. Evidence is stale when files owned by the step changed after `treeOid`.
Plan hashes are not code fingerprints (01 §7); this adds the missing code-side
binding. `metric-loop` results can be imported as evidence.

### X3 — Worktree lanes, concrete structures

- **Path-claim trie:** owned paths and globs per lane in a prefix tree with
  wildcard nodes; overlapping claims are rejected before a lane is created.
- **Lane state machine:** `claimed → prepared → running → review → integrating
  → merged | abandoned`. Doctor reports orphans (worktree without owner,
  unmerged branch for a completed step) and requires explicit disposition (W03).
- **Baseline fingerprint:** git tree OID of the starting state, dirty changes
  included, recorded in the lane manifest (W01).
- **Merge train:** topological order of lane steps from the dependency DAG,
  ties broken by predicted file overlap; combined state is retested after each
  integration (W02). Merges stay user- or orchestrator-authorized, never
  automatic.

### X4 — Hash-chained event log

Append-only `<id>.history.jsonl`; each entry carries operation, actor source,
before/after state hashes and the previous entry's hash. Compaction archives
closed log segments instead of rewriting them. Undo is a separately reviewed
follow-up, not part of X4.

### X5 — Cross-plan workspace graph

Optional links between plans in the same coordination root (for example a
roadmap blocking a migration plan). Read-only portfolio projection first;
cross-plan writes need their own locking design.

### X6 — Critical path and slack

Longest-path and slack over the step DAG, weighted by optional estimates.
Resume and inspect may show "blocks N downstream steps". Recommendations only;
never automatic execution.

### X7 — Session links

Record OpenCode session IDs from trusted host context (never model input) that
touched each step or lane, for `/handoff` and resume.

### Long-plan measurements behind P2–P6

Measured 2026-09-30 on a copy of a real plan after many hours of iteration
(257 KB JSON, 203 KB Markdown, 12 phases, 62 steps, 214 notes). Go stage B
timings were ~22–24 ms for every read-only operation including process start,
so parse/hash cost is not the bottleneck at this size; X1 matters only at
~10 MB. The costs are elsewhere:

- `notes` is 48% of the JSON; terminal (completed/cancelled) steps are 48% of
  step bytes; handwritten Markdown sections (work packages, execution
  receipts, decision register) are append-only.
- A full `read` returned 576 KB (~145k tokens) versus ~12 KB for `resume` at
  the 12000 budget. (hoshi-opencode2 now steers agents to bounded reads.)
- The v1 journal stores base64 before/after content, so one status change on
  this plan writes on the order of 1 MB plus fsyncs, growing with every note.
- Loose `.patch` files (~200 KB) sit in the workplan root unclassified.

### P2 — Compaction advisor

`resume` and `doctor` report `compactionRecommended` with the estimated
savings when terminal-step bytes, note count or total size cross configurable
thresholds. Advice only: compaction still requires preview → exact token →
authorized apply ([01 §7](01-core.md)).

### P3 — Note rollover

A compaction selection mode that archives notes older than the latest N (and
older than the newest checkpoint), keeping notes pinned by the decision
register or referenced by open steps/findings. Archives keep complete
originals. Uses the existing "selected history" mechanism; no V2 field change.

### P4 — Journal v2 by reference

A separately versioned `<id>.transaction.json` v2 that records digests and
the paths of same-directory staged files instead of inline base64 content.
v1 journals stay readable and recoverable. Needs its own crash/fault matrix
(S04) and must not weaken third-state detection (S05). This is the only
item here that adds a new artifact version.

### P5 — Markdown section index for resume

Hash-bound heading/marker ranges over the plan Markdown so `resume` can point
to "execution receipts §N" by offset instead of inlining text. Reads remain
byte-exact; offsets are invalidated by any Markdown hash change.

### P6 — Stray-file classification

Doctor lists files in the workplan root that are neither known artifacts,
sidecars, locks, staging files nor archives (for example `*.patch`), and
suggests moving them under `archive/`. Never moves or deletes them itself.

## 4. Not proposed

Ropes/piece tables, SQLite storage and Bloom filters remain deferred per
[03](03-performance.md) until measurements justify them.

## 5. Review checklist

For each candidate the owner decides: accept / defer / reject, scope, and the
stage it attaches to. Record decisions here with a date.

| ID | Decision | Date | Notes |
| --- | --- | --- | --- |
| X1 | to-review | | |
| X2 | accepted | 2026-09-30 | stage E1, after D.4 |
| X3 | accepted | 2026-09-30 | stage E2, after E1 (needs X2) |
| X4 | to-review | | |
| X5 | to-review | | |
| X6 | accepted | 2026-09-30 | implemented in stage D.2 |
| X7 | to-review | | |
| P2 | accepted | 2026-09-30 | stage D.4 |
| P3 | accepted | 2026-09-30 | stage D.4 |
| P4 | to-review | | |
| P5 | to-review | | |
| P6 | accepted | 2026-09-30 | covered by D.1 (F) and D.3 |
