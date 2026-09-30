// Regenerates src/registration.json from the reference TypeScript workplan
// plugin so that the adapter registers byte-identical tool descriptions and
// input schemas (same identities, argument shapes and model-facing text),
// plus the approved D.1 addition (workplan_read includeNotes).
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
console.log(JSON.stringify({
  source: "reference workplan-tools native registration (generated; do not edit)",
  tools,
  d1: {
    note: "Approved design change D.1 (docs/contracts.md §11 item B, 2026-09-30). Removing this key and every listed addition reproduces the reference snapshot byte-for-byte (sha256 below).",
    referenceSha256: createHash("sha256").update(reference).digest("hex"),
    additions: [{ tool: "workplan_read", property: "includeNotes" }],
  },
}, null, 2));
