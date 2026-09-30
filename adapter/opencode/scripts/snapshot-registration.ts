// Regenerates src/registration.json from the reference TypeScript workplan
// plugin so that the adapter registers byte-identical tool descriptions and
// input schemas (same identities, argument shapes and model-facing text).
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
console.log(JSON.stringify({ source: "reference workplan-tools native registration (generated; do not edit)", tools }, null, 2));
