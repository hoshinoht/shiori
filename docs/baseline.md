# TypeScript reference baseline (stage A)

Status: measured on 2026-09-30. This is reference data for [03](specs/03-performance.md).
It is not a Go result and not a performance target. Targets are set only after
the stage B measurement on the same machine and fixtures.

## Environment

| Item | Value |
| --- | --- |
| Machine | Apple M4 Pro, 12 cores (8 performance + 4 efficiency), 24 GiB RAM, on AC power |
| OS | macOS 27.0.1 (build 26A434), Darwin arm64, local APFS |
| Runtime | bun 1.4.0 |
| Reference code | Fingerprinted file by file in `testdata/MANIFEST.json` → `oracle.files`. The checkout at `fa060fc` has uncommitted changes, so the file hashes, not the commit, identify it |
| Library | zod 4.1.8 (the schema layer used by the reference) |
| Raw results | [`testdata/perf/baseline-results.json`](../testdata/perf/baseline-results.json): two complete runs |

## Fixtures

The fixtures are deterministic generated plans. `id` is `perf-plan`. Phases hold
50 steps each. Every step has a title, target, action and validation. The plan
also has one finding per 10 steps (severities rotate; every third finding is
resolved) and one note per 10 steps. The Markdown is the reference's generated
rendering, so every file counts as "generated". There are no spec files or
sidecars.

| Fixture | Steps / phases / findings / notes | JSON bytes | MD bytes | Stored as |
| --- | --- | --- | --- | --- |
| perf-100k | 258 / 6 / 25 / 25 | 102,415 | 83,621 | `testdata/perf/perf-100k/` |
| perf-1m | 2,631 / 53 / 263 / 263 | 1,048,711 | 860,239 | `testdata/perf/perf-1m.tar.gz` |
| perf-10m | 26,038 / 521 / 2,603 / 2,603 | 10,486,061 | 8,648,334 | `testdata/perf/perf-10m.tar.gz` |

Archive sha256: `perf-1m.tar.gz`
`f18891949bb1f2ddc37f97b4af6864cc3342a859c70cb284578d9e174a0ffbbc` and
`perf-10m.tar.gz` `701f2261d6cc4aee3e573886b78f4b9bdf8d802e77522f4cae9c2907f5fb4e4c`.
The archive bytes are not reproducible (tar metadata); the extracted file
hashes are the fixture identity:

```text
cd06bc5924c87ee1c20c95c7537c9851e1218f6d1c27608e4fe9b60c7a27a565  perf-100k/.opencode/workplan/perf-plan.json
f11fd877ebd38fb939619b7f663fcebb5cafd12ac508bb5072f421479ed49a35  perf-100k/.opencode/workplan/perf-plan.md
2561e5ef5d51549b500e135cf80a4135164b6ffcd3d0d64bdf27ff919ffff25a  perf-1m/.opencode/workplan/perf-plan.json
df0e796e1a42fa01f3e3cd42dbcf03f41000a33452cdd79ea28ddd30d1784b0a  perf-1m/.opencode/workplan/perf-plan.md
75b17b3339d7be5c6a671369eebc8e200d777fcdc6947fb09275cbf310354e36  perf-10m/.opencode/workplan/perf-plan.json
0d09581aade5ea64c4fba6d6c46e5c256cdfc874639e0496403bf842f6bcff22  perf-10m/.opencode/workplan/perf-plan.md
```

## Method

- **Operations.** Each operation calls the reference tool's `execute` with core
  input, default options and the fixture root as the workspace.
  - `read`: `{id}`, which includes the Markdown.
  - `inspect`: `{id}`, limit 100.
  - `resume`: `{id}`, maxChars 12000, limit 20.
  - `validate`: `{id}`.
  - `update`: `{id, appendNotes: ["bench note i"]}`, with no `expectedHash`.
- **Permission prompt.** The context's `ask` resolves at once. So `update`
  includes locking, staging, journaling, publication and fsync, but no
  host-permission or IPC time.
- **Cold.** Each sample is a new `bun` process that imports the reference and
  runs one call. The table reports:
  - the in-process time of that first call ("cold first call"), and
  - the wall time of the whole process ("cold process"). This includes bun start
    and module import. A no-op process of the same kind measured
    28 ms median / 44 MiB RSS for the 100k and 1m fixtures, and 29 ms / 61 MiB
    for 10m.
- **Samples.** 15 cold processes per operation (5 for perf-10m).
- **Warm.** One process runs 2 unrecorded warm-up calls, then 100 recorded calls
  (30 for perf-1m, 10 for perf-10m).
- **Update isolation.** For `update`, the original JSON and Markdown bytes are
  restored before each iteration. The restore is outside the timed region.
- **Peak RSS.** The maximum resident set size reported by `/usr/bin/time -l` for
  the process, which includes the bun runtime.
- **Percentiles.** p95 is the nearest-rank value. With only 5 cold samples, the
  p95 for perf-10m is the maximum.
- **Repeat.** The whole matrix ran twice. Run 2 is shown; the last column gives
  the run 1 warm median.

## Results (run 2; ms)

| Size | Op | Warm median | Warm p95 | Cold first call median / p95 | Cold process wall median / p95 | Peak RSS cold median (max) | Output chars | Run 1 warm median |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 100 KiB | read | 2.12 | 2.89 | 5.5 / 6.0 | 34 / 35 | 49 MiB (50) | 293,459 | 1.89 |
| 100 KiB | inspect | 1.64 | 2.40 | 5.5 / 6.6 | 34 / 36 | 47 MiB (48) | 34,604 | 1.70 |
| 100 KiB | resume | 1.93 | 2.79 | 6.4 / 7.4 | 35 / 36 | 47 MiB (48) | 11,440 | 1.91 |
| 100 KiB | validate | 2.02 | 3.23 | 5.9 / 6.5 | 35 / 38 | 48 MiB (48) | 815 | 1.66 |
| 100 KiB | update | 11.09 | 13.27 | 21.6 / 38.6 | 50 / 70 | 58 MiB (60) | 760 | 10.30 |
| 1 MiB | read | 7.10 | 9.28 | 14.7 / 15.5 | 42 / 44 | 84 MiB (89) | 3,004,147 | 7.01 |
| 1 MiB | inspect | 4.30 | 5.76 | 11.5 / 11.8 | 39 / 40 | 62 MiB (63) | 34,605 | 4.72 |
| 1 MiB | resume | 4.60 | 5.72 | 13.1 / 13.7 | 41 / 42 | 62 MiB (63) | 11,524 | 5.02 |
| 1 MiB | validate | 4.99 | 6.04 | 12.8 / 14.2 | 40 / 43 | 64 MiB (65) | 814 | 5.12 |
| 1 MiB | update | 24.87 | 27.62 | 41.5 / 45.5 | 70 / 76 | 126 MiB (128) | 759 | 25.31 |
| 10 MiB | read | 58.43 | 61.58 | 70.5 / 71.0 | 101 / 103 | 319 MiB (335) | 30,066,121 | 60.57 |
| 10 MiB | inspect | 24.80 | 26.92 | 38.2 / 38.7 | 68 / 73 | 178 MiB (179) | 34,612 | 26.82 |
| 10 MiB | resume | 26.17 | 28.13 | 40.7 / 42.6 | 71 / 72 | 180 MiB (180) | 11,536 | 26.44 |
| 10 MiB | validate | 29.07 | 35.27 | 47.0 / 48.1 | 77 / 79 | 226 MiB (227) | 819 | 28.37 |
| 10 MiB | update | 152.45 | 160.58 | 202.6 / 243.4 | 246 / 286 | 711 MiB (732) | 764 | 161.34 |

"Output chars" is the length of the returned text in UTF-16 code units.

## Observations (reference behaviour, not targets)

- Warm latency grows roughly linearly with plan size. From 1 MiB to 10 MiB it
  grows about 5.5–8×, and `update` is the most expensive operation: it parses,
  serializes and hashes the JSON, renders and compares the Markdown, journals
  and fsyncs. At 10 MiB, `update` peaks near 0.7 GiB RSS.
- `resume` stays inside its 12000-char budget at every size (11.4–11.5 K), but
  still costs O(plan) time and memory. `inspect` output is bounded by `limit`.
- `read` output has no bound: 30.1 M chars for the 10 MiB plan. This exceeds the
  16 MiB frame proposed in spec 02; see contracts D9.
- At 100 KiB, process start and module import (~28 ms) dominate every one-shot
  call. For the native adapter that is the case for a long-lived child
  (contracts 5.4).

## Not yet measured (spec 03 matrix gaps)

- Checkpoint, compaction preview/apply, doctor, and interrupted-state recovery
  latency.
- Plans with many spec files, deep dependency chains, heavy escaped Unicode,
  large unknown metadata, or 10,000+ findings.
- Separate counts of bytes read/written, parse/hash/serialization passes and
  allocations.
- IPC and permission-broker overhead (no adapter exists yet).
- Concurrent-process and cancellation stress.
- Linux/amd64.

Stage B should measure the Go read path on the same fixtures, fill in the rows
that apply, and repeat this table on the same machine before any target is set.
