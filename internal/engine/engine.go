// Package engine implements the read-only workplan operations (list, read,
// inspect, validate, resume, doctor) over byte-exact snapshots. Every
// operation opens artifacts read-only; none writes defaults, upgrades a
// format, repairs, creates locks or changes a file's mtime (spec 01 §3,
// spec 05 stage B exit gate).
package engine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/hoshinoht/shiori/internal/index"
	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
)

// Engine runs read-only operations for one trusted project root.
type Engine struct {
	RequestedRoot string // as supplied by the trusted caller (absolute)
	Root          string // canonical (symlinks resolved)
	Limits        snapshot.Limits
	// MaxResponseBytes bounds a single operation's serialized output
	// (contracts §5.2, D9). Zero means the default 64 MiB.
	MaxResponseBytes int

	// noGraph turns off the D.2 dependency-graph additions (contracts
	// §12). Test-only: the corpus comparators prove that a D.2 output
	// differs from the D.2-off output only by the approved change.
	noGraph bool
}

// DefaultMaxResponseBytes is the approved response frame limit.
const DefaultMaxResponseBytes = 64 << 20

// New resolves the root and returns an engine with default limits.
func New(root string) (*Engine, error) {
	req, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	canon, err := snapshot.CanonicalRoot(req)
	if err != nil {
		return nil, err
	}
	return &Engine{RequestedRoot: req, Root: canon, Limits: snapshot.DefaultLimits}, nil
}

func (e *Engine) maxResponse() int {
	if e.MaxResponseBytes > 0 {
		return e.MaxResponseBytes
	}
	return DefaultMaxResponseBytes
}

func (e *Engine) dir() string {
	return filepath.Join(e.Root, filepath.FromSlash(snapshot.WorkplanDir))
}

func (e *Engine) absRel(rel string) string { return filepath.Join(e.Root, filepath.FromSlash(rel)) }

func (e *Engine) load(id string) (*snapshot.Snapshot, error) {
	return snapshot.Load(e.Root, id, e.Limits)
}

// normalizeRequested normalizes a caller-supplied id (inputs are already
// validated to contain [A-Za-z0-9]).
func normalizeRequested(raw string) (string, error) { return model.NormalizeID(raw) }

// depView is the decoded dependency sidecar plus its issues.
type depView struct {
	recorded bool
	deps     *model.Dependencies
	issues   []string // unprefixed "path: message"
}

// dependencies decodes the sidecar; ix may be nil and is then built only
// when a sidecar exists.
func (e *Engine) dependencies(s *snapshot.Snapshot, ix *index.Plan) depView {
	v := depView{recorded: s.Dependencies.Exists}
	if !v.recorded {
		return v
	}
	if ix == nil {
		ix = index.Build(s.Plan)
	}
	parsed, err := ojson.Parse(s.Dependencies.Bytes)
	if err != nil {
		v.issues = []string{"Invalid workplan dependencies JSON at " + s.Dependencies.Path + ": " + err.Error()}
		return v
	}
	d, issues := model.DecodeDependencies(parsed.Value)
	if len(issues) > 0 {
		for _, is := range issues {
			v.issues = append(v.issues, is.String())
		}
		return v
	}
	v.deps = d
	v.issues = index.ValidateDependencies(ix, d)
	return v
}

func refValue(r model.StepRef) ojson.Value {
	return ojson.NewObject(2).
		Set("phaseId", ojson.StringValue(r.PhaseID)).
		Set("stepId", ojson.StringValue(r.StepID)).Value()
}

func (v depView) value() ojson.Value {
	deps := []ojson.Value{}
	terms := []ojson.Value{}
	if v.deps != nil {
		for _, en := range v.deps.Entries {
			on := make([]ojson.Value, len(en.DependsOn))
			for i, r := range en.DependsOn {
				on[i] = refValue(r)
			}
			deps = append(deps, ojson.NewObject(3).
				Set("phaseId", ojson.StringValue(en.PhaseID)).
				Set("stepId", ojson.StringValue(en.StepID)).
				Set("dependsOn", ojson.ArrayValue(on)).Value())
		}
		for _, ts := range v.deps.TerminalSummaries {
			terms = append(terms, ojson.NewObject(4).
				Set("phaseId", ojson.StringValue(ts.PhaseID)).
				Set("stepId", ojson.StringValue(ts.StepID)).
				Set("title", ojson.StringValue(ts.Title)).
				Set("status", ojson.StringValue(ts.Status)).Value())
		}
	}
	issues := v.issues
	if issues == nil {
		issues = []string{}
	}
	return ojson.NewObject(4).
		Set("recorded", ojson.BoolValue(v.recorded)).
		Set("dependencies", ojson.ArrayValue(deps)).
		Set("terminalSummaries", ojson.ArrayValue(terms)).
		Set("issues", ojson.StringsValue(issues)).Value()
}

// sliceValue is value() restricted to entries whose source step is
// selected or that depend on a selected step, and the terminal summaries
// those entries reference (D.1 filtered read). Issues stay complete.
func (v depView) sliceValue(sel map[model.StepRef]bool) ojson.Value {
	if v.deps == nil {
		return v.value()
	}
	d := *v.deps
	d.Entries = nil
	refs := map[model.StepRef]bool{}
	for _, en := range v.deps.Entries {
		keep := sel[model.StepRef{PhaseID: en.PhaseID, StepID: en.StepID}]
		for _, r := range en.DependsOn {
			keep = keep || sel[r]
		}
		if keep {
			d.Entries = append(d.Entries, en)
			for _, r := range en.DependsOn {
				refs[r] = true
			}
		}
	}
	d.TerminalSummaries = nil
	for _, ts := range v.deps.TerminalSummaries {
		if refs[model.StepRef{PhaseID: ts.PhaseID, StepID: ts.StepID}] || sel[model.StepRef{PhaseID: ts.PhaseID, StepID: ts.StepID}] {
			d.TerminalSummaries = append(d.TerminalSummaries, ts)
		}
	}
	v.deps = &d
	return v.value()
}

// Checkpoint freshness classes (contracts §7).
const (
	FreshnessMissing = "missing"
	FreshnessFresh   = "fresh"
	FreshnessStale   = "stale"
	FreshnessLegacy  = "legacy-unverified"
	FreshnessInvalid = "invalid"
	msgStale         = "Checkpoint does not match the current JSON/Markdown/spec manifest"
	msgLegacy        = "Checkpoint v1 only hashes JSON; multiartifact freshness is unverified"
)

// cpView classifies the checkpoint sidecar. Reads never upgrade it.
type cpView struct {
	exists    bool
	freshness string
	cp        *model.Checkpoint
	// issue is the doctor text after "checkpoint: "; diagnostic is the
	// resume diagnostic for an invalid checkpoint.
	issue      string
	diagnostic string
}

func classifyCheckpoint(s *snapshot.Snapshot) cpView {
	v := cpView{exists: s.Checkpoint.Exists, freshness: FreshnessMissing}
	if !v.exists {
		return v
	}
	parsed, err := ojson.Parse(s.Checkpoint.Bytes)
	if err != nil {
		v.freshness = FreshnessInvalid
		v.issue = err.Error()
		v.diagnostic = "Invalid workplan checkpoint JSON at " + s.Checkpoint.Path + ": " + err.Error()
		return v
	}
	cp, ok := model.DecodeCheckpoint(parsed.Value)
	if !ok {
		v.freshness = FreshnessInvalid
		v.issue = model.ErrCheckpointSchema
		v.diagnostic = "Invalid workplan checkpoint document at " + s.Checkpoint.Path + ": " + model.ErrCheckpointSchema
		return v
	}
	v.cp = cp
	switch {
	case cp.SchemaVersion == 1:
		v.freshness = FreshnessLegacy
		v.issue = msgLegacy
	case cp.PlanHash == s.PlanHash:
		v.freshness = FreshnessFresh
	default:
		v.freshness = FreshnessStale
		v.issue = msgStale
	}
	return v
}

// journalRel is the pending journal path for an id.
func journalRel(id string) string { return snapshot.SidecarRel(id, ".transaction.json") }

// recoveryPacket is the reference's short-circuit output for a plan with a
// pending journal (read/inspect/resume).
func recoveryPacket(s *snapshot.Snapshot, tool string) ojson.Value {
	b := ojson.NewObject(5).
		Set("recoveryRequired", ojson.BoolValue(true)).
		Set("journalPath", ojson.StringValue(journalRel(s.ID))).
		Set("planHash", ojson.StringValue(s.PlanHash)).
		Set("stateHash", ojson.StringValue(s.StateHash))
	switch tool {
	case "read":
		b.Set("workplan", ojson.NullValue())
	case "resume":
		b.Set("planFresh", ojson.BoolValue(false))
	}
	return b.Value()
}

// errUnsupported builds the bounded-output refusal (D9).
func (e *Engine) checkResponse(v ojson.Value, tool, id string) (ojson.Value, error) {
	if len(ojson.Pretty(v)) > e.maxResponse() {
		return ojson.Value{}, fmt.Errorf("%w: %s output for %s exceeds the %d-byte response limit; use includeMarkdown=false, workplan_inspect or workplan_resume", snapshot.ErrUnsupported, tool, id, e.maxResponse())
	}
	return v, nil
}

// IsUnsupported reports an unsupported_capability error.
func IsUnsupported(err error) bool { return errors.Is(err, snapshot.ErrUnsupported) }
