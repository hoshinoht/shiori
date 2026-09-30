// Regenerates src/registration.json from the reference TypeScript workplan
// plugin so that the adapter registers byte-identical tool descriptions and
// input schemas (same identities, argument shapes and model-facing text),
// plus the approved D.1 addition (workplan_read includeNotes), the D.3
// reset changes (wipe mode, previewToken, confirmation, accurate text) and
// the D.4 compaction noteRollover selector.
//
//   bun scripts/snapshot-registration.ts /path/to/workplan-tools/src/core > src/registration.json
//
// The reference is only executed; nothing else is copied. Input
// validation itself stays in the Go core (schema/v1/tools).
const core = process.argv[2];
if (!core) {
  console.error("usage: bun scripts/snapshot-registration.ts <workplan-tools/src/core>");
  process.exit(2);
}
const mod = await import(core);
const { nativeWorkplanInputSchemas, workplanInputJsonSchema, workplanToolDefinitions, createNativeWorkplanInputSchema } = mod;
const order = ["create", "update", "inspect", "validate", "read", "list", "patch", "reset", "resume", "checkpoint", "compact", "doctor"];
const tools: Array<{ name: string; description: string; input: unknown }> = [];
for (const key of order) {
  tools.push({
    name: `workplan_${key}`,
    description: workplanToolDefinitions[`workplan_${key}`].description,
    input: workplanInputJsonSchema(nativeWorkplanInputSchemas[key]),
  });
}
const { mode: _mode, ...previewArgs } = workplanToolDefinitions.workplan_compact.args;
tools.push({
  name: "workplan_compact_preview",
  description: "Read-only preview of the exact workplan history selection and archive intent; this tool never applies compaction.",
  input: workplanInputJsonSchema(createNativeWorkplanInputSchema(previewArgs)),
});
const reference = JSON.stringify({ source: "reference workplan-tools native registration (generated; do not edit)", tools }, null, 2) + "\n";
// Approved design change D.1 (docs/contracts.md §11 item B, 2026-09-30):
// the only deliberate difference from the reference registration. The
// adapter test removes these additions and the "d1" key again and checks
// the result against referenceSha256.
const { createHash } = await import("node:crypto");
const read = tools.find((tool) => tool.name === "workplan_read")!.input as { properties: Record<string, unknown> };
read.properties.includeNotes = {
  description: "Only with phaseId/stepId: also return reviewFindings and notes (default false). A filtered read returns the plan header, hashes and the selected phase/step; linked Markdown only with includeMarkdown=true",
  type: "boolean",
};
// Approved design change D.3 (docs/contracts.md §13 item 1, 2026-09-30):
// the reset wipe mode with its preview token and confirmation, and
// reset text that describes the status-only draft reset. Each changed
// value is recorded with its reference value so the test can restore it.
const reset = tools.find((tool) => tool.name === "workplan_reset")! as { description: string; input: any };
const changes: Array<{ tool: string; path: string[]; reference: unknown }> = [];
const change = (path: string[], value: unknown) => {
  let cur: any = reset;
  for (const key of path.slice(0, -1)) cur = cur[key];
  changes.push({ tool: "workplan_reset", path, reference: cur[path[path.length - 1]] });
  cur[path[path.length - 1]] = value;
};
change(["description"], "Reset a workplan: draft resets every status (plan, phases, steps) to draft and removes the checkpoint, keeping phases, steps, notes, findings and dependencies; wipe clears phases, findings and notes after a preview and an exact confirmation, archiving the originals first; markdown-only only regenerates the linked Markdown from the JSON.");
change(["input", "properties", "mode", "description"], "draft resets statuses only and keeps the plan content; wipe clears phases, findings and (unless preserveNotes) notes: call it first without previewToken/confirmation for a read-only preview, then again with that previewToken and confirmation=WIPE_PLAN_CONTENT; markdown-only only regenerates the Markdown");
change(["input", "properties", "mode", "enum"], ["draft", "markdown-only", "wipe"]);
change(["input", "properties", "preserveNotes", "description"], "Keep notes during a wipe (a draft reset always keeps notes)");
reset.input.properties.previewToken = { description: "mode=wipe only: the previewToken returned by the wipe preview", type: "string" };
reset.input.properties.confirmation = { description: "mode=wipe only: must be WIPE_PLAN_CONTENT to apply the previewed wipe", type: "string" };
// Approved design change D.4 (docs/contracts.md §15, 2026-09-30): the
// note rollover selector of workplan_compact and workplan_compact_preview,
// inserted after resolvedFindingIndexes.
const rollover = {
  description: "Instead of noteIndexes: archive every note older than the latest keepLatest (default 20) except pinned ones: [pinned] in the text or pinNoteIndexes, decision records (decision/decided or USER), notes naming an open step as phaseId/stepId or quoting an open finding title, and compaction archive pointers. Apply needs a fresh checkpoint; pass the same noteRollover to preview and apply",
  type: "object",
  properties: {
    keepLatest: { description: "Number of newest notes that always stay (default 20)", type: "integer", minimum: 1, maximum: 10000 },
    pinNoteIndexes: { description: "Zero-based indexes of further notes to keep", type: "array", items: { type: "integer", minimum: 0, maximum: 9007199254740991 } },
  },
  additionalProperties: false,
};
for (const name of ["workplan_compact", "workplan_compact_preview"]) {
  const input = tools.find((tool) => tool.name === name)!.input as { properties: Record<string, unknown> };
  const next: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(input.properties)) {
    next[key] = value;
    if (key === "resolvedFindingIndexes") next.noteRollover = rollover;
  }
  input.properties = next;
}
console.log(JSON.stringify({
  source: "reference workplan-tools native registration (generated; do not edit)",
  tools,
  d1: {
    note: "Approved design change D.1 (docs/contracts.md §11 item B, 2026-09-30). Removing this key and every listed addition reproduces the reference snapshot byte-for-byte (sha256 below).",
    referenceSha256: createHash("sha256").update(reference).digest("hex"),
    additions: [{ tool: "workplan_read", property: "includeNotes" }],
  },
  d3: {
    note: "Approved design change D.3 (docs/contracts.md §13 item 1, 2026-09-30). Removing this key and the d1 key, every listed addition, and restoring each listed change to its reference value reproduces the reference snapshot byte-for-byte (d1.referenceSha256).",
    additions: [{ tool: "workplan_reset", property: "previewToken" }, { tool: "workplan_reset", property: "confirmation" }],
    changes,
  },
  d4: {
    note: "Approved design change D.4 (docs/contracts.md §15, 2026-09-30): the note rollover selector of compaction (spec 06 P3). Removing this key with the d1/d3 keys, every listed addition, and restoring each d3 change reproduces the reference snapshot byte-for-byte (d1.referenceSha256).",
    additions: [{ tool: "workplan_compact", property: "noteRollover" }, { tool: "workplan_compact_preview", property: "noteRollover" }],
  },
}, null, 2));
