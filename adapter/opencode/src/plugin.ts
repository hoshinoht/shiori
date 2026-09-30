// Shiori OpenCode adapter (stage D, spec 02). It registers exactly the
// thirteen existing workplan_* identities with the reference plugin's
// descriptions and input schemas, takes trusted identity, root and
// AbortSignal only from the native ToolContext, asks the executing host's
// permission engine for the exact canonical resources of every prepared
// intent, and commits only that intent over the private stdio protocol.
import { realpath } from "node:fs/promises";
import { dirname, isAbsolute, relative, resolve, sep } from "node:path";

import type { Plugin } from "@opencode/plugin";
import type { ToolContext } from "@opencode/plugin/promise/tool";

import { CoreClient, ShioriError, type CoreClientOptions, type HostContext, type PreparedIntent } from "./core-client";
import { registerNativePermissionBridge, type NativePermissionBridge, type NativePermissionBridgeContext } from "./permission-bridge";
import registration from "./registration.json" with { type: "json" };

/** Plugin identity kept from the reference plugin (one writer per root). */
export const PLUGIN_ID = "workplan-tools";
/** Verified hosts (contracts §5.6); other versions fail closed at registration. */
export const SUPPORTED_HOST_VERSIONS = ["2.0.19", "2.0.20"] as const;

export const TOOL_NAMES = registration.tools.map((tool) => tool.name);

const authoringTools = new Set(["workplan_create", "workplan_update", "workplan_patch", "workplan_reset"]);
const authoringAgents = new Set(["plan", "orchestrator"]);
const progressTitles: Record<string, string> = {
  workplan_create: "Create workplan",
  workplan_checkpoint: "Checkpoint workplan",
  workplan_compact: "Compact workplan",
  workplan_compact_preview: "Compact workplan",
  workplan_list: "List workplans",
  workplan_patch: "Patch workplan",
  workplan_read: "Read workplan",
  workplan_resume: "Resume workplan",
  workplan_validate: "Validate workplan",
};

type ToolEditor = Parameters<Parameters<Plugin.Context["tool"]["transform"]>[0]>[0];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Role matrix (spec 02 §5); supplements host policy, never widens it. */
export function assertRole(toolName: string, rawInput: unknown, agent: string): void {
  const input = isRecord(rawInput) ? rawInput : {};
  if (authoringTools.has(toolName) && !authoringAgents.has(agent)) {
    throw new Error("Only the plan agent or orchestrator may author workplan lifecycle changes");
  }
  if (toolName === "workplan_checkpoint" && agent !== "orchestrator") {
    throw new Error("Only the orchestrator may update a workplan checkpoint");
  }
  if (toolName === "workplan_update" && input.recovery !== undefined && agent !== "orchestrator") {
    throw new Error("Only the orchestrator may recover a workplan transaction");
  }
  if (toolName === "workplan_compact" && input.mode === "apply" && agent !== "orchestrator") {
    throw new Error("Only the orchestrator may apply workplan compaction");
  }
}

function within(root: string, path: string): boolean {
  const r = relative(root, path);
  return r !== "" && r !== ".." && !r.startsWith(`..${sep}`) && !isAbsolute(r);
}

/**
 * Every filesystem path a commit may create, replace, delete, lock or
 * stage, plus their parent directories inside the project: the resources
 * the host is asked to allow as "edit" (the reference requested the same
 * class of resources). Reads are preconditions only.
 */
export function editResources(root: string, prepared: PreparedIntent): string[] {
  const r = prepared.resources;
  const paths = [...r.writePaths, ...r.deletePaths, ...r.lockPaths, ...r.stagingPaths, ...r.archivePaths];
  const all = new Set<string>();
  for (const path of paths) {
    all.add(path);
    let dir = dirname(path);
    while (dir !== root && within(root, dir)) {
      all.add(dir);
      const parent = dirname(dir);
      if (parent === dir) break;
      dir = parent;
    }
  }
  return [...all].sort();
}

/**
 * Independent scope check of a prepared intent before any host request
 * (spec 02 §3 step 2): bound to this root and tool, every resource an
 * exact absolute path inside the project's .opencode directory.
 */
export function validatePrepared(root: string, toolName: string, prepared: PreparedIntent): void {
  if (!isRecord(prepared) || prepared.canonicalRoot !== root) {
    throw new Error("Native mutation intent does not match the active canonical project");
  }
  if (prepared.tool !== toolName || typeof prepared.intentId !== "string" || !/^[0-9a-f]{64}$/.test(prepared.intentDigest) ||
    !/^[0-9a-f]{64}$/.test(prepared.capability) || typeof prepared.workplanId !== "string" || !isRecord(prepared.resources)) {
    throw new Error("Native mutation intent is malformed; no permission was requested and nothing was changed");
  }
  const scope = resolve(root, ".opencode");
  for (const key of ["readPaths", "writePaths", "deletePaths", "lockPaths", "stagingPaths", "archivePaths"] as const) {
    const list = prepared.resources[key];
    if (!Array.isArray(list)) throw new Error("Native mutation intent is malformed; no permission was requested and nothing was changed");
    for (const path of list) {
      if (typeof path !== "string" || !isAbsolute(path) || resolve(path) !== path || path.includes("\0")) {
        throw new Error("Native mutation intent contains a non-canonical resource; no permission was requested and nothing was changed");
      }
      // Linked Markdown and specs may be read anywhere in the project;
      // every write-class resource must stay under .opencode.
      if (key === "readPaths" ? !within(root, path) : !within(scope, path)) {
        throw new Error("Native mutation intent reaches outside the workplan artifact scope; no permission was requested and nothing was changed");
      }
    }
  }
}

function sameResources(actual: readonly string[], expected: readonly string[]): boolean {
  const expectedSorted = [...expected].sort();
  return actual.length === expectedSorted.length && actual.every((path, index) => path === expectedSorted[index]);
}

function permissionDecision(value: string): "allow" | "deny" | "ask" | "unknown" {
  return value === "allow" || value === "deny" || value === "ask" ? value : "unknown";
}

async function collectDoctorRuntimeFacts(ctx: Plugin.Context, root: string, toolContext: ToolContext, bridge: NativePermissionBridge) {
  let effectiveTools: string[] | null = null;
  let plugins: any[] | null = null;
  const notes: string[] = [];
  try {
    effectiveTools = (await ctx.tool.list()).map((tool) => tool.id).filter((name) => name.startsWith("workplan_"));
  } catch {
    notes.push("Effective tool catalog is unavailable through the public plugin context.");
  }
  try {
    plugins = (await ctx.plugin.list(undefined, { signal: toolContext.signal })).data as any[];
  } catch {
    notes.push("Active plugin facts are unavailable through the public plugin context.");
  }
  const ownPlugin = plugins?.find((plugin) => plugin.id === PLUGIN_ID);
  const builtinPlan = plugins?.find((plugin) => plugin.id === "opencode.plan");
  const rules: Array<{ resource: string; decision: "allow" | "deny" | "ask" | "unknown"; source: string }> = [];
  let agentRead = false;
  let sessionRead = false;
  try {
    const agent = (await ctx.agent.get({ agentID: toolContext.agent } as any, { signal: toolContext.signal } as any)).data as any;
    agentRead = true;
    for (const rule of agent.permissions) rules.push({ resource: rule.resource, decision: permissionDecision(rule.effect), source: `agent:${rule.action}` });
  } catch {
    notes.push("Caller agent permission facts are unavailable.");
  }
  try {
    const session = await ctx.session.get({ sessionID: toolContext.sessionID } as any, { signal: toolContext.signal } as any) as any;
    if (await realpath(session.location.directory) === root) {
      sessionRead = true;
      for (const rule of session.permissions ?? []) rules.push({ resource: rule.resource, decision: permissionDecision(rule.effect), source: `session:${rule.action}` });
    } else {
      notes.push("The current session location differs from the canonical project; session rules are unknown.");
    }
  } catch {
    notes.push("Current session permission facts are unavailable.");
  }
  const bridgeFacts = bridge.diagnostics();
  notes.push("Effective permission remains unknown: public plugin APIs do not expose complete organization/hard-policy precedence.");
  if (effectiveTools === null) notes.push("Configured-only tool names are unknown; only a successful effective tool catalog is authoritative.");
  return {
    registrations: { effective: effectiveTools, configured: null },
    plugin: {
      id: PLUGIN_ID,
      configured: ownPlugin ? true : null,
      effective: plugins === null ? null : ownPlugin ? ownPlugin.state?.status === "active" : false,
      canonicalLocation: root,
    },
    permission: {
      status: "unknown",
      agent: toolContext.agent,
      sessionID: toolContext.sessionID,
      rules,
      detail: [
        `Agent rules ${agentRead ? "read" : "unknown"}; session rules ${sessionRead ? "read" : "unknown"}.`,
        `Bridge client ${bridgeFacts.clientVersion}, RPC ${bridgeFacts.rpcRegistration}, service ${bridgeFacts.serviceDiscovery}, host ${bridgeFacts.hostBinding}, event stream ${bridgeFacts.eventStream}, runtime ${bridgeFacts.runtimeVersion ?? "unknown"}, last failure ${bridgeFacts.lastFailure ?? "none"}.`,
        ...notes,
      ].join(" "),
    },
    builtinPlan: {
      configured: null,
      effective: plugins === null ? null : Boolean(builtinPlan && builtinPlan.state?.status === "active"),
    },
  };
}

export interface AdapterDeps {
  /** Core client factory (tests inject a fake transport). */
  readonly core?: (options: CoreClientOptions) => CoreClient;
  /** Permission bridge factory (tests inject fakes of the host domains). */
  readonly bridge?: (ctx: NativePermissionBridgeContext) => Promise<NativePermissionBridge>;
  readonly env?: NodeJS.ProcessEnv;
}

function progress(toolContext: ToolContext, toolName: string): void {
  const title = progressTitles[toolName];
  if (!title) return;
  try {
    void toolContext.progress({ status: title }).catch(() => {});
  } catch {
    // Progress is best-effort; it never authorizes or commits anything.
  }
}

function toolError(error: unknown): Error {
  if (error instanceof ShioriError) {
    const e = new Error(error.message) as Error & { errorClass?: string };
    e.errorClass = error.errorClass;
    return e;
  }
  return error instanceof Error ? error : new Error(String(error));
}

export function createPlugin(deps: AdapterDeps = {}) {
  return {
    id: PLUGIN_ID,
    async setup(ctx: Plugin.Context) {
      const hostVersion = ctx.app?.version;
      if (!(SUPPORTED_HOST_VERSIONS as readonly string[]).includes(hostVersion)) {
        throw new Error(`Shiori workplan adapter supports OpenCode ${SUPPORTED_HOST_VERSIONS.join(" and ")} only (this host reports ${String(hostVersion)}); no workplan tools were registered.`);
      }
      const root = await realpath(ctx.location.project.directory);
      const env = deps.env ?? process.env;
      const bin = typeof ctx.options?.bin === "string" ? ctx.options.bin : env.SHIORI_BIN;
      const coreOptions: CoreClientOptions = { bin, env, clientName: "shiori-opencode" };
      const core = deps.core ? deps.core(coreOptions) : new CoreClient(coreOptions);
      const bridge = await (deps.bridge ?? ((c) => registerNativePermissionBridge(c)))(ctx as unknown as NativePermissionBridgeContext);

      const execute = async (toolName: string, rawInput: unknown, toolContext: ToolContext): Promise<{ content: string }> => {
        assertRole(toolName, rawInput, toolContext.agent);
        const signal = toolContext.signal;
        const hostContext: HostContext = {
          mode: "native",
          canonicalRoot: root,
          sessionID: toolContext.sessionID,
          agent: toolContext.agent,
          messageID: toolContext.messageID,
          callID: toolContext.id,
          ...(toolName === "workplan_doctor" ? { runtimeFacts: await collectDoctorRuntimeFacts(ctx, root, toolContext, bridge) } : {}),
        };
        const input = rawInput === undefined ? {} : rawInput;
        if (!isRecord(input)) {
          // A non-object never reaches the core (its frame requires an object).
          const received = input === null ? "null" : Array.isArray(input) ? "array" : typeof input;
          throw new Error(`Invalid ${toolName.replace(/^workplan_/, "")} input: $: Invalid input: expected object, received ${received}`);
        }
        const prepareId = core.newRequestId();
        let response;
        try {
          response = await core.request(toolName, input, hostContext, signal, prepareId);
        } catch (error) {
          throw toolError(error);
        }
        if (response.result && typeof response.result.text === "string") {
          progress(toolContext, toolName);
          return { content: response.result.text };
        }
        const prepared = response.prepared;
        if (!prepared) throw new Error("The Shiori core returned neither a result nor a prepared intent; nothing was changed.");
        progress(toolContext, toolName);
        let committed = false;
        // An abort at any point expires the prepared intent in the core, so
        // a later approval can never commit it.
        const expire = () => core.cancel(prepareId);
        signal?.addEventListener?.("abort", expire, { once: true });
        try {
          validatePrepared(root, toolName, prepared);
          const resources = editResources(root, prepared);
          const trusted = {
            sessionID: toolContext.sessionID,
            agent: toolContext.agent,
            messageID: toolContext.messageID,
            toolCallID: toolContext.id,
            resources,
          };
          const receipt = await bridge.authorizeEdit(trusted, { signal });
          if (receipt.sessionID !== trusted.sessionID || receipt.agent !== trusted.agent ||
            receipt.source.messageID !== trusted.messageID || receipt.source.id !== trusted.toolCallID ||
            !sameResources(receipt.resources, resources)) {
            throw new Error("Native permission receipt no longer matches the trusted caller and exact write intent");
          }
          if (signal?.aborted) throw new ShioriError("cancelled", "The workplan permission request was cancelled; a later user reply cannot authorize this invocation.");
          committed = true;
          const result = await core.request("shiori.commit", {
            intentId: prepared.intentId,
            intentDigest: prepared.intentDigest,
            capability: prepared.capability,
          }, hostContext, signal);
          if (typeof result.result?.text !== "string") throw new Error("The Shiori core commit returned no result text");
          return { content: result.result.text };
        } catch (error) {
          if (!committed) {
            // Release the intent without side effects (best effort; the core
            // also expires it on cancel and disconnect).
            core.request("shiori.discard", { intentId: prepared.intentId, intentDigest: prepared.intentDigest }, hostContext).catch(() => {});
          }
          throw toolError(error);
        } finally {
          signal?.removeEventListener?.("abort", expire);
        }
      };

      let disposeTools: (() => Promise<void>) | undefined;
      try {
        const handle = await ctx.tool.transform((editor: ToolEditor) => {
          for (const tool of registration.tools) {
            editor.add({
              name: tool.name,
              description: tool.description,
              input: tool.input as any,
              options: { codemode: true },
              execute: (input: unknown, toolContext: ToolContext) => execute(tool.name, input, toolContext),
            } as any);
          }
        });
        disposeTools = () => handle.dispose();
      } catch (error) {
        await bridge.dispose();
        await core.dispose();
        throw error;
      }
      return async () => {
        try {
          await disposeTools?.();
        } finally {
          try {
            await bridge.dispose();
          } finally {
            await core.dispose();
          }
        }
      };
    },
  };
}
