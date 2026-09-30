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

Stage B measured the Go read path on the same fixtures and machine; see below.
No performance target is set yet (spec 03 §1: stage F sets targets from these
two tables).

## Go stage B results (same machine, 2026-09-30)

| Item | Value |
| --- | --- |
| Machine / OS | As above (Apple M4 Pro, macOS 27.0.1, darwin/arm64, local APFS) |
| Toolchain | Go 1.27.1, `CGO_ENABLED=0 go build -trimpath` |
| Code | Working tree of stage B (uncommitted; the engine at the time of this run) |
| Fixtures | The same three fixtures, verified against the sha256 list above before measuring |
| Raw results | [`testdata/perf/go-baseline-results.json`](../testdata/perf/go-baseline-results.json) |
| Harness | `SHIORI_BASELINE=<out.json> go test ./internal/engine -run '^TestBaselineMatrix$' -v` |

Method, matched to the reference:

- **Inputs.** The same operation inputs, with default options: `read {id}`
  including the Markdown; `inspect {id}` at limit 100; `resume {id}` at 12000
  and limit 20; `validate {id}`. The timed region includes serializing the
  result text, which the reference also returns.
- **Warm.** In one process, 2 unrecorded calls, then 100 recorded (30 for
  perf-1m, 10 for perf-10m).
- **Cold first call.** The in-process time of the first call in a fresh
  process: 15 processes (5 for perf-10m).
- **Cold process wall.** The wall time of the `shiori <op> perf-plan --json`
  binary as a fresh process, including start-up and writing the output.
- **Peak RSS.** The maximum resident set size of that CLI process, from
  `/usr/bin/time -l`.
- **Percentiles.** p95 is the nearest-rank value.

"Output chars" is in UTF-16 code units. It includes the absolute root path,
which differs in length from the reference's measurement root, so it differs
from the reference by a few characters. `update` is not measured: writes are
stage C.

| Size | Op | Warm median | Warm p95 | Cold first call median / p95 | Cold process wall median / p95 | Peak RSS median (max) | Output chars |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 100 KiB | read | 0.79 | 0.89 | 0.90 / 3.45 | 6.6 / 432.7¹ | 9 MiB (9) | 293,557 |
| 100 KiB | inspect | 0.43 | 0.63 | 0.61 / 0.73 | 5.6 / 5.8 | 7 MiB (7) | 34,702 |
| 100 KiB | resume | 0.46 | 0.66 | 0.60 / 0.73 | 5.6 / 5.9 | 7 MiB (7) | 11,489 |
| 100 KiB | validate | 0.38 | 0.59 | 0.53 / 0.59 | 5.7 / 6.6 | 6 MiB (6) | 913 |
| 1 MiB | read | 7.26 | 7.83 | 8.09 / 8.65 | 14.3 / 15.1 | 25 MiB (28) | 3,004,245 |
| 1 MiB | inspect | 3.91 | 4.13 | 4.17 / 4.47 | 9.5 / 9.7 | 12 MiB (12) | 34,703 |
| 1 MiB | resume | 4.51 | 4.82 | 4.85 / 4.97 | 10.3 / 10.9 | 14 MiB (14) | 11,573 |
| 1 MiB | validate | 3.93 | 4.25 | 4.13 / 4.27 | 9.5 / 9.8 | 12 MiB (12) | 912 |
| 10 MiB | read | 81.82 | 95.74 | 86.90 / 94.95 | 101.3 / 105.1 | 176 MiB (178) | 30,066,219 |
| 10 MiB | inspect | 36.15 | 36.74 | 37.20 / 37.42 | 42.8 / 43.0 | 52 MiB (52) | 34,710 |
| 10 MiB | resume | 40.58 | 42.72 | 41.29 / 42.01 | 47.7 / 48.3 | 71 MiB (72) | 11,585 |
| 10 MiB | validate | 35.51 | 36.54 | 36.47 / 38.13 | 42.9 / 43.3 | 52 MiB (60) | 917 |

¹ This is the first execution of a freshly built binary. macOS checks a new
executable on its first launch, so this one sample includes that check. Every
other sample is 5–7 ms.

### Comparison with the reference (median; ratio = TS / Go)

| Size | Op | Warm TS → Go (ms) | Cold process TS → Go (ms) | Peak RSS TS → Go (MiB) |
| --- | --- | --- | --- | --- |
| 100 KiB | read | 2.12 → 0.79 (2.7×) | 34 → 6.6 (5.2×) | 49 → 9 |
| 100 KiB | inspect | 1.64 → 0.43 (3.8×) | 34 → 5.6 (6.1×) | 47 → 7 |
| 100 KiB | resume | 1.93 → 0.46 (4.2×) | 35 → 5.6 (6.3×) | 47 → 7 |
| 100 KiB | validate | 2.02 → 0.38 (5.3×) | 35 → 5.7 (6.1×) | 48 → 6 |
| 1 MiB | read | 7.10 → 7.26 (1.0×) | 42 → 14.3 (2.9×) | 84 → 25 |
| 1 MiB | inspect | 4.30 → 3.91 (1.1×) | 39 → 9.5 (4.1×) | 62 → 12 |
| 1 MiB | resume | 4.60 → 4.51 (1.0×) | 41 → 10.3 (4.0×) | 62 → 14 |
| 1 MiB | validate | 4.99 → 3.93 (1.3×) | 40 → 9.5 (4.2×) | 64 → 12 |
| 10 MiB | read | 58.43 → 81.82 (0.71×) | 101 → 101.3 (1.0×) | 319 → 176 |
| 10 MiB | inspect | 24.80 → 36.15 (0.69×) | 68 → 42.8 (1.6×) | 178 → 52 |
| 10 MiB | resume | 26.17 → 40.58 (0.64×) | 71 → 47.7 (1.5×) | 180 → 71 |
| 10 MiB | validate | 29.07 → 35.51 (0.82×) | 77 → 42.9 (1.8×) | 226 → 52 |

Observations (not targets):

- Go wins every cold measurement, because there is no runtime or module start
  (~5 ms against bun's ~28 ms), and it uses roughly 2–7× less memory at every size.
- Warm, Go is faster at 100 KiB. It is about even at 1 MiB, and 1.2–1.6×
  slower at 10 MiB. At 10 MiB the dominant costs are the kernel page faults
  for the 10–30 MB buffers (`runtime.madvise`) and the single parse of the
  10 MiB JSON. JavaScriptCore's JSON parser and warmed heap are strong on this
  workload.
- `resume`, `inspect` and `validate` still cost O(plan) in both engines:
  every call reads, hashes and decodes the whole artifact set (spec 03 §3,
  reuse and caching are stage F).
- **First-implementation numbers.** The 10 MiB warm medians before the stage B
  allocation fixes were: read 122.9, inspect 68.7, resume 72.3,
  validate 71.2 ms. The fixes were a pre-sized file read, no per-object maps,
  lazy field paths, zero-copy strings over the immutable artifact buffer and
  slot-backed optional fields. They cut allocations about 7×. The before/after
  profiles were taken on the same fixtures.

The reference-table gaps listed above remain open for Go as well. Go also has
no `update`, checkpoint or compaction numbers yet.
