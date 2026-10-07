package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/hoshinoht/shiori/internal/evidence"
	"github.com/hoshinoht/shiori/internal/index"
	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/storage"
)

// Evidence ledger (spec 06 X2): outside the state manifest, so it never
// changes planHash/stateHash; an invalid ledger degrades only its views.

func evidenceRel(id string) string { return snapshot.SidecarRel(id, evidence.Suffix) }

// evView is the ledger of a plan as the read surfaces see it.
type evView struct {
	exists bool
	rel    string
	ledger *evidence.Ledger // nil when absent or invalid
	issues []string
	art    snapshot.Artifact
}

// loadEvidence reads and decodes the ledger.
func (e *Engine) loadEvidence(id string) (evView, error) {
	v := evView{rel: evidenceRel(id)}
	r := &snapshot.Reader{Root: e.Root, Limits: e.Limits}
	a, err := r.ReadFile(v.rel)
	if err != nil {
		return v, err
	}
	v.art, v.exists = a, a.Exists
	if !a.Exists {
		return v, nil
	}
	parsed, perr := ojson.Parse(a.Bytes)
	if perr != nil {
		v.issues = []string{"Invalid evidence ledger JSON at " + a.Path + ": " + perr.Error()}
		return v, nil
	}
	l, issues := evidence.Decode(parsed.Value)
	if len(issues) > 0 {
		v.issues = issues
		return v, nil
	}
	if l.ID != id {
		v.issues = []string{"id: Evidence ledger belongs to " + l.ID + ", not " + id}
		return v, nil
	}
	v.ledger = l
	return v, nil
}

// currentTree is the working tree with the ledger's and extra scope paths.
func (e *Engine) currentTree(l *evidence.Ledger, extra []string) evidence.Current {
	scope := append([]string{}, extra...)
	if l != nil {
		scope = append(scope, l.ScopePaths()...)
	}
	scope = dedupe(scope)
	t, err := evidence.Snapshot(context.Background(), e.Root, snapshot.WorkplanDir, scope)
	return evidence.Current{Tree: t, Err: err}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// evidenceSummary counts step states and completed steps without fresh evidence.
type evidenceSummary struct {
	counts              map[string]int
	completedUnverified []model.StepRef
}

func summarizeEvidence(p *model.Plan, views map[model.StepRef]*evidence.StepView) evidenceSummary {
	s := evidenceSummary{counts: map[string]int{}}
	for i := range p.Phases {
		for j := range p.Phases[i].Steps {
			st := &p.Phases[i].Steps[j]
			ref := model.StepRef{PhaseID: p.Phases[i].ID, StepID: st.ID}
			state := evidence.StateNone
			if v := views[ref]; v != nil {
				state = v.State
				s.counts[state]++
			}
			if st.Status == "completed" && state != evidence.StateFresh {
				s.completedUnverified = append(s.completedUnverified, ref)
			}
		}
	}
	return s
}

func (s evidenceSummary) countsValue() ojson.Value {
	b := ojson.NewObject(4)
	for _, k := range []string{evidence.StateFresh, evidence.StateStale, evidence.StateFailing, evidence.StateUnknown} {
		b.Set(k, ojson.IntValue(int64(s.counts[k])))
	}
	return b.Value()
}

// orphanRecords counts records for steps no longer in the plan.
func orphanRecords(l *evidence.Ledger, ix *index.Plan) int {
	n := 0
	for _, r := range l.Records {
		if _, ok := ix.Step(index.StepKey{PhaseID: r.PhaseID, StepID: r.StepID}); !ok {
			n++
		}
	}
	return n
}

// doctorEvidence is doctor's per-plan evidence member.
func (e *Engine) doctorEvidence(p *model.Plan, ev evView) ojson.Value {
	b := ojson.NewObject(8).
		Set("path", ojson.StringValue(ev.rel)).
		Set("valid", ojson.BoolValue(ev.ledger != nil))
	if ev.ledger == nil {
		return b.Set("issues", ojson.StringsValue(ev.issues)).Value()
	}
	cur := e.currentTree(ev.ledger, nil)
	views := ev.ledger.Views(cur)
	sum := summarizeEvidence(p, views)
	refs := make([]ojson.Value, 0, len(sum.completedUnverified))
	for i, r := range sum.completedUnverified {
		if i >= evidenceListCap {
			break
		}
		refs = append(refs, refValue(r))
	}
	b.Set("records", ojson.IntValue(int64(len(ev.ledger.Records)))).
		Set("tree", cur.TreeValue()).
		Set("steps", sum.countsValue()).
		Set("completedUnverified", ojson.NewObject(2).
			Set("count", ojson.IntValue(int64(len(sum.completedUnverified)))).
			Set("steps", ojson.ArrayValue(refs)).Value())
	if n := orphanRecords(ev.ledger, index.Build(p)); n > 0 {
		b.Set("orphanRecords", ojson.IntValue(int64(n)))
	}
	return b.Value()
}

// evidenceListCap bounds listed step references.
const evidenceListCap = 20

// resumeEvidence is resume's compact evidence advice.
func resumeEvidence(p *model.Plan, views map[model.StepRef]*evidence.StepView, cur *index.StepKey) ojson.Value {
	sum := summarizeEvidence(p, views)
	b := ojson.NewObject(3).
		Set("steps", sum.countsValue()).
		Set("completedUnverified", ojson.IntValue(int64(len(sum.completedUnverified))))
	if cur != nil {
		state := evidence.StateNone
		if v := views[model.StepRef{PhaseID: cur.PhaseID, StepID: cur.StepID}]; v != nil {
			state = v.State
		}
		b.Set("current", ojson.StringValue(state))
	}
	return b.Value()
}

// evidenceRecords builds the recordEvidence records. A pinned EvidenceTree
// (the CLI's tree from before it ran the command) replaces the computed one.
func (e *Engine) evidenceRecords(data ojson.Value, p *model.Plan, stored *evidence.Ledger, now string) ([]evidence.Record, evidence.Current, error) {
	ix := index.Build(p)
	var out []evidence.Record
	var scopes []string
	for i, rv := range getObjs(data, "recordEvidence") {
		at := fmt.Sprintf("recordEvidence.%d", i)
		var r evidence.Record
		r.PhaseID, _ = getStr(rv, "phaseId")
		r.StepID, _ = getStr(rv, "stepId")
		if _, ok := ix.Step(index.StepKey{PhaseID: r.PhaseID, StepID: r.StepID}); !ok {
			if _, ok := ix.PhaseByID[r.PhaseID]; !ok {
				return nil, evidence.Current{}, fmt.Errorf("Phase not found: %s", r.PhaseID)
			}
			return nil, evidence.Current{}, fmt.Errorf("Step not found in phase %s: %s", r.PhaseID, r.StepID)
		}
		r.Command, _ = getStr(rv, "command")
		ec, _ := rv.Get("exitCode")
		f, _ := ec.Float()
		r.ExitCode = int64(f)
		if o, ok := getStr(rv, "output"); ok {
			sum := sha256.Sum256([]byte(o))
			d := hex.EncodeToString(sum[:])
			r.OutputDigest = &d
		}
		if d, ok := getStr(rv, "outputDigest"); ok {
			r.OutputDigest = &d
		}
		if s, ok := getStr(rv, "summary"); ok && !model.Blank(s) {
			t := model.TrimJS(s)
			r.Summary = &t
		}
		paths, _ := getList(rv, "scope")
		for j, raw := range paths {
			t := strings.TrimSuffix(model.TrimJS(raw), "/")
			rel, err := snapshot.NormalizeSpecFile(e.Root, t)
			if err != nil || t == "" || rel == "" || rel == "." {
				return nil, evidence.Current{}, fmt.Errorf("Invalid evidence input: %s.scope.%d: Evidence scope must be a path inside the workspace root: %s", at, j, raw)
			}
			if rel == snapshot.WorkplanDir || strings.HasPrefix(rel, snapshot.WorkplanDir+"/") {
				return nil, evidence.Current{}, fmt.Errorf("Invalid evidence input: %s.scope.%d: Evidence scope cannot name the workplan directory (its files are not code under test): %s", at, j, raw)
			}
			r.Scope = append(r.Scope, evidence.ScopeEntry{Path: rel})
			scopes = append(scopes, rel)
		}
		r.Source = e.evidenceSource()
		r.RecordedAt = now
		out = append(out, r)
	}
	cur := e.currentTree(stored, scopes)
	recTree := cur.Tree
	if e.EvidenceTree != nil {
		recTree = e.EvidenceTree
	}
	if recTree != nil {
		for i := range out {
			oid := recTree.OID
			out[i].TreeOID = &oid
			for k := range out[i].Scope {
				o, ok := recTree.Scope[out[i].Scope[k].Path]
				if !ok {
					return nil, evidence.Current{}, fmt.Errorf("pinned evidence tree has no entry for scope %s", out[i].Scope[k].Path)
				}
				out[i].Scope[k].Digest = o
			}
		}
	}
	return out, cur, nil
}

// evidenceSource is the trusted surface's record source.
func (e *Engine) evidenceSource() string {
	if e.EvidenceSource != "" {
		return e.EvidenceSource
	}
	return evidence.SourceAgent
}

// evidenceOnly reports an update that only records evidence; it writes
// the ledger alone and leaves the plan and its hashes untouched.
func evidenceOnly(data ojson.Value) bool {
	if !has(data, "recordEvidence") {
		return false
	}
	for _, m := range data.Members() {
		switch m.Key {
		case "id", "expectedHash", "workspaceRoot", "recordEvidence":
		case "replaceMarkdown":
			if m.Value.Bool() {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// msgEvidenceInvalid refuses to overwrite a ledger that does not decode.
func msgEvidenceInvalid(ev evView) error {
	return fmt.Errorf("Cannot record evidence: the evidence ledger at %s is invalid (%s). Fix or move it aside first; it is never overwritten", ev.art.Path, strings.Join(ev.issues, "; "))
}

// updateEvidence returns the new ledger bytes (nil: unchanged), the result
// member and warnings for steps completed without fresh evidence. Plans
// that never recorded evidence get no warnings.
func (e *Engine) updateEvidence(id string, data ojson.Value, old, p *model.Plan) (evView, []byte, *ojson.Value, []string, error) {
	ev, err := e.loadEvidence(id)
	if err != nil {
		return ev, nil, nil, nil, err
	}
	recording := has(data, "recordEvidence")
	var completed []model.StepRef
	oldIx := index.Build(old)
	for i := range p.Phases {
		for j := range p.Phases[i].Steps {
			st := &p.Phases[i].Steps[j]
			if st.Status != "completed" {
				continue
			}
			prev, ok := oldIx.Step(index.StepKey{PhaseID: p.Phases[i].ID, StepID: st.ID})
			if !ok || prev.Status != "completed" {
				completed = append(completed, model.StepRef{PhaseID: p.Phases[i].ID, StepID: st.ID})
			}
		}
	}
	if !recording && (!ev.exists || len(completed) == 0) {
		return ev, nil, nil, nil, nil
	}
	if !recording && ev.ledger == nil {
		return ev, nil, nil, nil, nil // an invalid ledger degrades only the warnings
	}
	if recording && ev.exists && ev.ledger == nil {
		return ev, nil, nil, nil, msgEvidenceInvalid(ev)
	}
	now := e.nowISO()
	var recs []evidence.Record
	var cur evidence.Current
	if recording {
		if recs, cur, err = e.evidenceRecords(data, p, ev.ledger, now); err != nil {
			return ev, nil, nil, nil, err
		}
	} else {
		cur = e.currentTree(ev.ledger, nil)
	}
	l := &evidence.Ledger{ID: id}
	if ev.ledger != nil {
		l.Records = ev.ledger.Records
	}
	var after []byte
	var result *ojson.Value
	if recording {
		l.Append(recs...)
		l.UpdatedAt = now
		after = evidence.Encode(l)
		rv := make([]ojson.Value, len(recs))
		for i, r := range recs {
			rv[i] = ojson.NewObject(6).
				Set("phaseId", ojson.StringValue(r.PhaseID)).
				Set("stepId", ojson.StringValue(r.StepID)).
				Set("command", ojson.StringValue(r.Command)).
				Set("exitCode", ojson.IntValue(r.ExitCode)).
				Set("state", ojson.StringValue(cur.RecordState(r))).
				Set("treeOid", ojson.NullableString(r.TreeOID)).Value()
		}
		v := ojson.NewObject(4).
			Set("path", ojson.StringValue(ev.rel)).
			Set("recorded", ojson.ArrayValue(rv)).
			Set("tree", cur.TreeValue()).Value()
		result = &v
	}
	var warnings []string
	if len(completed) > 0 {
		views := l.Views(cur)
		for i, ref := range completed {
			if i >= evidenceListCap {
				warnings = append(warnings, fmt.Sprintf("%d more steps were completed without fresh passing evidence", len(completed)-i))
				break
			}
			state := evidence.StateNone
			if v := views[ref]; v != nil {
				state = v.State
			}
			if state != evidence.StateFresh {
				warnings = append(warnings, fmt.Sprintf("Step %s/%s is completed without fresh passing evidence (%s); record its validation command with recordEvidence", ref.PhaseID, ref.StepID, state))
			}
		}
	}
	return ev, after, result, warnings, nil
}

// prepareEvidenceOnly prepares an update that only records evidence.
func (e *Engine) prepareEvidenceOnly(s *snapshot.Snapshot, data ojson.Value) (*Prepared, error) {
	id := s.ID
	ev, after, evResult, _, err := e.updateEvidence(id, data, s.Plan, s.Plan)
	if err != nil {
		return nil, err
	}
	specs := []targetSpec{{rel: ev.rel, kind: "evidence", before: ev.art.Bytes, beforeOK: ev.exists, after: after, afterOK: true, forceWrite: true}}
	tx := storage.NewUUID()
	in := e.buildIntent("update", id, tx, specs, readsOf(s.StateManifest))
	if err := e.checkTargetPaths(in); err != nil {
		return nil, err
	}
	prep := &Prepared{Tool: "workplan_update", Intent: in}
	prep.result = func(sync bool) (Output, error) {
		b := ojson.NewObject(8).
			Set("updated", ojson.BoolValue(true)).
			Set("path", ojson.StringValue(e.absRel(s.JSON.Rel))).
			Set("planPath", ojson.StringValue(e.absRel(s.Plan.PlanFile))).
			Set("workplan", s.Plan.Summary()).
			Set("planHash", ojson.StringValue(s.PlanHash)).
			Set("stateHash", ojson.StringValue(s.StateHash)).
			Set("directorySync", dirSyncValue(sync)).
			Set("evidence", *evResult)
		return Output{Value: b.Value()}, nil
	}
	return finalize(prep), nil
}
