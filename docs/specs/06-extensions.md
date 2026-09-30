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

## 4. Not proposed

Ropes/piece tables, SQLite storage and Bloom filters remain deferred per
[03](03-performance.md) until measurements justify them.

## 5. Review checklist

For each candidate the owner decides: accept / defer / reject, scope, and the
stage it attaches to. Record decisions here with a date.

| ID | Decision | Date | Notes |
| --- | --- | --- | --- |
| X1 | to-review | | |
| X2 | to-review | | |
| X3 | to-review | | |
| X4 | to-review | | |
| X5 | to-review | | |
| X6 | to-review | | |
| X7 | to-review | | |
