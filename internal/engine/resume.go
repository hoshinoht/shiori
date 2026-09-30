package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hoshinoht/shiori/internal/index"
	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
)

// Resume defaults and bounds (spec 01 §8).
const (
	DefaultResumeMaxChars = 12000
	DefaultResumeLimit    = 20
	MinResumeMaxChars     = 4096
	MaxResumeMaxChars     = 64000
)

const (
	instructionFresh = "Start from the current step and recorded dependencies; recentValidation is evidence text marked unverified."
	instructionStale = "Checkpoint guidance is not fresh. Reconfirm unverified guardrails, blockers and references before relying on them; nextAction is withheld."
)

type currentView struct {
	phaseID, phaseTitle, phaseStatus string
	stepID, stepTitle, stepStatus    string
	target, action, validation       *string
	readiness                        readinessView
}

// readinessView is the D.2 readiness of an open step (contracts §12
// G1/G6), shown only when the plan has a valid dependency sidecar.
type readinessView struct {
	shown     bool
	ready     bool
	blockedBy []index.Prereq // prerequisites not completed, stored order
	unblocks  int            // open steps held up, transitively
}

func readinessOf(g *index.Graph, k index.StepKey) readinessView {
	if g == nil {
		return readinessView{}
	}
	unmet := g.Unmet(k)
	return readinessView{shown: true, ready: len(unmet) == 0, blockedBy: unmet, unblocks: g.Unblocks(k)}
}

// set adds readiness members compactly: a ready step carries how many
// open steps it unblocks, a blocked one its unmet prerequisites (ids and
// status only, at most the pinned-list cap, with the omitted count).
func (r readinessView) set(b *ojson.Builder, listCap int) {
	if !r.shown {
		return
	}
	if r.ready {
		b.Set("readiness", ojson.StringValue("ready")).
			Set("unblocks", ojson.IntValue(int64(r.unblocks)))
		return
	}
	n := shown(len(r.blockedBy), listCap)
	refs := make([]ojson.Value, n)
	for i := 0; i < n; i++ {
		refs[i] = stepRefStatus(r.blockedBy[i].Key, r.blockedBy[i].Status)
	}
	b.Set("readiness", ojson.StringValue("blocked")).
		Set("blockedBy", ojson.ArrayValue(refs))
	if o := len(r.blockedBy) - n; o > 0 {
		b.Set("blockedByOmitted", ojson.IntValue(int64(o)))
	}
}

type findingView struct {
	index           int
	severity, title string
	detail, source  *string
	status          string
	metadataOmitted bool
}

type pageItem struct {
	kind string // active-work | finding | reference
	// active-work
	phaseID, phaseTitle, phaseStatus, stepID, title, status string
	target, action, validation                              *string
	readiness                                               readinessView
	// finding
	finding findingView
	// reference
	reference, source string
}

// resumeModel is the complete, untruncated continuation state.
type resumeModel struct {
	path, planFile      string
	planHash, stateHash string
	planFresh           bool
	cpExists, cpFresh   bool
	freshness           string
	sourceUpdatedAt     *string
	summary, nextAction *string
	current             *currentView
	blockers            []string
	blockersTotal       int
	guardrails          []string // shown candidates (withheld when not fresh)
	guardrailsTotal     int
	references          []string
	referencesTotal     int
	recentValidation    []string
	diagnostic          *string
	id                  string
	title               *string
	goal, status        string
	scope, nonGoals     []string
	constraints         []string
	relevantFiles       []string
	updatedAt           string
	depsRecorded        bool
	depsValid           bool
	currentDeps         []model.StepRef
	critical            *criticalView // D.3.1 compact critical path
	high                []findingView
	highCounts          [3]int
	warnings            []string
	items               []pageItem
	offset, limit       int
	maxChars            int
	phaseFilter         *string
	stepFilter          *string
	activeWorkTotal     int
	findingsTotal       int
	referencesPageTotal int
	historicalNotes     int
	resolvedFindings    int
	instruction         string
}

// criticalView is resume's compact critical path (D.3.1, contracts §14
// item 4): its length and the first open step on it. The full path stays
// in workplan_inspect and workplan_doctor.
type criticalView struct {
	length int
	next   index.StepKey
}

// resumeParams is one degradation level. Caps are UTF-16 code units
// including the trailing ellipsis; 0 means the class is not truncated.
type resumeParams struct {
	listCap   int // pinned list length cap
	titleCap  int // titles (plan, phase, step, finding)
	longCap   int // goal, page-item target/action/validation, list entries
	pinnedCap int // current work: summary, next action, current step prose
	protCap   int // file paths, references, instruction
	items     int // page items to include
	compact   bool
}

// Resume implements workplan_resume: a bounded continuation packet whose
// complete serialized text (UTF-16 code units) never exceeds maxChars.
func (e *Engine) Resume(in ResumeInput) (ojson.Value, string, error) {
	id, err := normalizeRequested(in.ID)
	if err != nil {
		return ojson.Value{}, "", err
	}
	s, err := e.load(id)
	if err != nil {
		return ojson.Value{}, "", err
	}
	if s.Journal.Exists {
		v := recoveryPacket(s, "resume")
		return v, string(ojson.Pretty(v)), nil
	}
	if err := model.UniqueIDError(s.Plan); err != nil {
		return ojson.Value{}, "", err
	}
	m, err := e.resumeModel(s, in)
	if err != nil {
		return ojson.Value{}, "", err
	}
	v, text, err := chooseResume(m)
	return v, text, err
}

func (e *Engine) resumeModel(s *snapshot.Snapshot, in ResumeInput) (*resumeModel, error) {
	p := s.Plan
	ix := index.Build(p)
	m := &resumeModel{
		path: s.JSON.Path, planFile: p.PlanFile,
		planHash: s.PlanHash, stateHash: s.StateHash,
		planFresh: len(s.MissingPlanArtifacts) == 0,
		id:        p.ID, title: p.Title, goal: p.Goal, status: p.Status,
		scope: p.Scope, nonGoals: p.NonGoals, constraints: p.Constraints,
		relevantFiles: p.RelevantFiles, updatedAt: p.UpdatedAt,
		maxChars: DefaultResumeMaxChars, limit: DefaultResumeLimit,
		phaseFilter: in.PhaseID, stepFilter: in.StepID,
		historicalNotes: len(p.Notes),
	}
	if in.MaxChars != nil {
		m.maxChars = *in.MaxChars
	}
	if in.Limit != nil {
		m.limit = *in.Limit
	}

	// Filters.
	phaseIdx := -1
	if in.PhaseID != nil {
		i, ok := ix.PhaseByID[*in.PhaseID]
		if !ok {
			return nil, fmt.Errorf("Phase not found: %s", *in.PhaseID)
		}
		phaseIdx = i
	}
	if in.StepID != nil {
		n := 0
		for i := range p.Phases {
			if phaseIdx >= 0 && i != phaseIdx {
				continue
			}
			for j := range p.Phases[i].Steps {
				if p.Phases[i].Steps[j].ID == *in.StepID {
					n++
				}
			}
		}
		if n != 1 {
			return nil, fmt.Errorf("Step filter must identify exactly one step: %s", *in.StepID)
		}
	}

	// Checkpoint.
	cv := classifyCheckpoint(s)
	m.cpExists = cv.exists
	m.freshness = cv.freshness
	m.cpFresh = cv.freshness == FreshnessFresh
	var warnings []string
	// D.2 cancelled-prerequisite warnings go after the checkpoint issue
	// (when there is one) and before the unverified checkpoint lines, so
	// the pinned list cap does not hide current plan facts behind them.
	d2At := 0
	if cv.cp != nil {
		sua := cv.cp.SourceUpdatedAt
		m.sourceUpdatedAt = &sua
		m.blockers = cv.cp.Blockers
		m.blockersTotal = len(cv.cp.Blockers)
		m.guardrailsTotal = len(cv.cp.Guardrails)
		m.referencesTotal = len(cv.cp.References)
		if m.cpFresh {
			sum, next := cv.cp.Summary, cv.cp.NextAction
			m.summary, m.nextAction = &sum, &next
			m.guardrails = cv.cp.Guardrails
			m.references = cv.cp.References
			m.recentValidation = cv.cp.RecentValidation
		} else {
			warnings = append(warnings, cv.issue)
			d2At = 1
			// D.3.1 (contracts §14 item 3): a stale checkpoint names what
			// changed, compactly (the doctor issue has the full detail).
			if cv.freshness == FreshnessStale && !e.noD31 {
				d := staleDiagnostic(cv.staleChanged)
				m.diagnostic = &d
			}
			for _, g := range cv.cp.Guardrails {
				warnings = append(warnings, "Unverified checkpoint guardrail: "+g)
			}
			for _, b := range cv.cp.Blockers {
				warnings = append(warnings, "Unverified checkpoint blocker: "+b)
			}
			for _, r := range cv.cp.References {
				warnings = append(warnings, "Unverified checkpoint reference: "+r)
			}
		}
	} else if cv.freshness == FreshnessInvalid {
		d := cv.diagnostic
		m.diagnostic = &d
		warnings = append(warnings, d)
		d2At = 1
	}
	if m.cpFresh {
		m.instruction = instructionFresh
	} else {
		m.instruction = instructionStale
	}

	// Current step: a fresh checkpoint's recorded position when it still
	// resolves; otherwise the first in-progress step, else the first
	// unfinished step. Stale guidance never drives it.
	var cur *index.StepKey
	if m.cpFresh && cv.cp.Current != nil {
		k := index.StepKey{PhaseID: cv.cp.Current.PhaseID, StepID: cv.cp.Current.StepID}
		if _, ok := ix.StepByKey[k]; ok {
			cur = &k
		}
	}
	if cur == nil {
		cur = firstActive(p)
	}
	if cur != nil {
		loc := ix.StepByKey[*cur]
		ph := &p.Phases[loc.Phase]
		st := &ph.Steps[loc.Step]
		m.current = &currentView{
			phaseID: ph.ID, phaseTitle: ph.Title, phaseStatus: ph.Status,
			stepID: st.ID, stepTitle: st.Title, stepStatus: st.Status,
			target: st.Target, action: st.Action, validation: st.Validation,
		}
	}

	// Dependencies.
	dv := e.dependencies(s, ix)
	m.depsRecorded = dv.recorded
	m.depsValid = len(dv.issues) == 0
	for _, is := range dv.issues {
		warnings = append(warnings, "Dependency metadata warning: "+is)
	}
	if dv.deps != nil && cur != nil {
		for _, en := range dv.deps.Entries {
			if en.PhaseID == cur.PhaseID && en.StepID == cur.StepID {
				m.currentDeps = append(m.currentDeps, en.DependsOn...)
			}
		}
	}
	// D.2 (G1/G3/G6): readiness of the current step and the open items;
	// open steps behind a cancelled prerequisite are flagged.
	g := e.graph(ix, dv)
	if g != nil {
		if cur != nil {
			m.current.readiness = readinessOf(g, *cur)
		}
		var d2 []string
		for _, k := range g.Steps() {
			st, _ := g.Status(k)
			if !index.IsOpen(st) {
				continue
			}
			if c := cancelledOf(g.Unmet(k)); len(c) > 0 {
				d2 = append(d2, "Dependency order warning: "+strings.TrimPrefix(cancelledWarning(k, c), "dependencies: "))
			}
		}
		if len(d2) > 0 {
			warnings = append(warnings[:d2At:d2At], append(d2, warnings[d2At:]...)...)
		}
		// D.3.1 (contracts §14 item 4): the critical path's length and the
		// first open step on it, when it chains at least two open steps.
		if !e.noD31 {
			if cp := g.Critical(); len(cp.Steps) >= 2 {
				for _, k := range cp.Steps {
					if st, _ := g.Status(k); index.IsOpen(st) {
						m.critical = &criticalView{length: len(cp.Steps), next: k}
						break
					}
				}
			}
		}
	}
	m.warnings = warnings

	// Findings.
	buckets := index.BuildBuckets(p)
	m.resolvedFindings = buckets.Resolved
	for _, i := range buckets.High() {
		m.high = append(m.high, findingOf(p, i))
		m.highCounts[index.SeverityRank[p.Findings[i].Severity]]++
	}
	low := buckets.Low()
	m.findingsTotal = len(m.high) + len(low)

	// Page: active work (excluding current), low findings, references.
	activeFiltered := 0
	for i := range p.Phases {
		if phaseIdx >= 0 && i != phaseIdx {
			continue
		}
		ph := &p.Phases[i]
		for j := range ph.Steps {
			st := &ph.Steps[j]
			if !isActive(st.Status) {
				continue
			}
			if in.StepID != nil && st.ID != *in.StepID {
				continue
			}
			if cur != nil && ph.ID == cur.PhaseID && st.ID == cur.StepID {
				continue
			}
			activeFiltered++
			m.items = append(m.items, pageItem{kind: "active-work",
				phaseID: ph.ID, phaseTitle: ph.Title, phaseStatus: ph.Status,
				stepID: st.ID, title: st.Title, status: st.Status,
				target: st.Target, action: st.Action, validation: st.Validation,
				readiness: readinessOf(g, index.StepKey{PhaseID: ph.ID, StepID: st.ID})})
		}
	}
	if g != nil {
		// D.2 (G1/G6): ready work first, ranked by how many open steps it
		// unblocks (ties keep plan order), then blocked work in plan order.
		sort.SliceStable(m.items, func(i, j int) bool {
			a, b := m.items[i].readiness, m.items[j].readiness
			if a.ready != b.ready {
				return a.ready
			}
			return a.ready && a.unblocks > b.unblocks
		})
	}
	m.activeWorkTotal = activeFiltered
	if cur != nil {
		m.activeWorkTotal++
	}
	for _, i := range low {
		m.items = append(m.items, pageItem{kind: "finding", finding: findingOf(p, i)})
	}
	for _, r := range p.RelevantFiles {
		m.items = append(m.items, pageItem{kind: "reference", reference: r, source: "current-plan"})
	}
	m.referencesPageTotal = len(p.RelevantFiles)
	if m.cpFresh {
		for _, r := range cv.cp.References {
			m.items = append(m.items, pageItem{kind: "reference", reference: r, source: "checkpoint"})
		}
		m.referencesPageTotal += len(cv.cp.References)
	}

	// Cursor.
	if in.Cursor != nil {
		c, err := parseResumeCursor(*in.Cursor)
		if err != nil {
			return nil, err
		}
		if c.stateHash != s.StateHash || c.maxChars != m.maxChars || c.limit != m.limit ||
			!eqPtr(c.phaseID, filterDigest(in.PhaseID)) || !eqPtr(c.stepID, filterDigest(in.StepID)) || c.offset > len(m.items) {
			return nil, errResumeCursorStale
		}
		m.offset = c.offset
	}
	return m, nil
}

func isActive(status string) bool { return status != "completed" && status != "cancelled" }

func firstActive(p *model.Plan) *index.StepKey {
	for i := range p.Phases {
		for j := range p.Phases[i].Steps {
			if p.Phases[i].Steps[j].Status == "in_progress" {
				return &index.StepKey{PhaseID: p.Phases[i].ID, StepID: p.Phases[i].Steps[j].ID}
			}
		}
	}
	for i := range p.Phases {
		for j := range p.Phases[i].Steps {
			if isActive(p.Phases[i].Steps[j].Status) {
				return &index.StepKey{PhaseID: p.Phases[i].ID, StepID: p.Phases[i].Steps[j].ID}
			}
		}
	}
	return nil
}

func findingOf(p *model.Plan, i int) findingView {
	f := &p.Findings[i]
	status := "open"
	if f.Status != nil {
		status = *f.Status
	}
	return findingView{index: i, severity: f.Severity, title: f.Title, detail: f.Detail, source: f.Source,
		status: status, metadataOmitted: len(f.Unknown) > 0}
}

// textClass selects the display cap of a packet string (D.1, contracts
// §11). Protected strings are file paths, references and the fixed
// instruction; ids, hashes, enums, counts and retrieval pointers are never
// passed through truncation at all.
type textClass int

const (
	clsProtected textClass = iota
	clsTitle
	clsLong
	clsPinned // current work: summary, next action, current target/action/validation
)

// truncState records truncated display fields in traversal order.
type truncState struct {
	caps        [4]int // per textClass; 0 = no cap
	danger      []string
	other       []string
	dangerCount int
}

func (t *truncState) str(path, s string, cls textClass, danger bool) ojson.Value {
	c := t.caps[cls]
	if c <= 0 {
		return ojson.StringValue(s)
	}
	out, cut := ojson.TruncateUTF16(s, c)
	if cut {
		t.record(path, danger)
	}
	return ojson.StringValue(out)
}

func (t *truncState) ptr(path string, s *string, cls textClass, danger bool) ojson.Value {
	if s == nil {
		return ojson.NullValue()
	}
	return t.str(path, *s, cls, danger)
}

func (t *truncState) record(path string, danger bool) {
	if danger {
		t.danger = append(t.danger, path)
		t.dangerCount++
	} else {
		t.other = append(t.other, path)
	}
}

func (t *truncState) list(path string, items []string, cap int, cls textClass, danger bool) ojson.Value {
	n := len(items)
	if n > cap {
		n = cap
	}
	out := make([]ojson.Value, n)
	for i := 0; i < n; i++ {
		out[i] = t.str(path+"["+strconv.Itoa(i)+"]", items[i], cls, danger)
	}
	return ojson.ArrayValue(out)
}

func shown(total, cap int) int {
	if total > cap {
		return cap
	}
	return total
}

func (t *truncState) finding(path string, f findingView, withKind bool, danger bool) ojson.Value {
	b := ojson.NewObject(9)
	if withKind {
		b.Set("kind", ojson.StringValue("finding"))
	}
	b.Set("index", ojson.IntValue(int64(f.index))).
		Set("severity", ojson.StringValue(f.severity)).
		Set("title", t.str(path+".title", f.title, clsTitle, danger)).
		Set("detail", t.ptr(path+".detail", f.detail, clsLong, danger)).
		Set("source", t.ptr(path+".source", f.source, clsLong, danger)).
		Set("status", ojson.StringValue(f.status)).
		Set("metadataOmitted", ojson.BoolValue(f.metadataOmitted))
	if f.metadataOmitted {
		t.record(path+".customMetadata", danger)
	}
	return b.Value()
}

// build renders the packet for one degradation level.
func (m *resumeModel) build(pr resumeParams) ojson.Value {
	t := &truncState{caps: [4]int{pr.protCap, pr.titleCap, pr.longCap, pr.pinnedCap}}
	L := pr.listCap

	pathV := t.str("path", m.path, clsProtected, false)
	planFileV := t.str("planFile", m.planFile, clsProtected, false)
	// The summary is truncated (and listed) before the current position.
	summaryV := t.ptr("checkpoint.summary", m.summary, clsPinned, false)
	var current ojson.Value
	if m.current == nil {
		current = ojson.NullValue()
	} else {
		c := m.current
		cb := ojson.NewObject(12).
			Set("phaseId", ojson.StringValue(c.phaseID)).
			Set("phaseTitle", t.str("checkpoint.current.phaseTitle", c.phaseTitle, clsTitle, false)).
			Set("phaseStatus", ojson.StringValue(c.phaseStatus)).
			Set("stepId", ojson.StringValue(c.stepID)).
			Set("stepTitle", t.str("checkpoint.current.stepTitle", c.stepTitle, clsTitle, false)).
			Set("stepStatus", ojson.StringValue(c.stepStatus))
		c.readiness.set(cb, L)
		current = cb.
			Set("target", t.ptr("checkpoint.current.target", c.target, clsPinned, false)).
			Set("action", t.ptr("checkpoint.current.action", c.action, clsPinned, false)).
			Set("validation", t.ptr("checkpoint.current.validation", c.validation, clsPinned, false)).Value()
	}
	checkpoint := ojson.NewObject(20).
		Set("exists", ojson.BoolValue(m.cpExists)).
		Set("fresh", ojson.BoolValue(m.cpFresh)).
		Set("freshness", ojson.StringValue(m.freshness)).
		Set("sourceUpdatedAt", ojson.NullableString(m.sourceUpdatedAt)).
		Set("summary", summaryV).
		Set("current", current).
		Set("nextAction", t.ptr("checkpoint.nextAction", m.nextAction, clsPinned, false)).
		Set("blockers", t.list("checkpoint.blockers", m.blockers, L, clsLong, true)).
		Set("blockersTotal", ojson.IntValue(int64(m.blockersTotal))).
		Set("guardrails", t.list("checkpoint.guardrails", m.guardrails, L, clsLong, true)).
		Set("guardrailsTotal", ojson.IntValue(int64(m.guardrailsTotal))).
		Set("references", t.list("checkpoint.references", m.references, L, clsProtected, true)).
		Set("referencesTotal", ojson.IntValue(int64(m.referencesTotal))).
		Set("recentValidation", t.list("checkpoint.recentValidation", m.recentValidation, L, clsLong, true)).
		Set("evidenceStatus", ojson.StringValue("unverified")).
		Set("diagnostic", t.ptr("checkpoint.diagnostic", m.diagnostic, clsLong, false)).Value()

	var titleV ojson.Value
	if m.title == nil {
		titleV = ojson.NullValue()
	} else {
		titleV = t.str("workplan.title", *m.title, clsTitle, false)
	}
	workplan := ojson.NewObject(12).
		Set("id", ojson.StringValue(m.id)).
		Set("title", titleV).
		Set("goal", t.str("workplan.goal", m.goal, clsLong, false)).
		Set("status", ojson.StringValue(m.status)).
		Set("scope", t.list("workplan.scope", m.scope, L, clsLong, true)).
		Set("scopeTotal", ojson.IntValue(int64(len(m.scope)))).
		Set("nonGoals", t.list("workplan.nonGoals", m.nonGoals, L, clsLong, true)).
		Set("nonGoalsTotal", ojson.IntValue(int64(len(m.nonGoals)))).
		Set("constraints", t.list("workplan.constraints", m.constraints, L, clsLong, true)).
		Set("constraintsTotal", ojson.IntValue(int64(len(m.constraints)))).
		Set("relevantFiles", t.list("workplan.relevantFiles", m.relevantFiles, L, clsProtected, false)).
		Set("updatedAt", ojson.StringValue(m.updatedAt)).Value()

	depRefs := make([]ojson.Value, 0, shown(len(m.currentDeps), L))
	for i := 0; i < shown(len(m.currentDeps), L); i++ {
		depRefs = append(depRefs, refValue(m.currentDeps[i]))
	}
	currentDependencies := ojson.NewObject(4).
		Set("recorded", ojson.BoolValue(m.depsRecorded)).
		Set("valid", ojson.BoolValue(m.depsValid)).
		Set("total", ojson.IntValue(int64(len(m.currentDeps)))).
		Set("references", ojson.ArrayValue(depRefs)).Value()
	var criticalPath ojson.Value
	if m.critical != nil {
		criticalPath = ojson.NewObject(2).
			Set("length", ojson.IntValue(int64(m.critical.length))).
			Set("nextStep", ojson.NewObject(2).
				Set("phaseId", ojson.StringValue(m.critical.next.PhaseID)).
				Set("stepId", ojson.StringValue(m.critical.next.StepID)).Value()).Value()
	}

	high := make([]ojson.Value, 0, shown(len(m.high), L))
	for i := 0; i < shown(len(m.high), L); i++ {
		high = append(high, t.finding("safety.highFindings["+strconv.Itoa(i)+"]", m.high[i], false, true))
	}
	warnings := t.list("safety.unverifiedWarnings", m.warnings, L, clsLong, true)

	omitted := [8]int{
		len(m.constraints) - shown(len(m.constraints), L),
		len(m.blockers) - shown(len(m.blockers), L),
		len(m.high) - shown(len(m.high), L),
		len(m.currentDeps) - shown(len(m.currentDeps), L),
		len(m.guardrails) - shown(len(m.guardrails), L),
		len(m.references) - shown(len(m.references), L),
		len(m.scope) - shown(len(m.scope), L),
		len(m.nonGoals) - shown(len(m.nonGoals), L),
	}

	// Page items.
	total := len(m.items)
	n := pr.items
	if rem := total - m.offset; n > rem {
		n = rem
	}
	if n < 0 {
		n = 0
	}
	items := make([]ojson.Value, 0, n)
	for i := 0; i < n; i++ {
		it := &m.items[m.offset+i]
		base := "page.items[" + strconv.Itoa(i) + "]"
		switch it.kind {
		case "active-work":
			ib := ojson.NewObject(13).
				Set("kind", ojson.StringValue(it.kind)).
				Set("phaseId", ojson.StringValue(it.phaseID)).
				Set("phaseTitle", t.str(base+".phaseTitle", it.phaseTitle, clsTitle, false)).
				Set("phaseStatus", ojson.StringValue(it.phaseStatus)).
				Set("stepId", ojson.StringValue(it.stepID)).
				Set("title", t.str(base+".title", it.title, clsTitle, false)).
				Set("status", ojson.StringValue(it.status))
			it.readiness.set(ib, L)
			items = append(items, ib.
				Set("target", t.ptr(base+".target", it.target, clsLong, false)).
				Set("action", t.ptr(base+".action", it.action, clsLong, false)).
				Set("validation", t.ptr(base+".validation", it.validation, clsLong, false)).Value())
		case "finding":
			items = append(items, t.finding(base, it.finding, true, false))
		default:
			items = append(items, ojson.NewObject(3).
				Set("kind", ojson.StringValue(it.kind)).
				Set("reference", t.str(base+".reference", it.reference, clsProtected, false)).
				Set("source", ojson.StringValue(it.source)).Value())
		}
	}
	next := ojson.NullValue()
	if m.offset+n < total {
		next = ojson.StringValue(encodeCursor(resumeDomain, resumeCursor{
			stateHash: m.stateHash, maxChars: m.maxChars, limit: m.limit,
			phaseID: m.phaseFilter, stepID: m.stepFilter, offset: m.offset + n,
		}.fields()))
	}
	page := ojson.NewObject(7).
		Set("total", ojson.IntValue(int64(total))).
		Set("offset", ojson.IntValue(int64(m.offset))).
		Set("limit", ojson.IntValue(int64(m.limit))).
		Set("returned", ojson.IntValue(int64(n))).
		Set("omitted", ojson.IntValue(int64(total-m.offset-n))).
		Set("items", ojson.ArrayValue(items)).
		Set("nextCursor", next).Value()

	instruction := t.str("instruction", m.instruction, clsProtected, false)

	omittedSum := 0
	for _, o := range omitted {
		omittedSum += o
	}
	// Overflow is any omitted danger item or any truncated display field
	// (reference behaviour; a page that merely continues is not overflow).
	overflow := omittedSum > 0 || t.dangerCount > 0 || len(t.danger)+len(t.other) > 0
	pointers := []string{}
	if overflow {
		pointers = []string{"workplan_read:" + m.id, "workplan_inspect:" + m.id}
	}
	safety := ojson.NewObject(12).
		Set("highFindings", ojson.ArrayValue(high)).
		Set("highFindingsTotal", ojson.IntValue(int64(len(m.high)))).
		Set("highFindingCounts", ojson.NewObject(3).
			Set("blocker", ojson.IntValue(int64(m.highCounts[0]))).
			Set("critical", ojson.IntValue(int64(m.highCounts[1]))).
			Set("major", ojson.IntValue(int64(m.highCounts[2]))).Value()).
		Set("unverifiedWarnings", warnings).
		Set("unverifiedWarningCount", ojson.IntValue(int64(len(m.warnings)))).
		Set("unverifiedWarningsOmitted", ojson.IntValue(int64(len(m.warnings)-shown(len(m.warnings), L)))).
		Set("overflow", ojson.BoolValue(overflow)).
		Set("truncatedDangerFieldCount", ojson.IntValue(int64(t.dangerCount))).
		Set("omittedDangerCounts", ojson.NewObject(8).
			Set("constraints", ojson.IntValue(int64(omitted[0]))).
			Set("blockers", ojson.IntValue(int64(omitted[1]))).
			Set("highFindings", ojson.IntValue(int64(omitted[2]))).
			Set("dependencies", ojson.IntValue(int64(omitted[3]))).
			Set("guardrails", ojson.IntValue(int64(omitted[4]))).
			Set("references", ojson.IntValue(int64(omitted[5]))).
			Set("scope", ojson.IntValue(int64(omitted[6]))).
			Set("nonGoals", ojson.IntValue(int64(omitted[7]))).Value()).
		Set("overflowPointers", ojson.StringsValue(pointers)).Value()

	all := append(append([]string{}, t.danger...), t.other...)
	pathCap := resumePathCap(m.maxChars)
	listed := all
	if len(listed) > pathCap {
		listed = listed[:pathCap]
	}

	out := ojson.NewObject(16).
		Set("path", pathV).
		Set("planFile", planFileV).
		Set("hashes", ojson.NewObject(2).
			Set("planHash", ojson.StringValue(m.planHash)).
			Set("stateHash", ojson.StringValue(m.stateHash)).Value()).
		Set("planFresh", ojson.BoolValue(m.planFresh)).
		Set("checkpoint", checkpoint).
		Set("workplan", workplan).
		Set("currentDependencies", currentDependencies)
	if m.critical != nil {
		out.Set("criticalPath", criticalPath)
	}
	return out.
		Set("safety", safety).
		Set("page", page).
		Set("counts", ojson.NewObject(6).
			Set("activeWorkTotal", ojson.IntValue(int64(m.activeWorkTotal))).
			Set("findingsTotal", ojson.IntValue(int64(m.findingsTotal))).
			Set("highFindingsTotal", ojson.IntValue(int64(len(m.high)))).
			Set("referencesTotal", ojson.IntValue(int64(m.referencesPageTotal))).
			Set("historicalNotesOmitted", ojson.IntValue(int64(m.historicalNotes))).
			Set("resolvedFindingsOmitted", ojson.IntValue(int64(m.resolvedFindings))).Value()).
		Set("retrieval", ojson.NewObject(3).
			Set("read", ojson.StringValue("workplan_read id="+m.id)).
			Set("inspect", ojson.StringValue("workplan_inspect id="+m.id)).
			Set("dependencies", ojson.StringValue(snapshot.SidecarRel(m.id, ".dependencies.json"))).Value()).
		Set("truncatedFields", ojson.StringsValue(listed)).
		Set("truncatedFieldCount", ojson.IntValue(int64(len(all)))).
		Set("truncatedFieldPathsOmitted", ojson.IntValue(int64(len(all)-len(listed)))).
		Set("instruction", instruction).Value()
}

func (m *resumeModel) render(pr resumeParams) (ojson.Value, []byte) {
	v := m.build(pr)
	if pr.compact {
		return v, ojson.Compact(v)
	}
	return v, ojson.Pretty(v)
}

// Resume budget policy (D.1, approved 2026-09-30; contracts §11 item A).
//
// The reference shortened every display string (down to 1 code unit)
// before it returned fewer page items, so a default-budget packet for a
// long roadmap carried ~30-character fragments. D.1 balances readable
// text against a useful page: a target page of min(limit, T) items, T = 8
// at maxChars >= 12000, 4 at >= 6000 and 2 below.
//
//  1. While the page can still hold the target, text shrinks first:
//     readability tiers from uncapped down to prose 240 / titles 80, then
//     the floor prose 120 / titles 80; each tier returns the largest page
//     (>= target) that fits. Current work (checkpoint summary, next
//     action, current step target/action/validation) keeps at least 512.
//  2. Below the target, the existing order continues: at the floor the
//     page shrinks to one item (the cursor carries the rest), then current
//     work drops to the floor, then the pinned lists (scope, constraints,
//     guardrails, ...) show fewer entries, down to one each; the totals
//     and omittedDangerCounts that exist stay exact and overflow is set.
//  3. Only when not even one page item (or, with nothing left to page,
//     the pinned packet alone) fits that way, text shortens below the
//     minimums (emergency caps), then paths/references too.
//     Every shortened string ends in "…" and is listed in
//     truncatedFields, and safety.overflow is set.
//  4. As a last resort the page is dropped (zero items).
//
// File paths, references and the instruction are never shortened before
// step 3; ids, hashes, enums, counts and retrieval pointers never are.
// Within a level the pretty packet is preferred unless the compact one
// carries more page items. Every accepted budget still yields a packet
// within maxChars unless machine ids alone exceed it (contracts §10a
// item 2, unchanged).
const (
	resumeMinTitle   = 80
	resumeMinLong    = 120
	resumeTargetLong = 240 // prose floor while the page is above target
	resumePinnedMin  = 512 // current-work cap while the page is at target
)

// resumeTiers are the readability tiers {longCap, titleCap} used while
// the target page still fits; 0 = uncapped.
var resumeTiers = [][2]int{{0, 0}, {2048, 512}, {1024, 256}, {512, 200}, {resumeTargetLong, resumeMinTitle}, {resumeMinLong, resumeMinTitle}}

// resumeEmergencyCaps apply below the readability minimums (step 3).
var resumeEmergencyCaps = []int{100, 80, 64, 48, 32, 24, 16, 10, 4, 2, 1}

// resumeTargetPage is the page size text shrinks to protect (step 1).
func resumeTargetPage(maxChars, limit int) int {
	t := 2
	switch {
	case maxChars >= 12000:
		t = 8
	case maxChars >= 6000:
		t = 4
	}
	if limit < t {
		return limit
	}
	return t
}

// chooseResume applies the budget policy: the first degradation level
// whose complete text fits maxChars (UTF-16 code units). The D.3.1
// critical path is advisory (the full path stays in inspect and doctor),
// so it is dropped before any text goes below the D.1 minimums.
func chooseResume(m *resumeModel) (ojson.Value, string, error) {
	if v, text, ok := chooseResumeLevels(m); ok {
		return v, text, nil
	}
	if m.critical != nil {
		m.critical = nil
		if v, text, ok := chooseResumeLevels(m); ok {
			return v, text, nil
		}
	}
	return chooseResumeEmergency(m)
}

// chooseResumeLevels tries the readable levels (steps 1 and 2).
func chooseResumeLevels(m *resumeModel) (ojson.Value, string, bool) {
	L := resumeListCap(m.maxChars)
	remaining := len(m.items) - m.offset
	if remaining < 0 {
		remaining = 0
	}
	maxN := m.limit
	if maxN > remaining {
		maxN = remaining
	}
	target := resumeTargetPage(m.maxChars, m.limit)
	if target > maxN {
		target = maxN
	}
	try := func(pr resumeParams) (ojson.Value, []byte, bool) {
		v, b := m.render(pr)
		return v, b, ojson.UTF16LenBytes(b) <= m.maxChars
	}
	// fitN is the largest page size in 1..maxN that fits (0 if none).
	// Packet length grows with the page except that the last page drops
	// its cursor, so maxN is tried first and the rest is bisected.
	fitN := func(base resumeParams) int {
		base.items = maxN
		if _, _, ok := try(base); ok {
			return maxN
		}
		lo, hi := 0, maxN-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			base.items = mid
			if _, _, ok := try(base); ok {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return lo
	}
	pinned := func(long int) int {
		if long == 0 || long >= resumePinnedMin {
			return long
		}
		return resumePinnedMin
	}
	type level struct{ long, title, pinned, list, min int }
	var levels []level
	for _, tier := range resumeTiers {
		levels = append(levels, level{tier[0], tier[1], pinned(tier[0]), L, target})
	}
	levels = append(levels,
		level{resumeMinLong, resumeMinTitle, resumePinnedMin, L, 1},
		level{resumeMinLong, resumeMinTitle, resumeMinLong, L, 1})
	for l := L - 1; l >= 1; l-- {
		levels = append(levels, level{resumeMinLong, resumeMinTitle, resumeMinLong, l, 1})
	}
	for _, lv := range levels {
		base := resumeParams{listCap: lv.list, longCap: lv.long, titleCap: lv.title, pinnedCap: lv.pinned}
		if maxN == 0 {
			for _, compact := range []bool{false, true} {
				base.compact = compact
				if v, b, ok := try(base); ok {
					return v, string(b), true
				}
			}
			continue
		}
		nP := fitN(base)
		nC := 0
		if nP < maxN {
			cb := base
			cb.compact = true
			nC = fitN(cb)
		}
		min := lv.min
		if min < 1 {
			min = 1
		}
		if nP < min && nC < min {
			continue
		}
		if nP >= nC {
			base.items = nP
		} else {
			base.items, base.compact = nC, true
		}
		v, b := m.render(base)
		return v, string(b), true
	}
	return ojson.Value{}, "", false
}

// chooseResumeEmergency applies the emergency caps (steps 3 and 4).
func chooseResumeEmergency(m *resumeModel) (ojson.Value, string, error) {
	remaining := len(m.items) - m.offset
	if remaining < 0 {
		remaining = 0
	}
	maxN := m.limit
	if maxN > remaining {
		maxN = remaining
	}
	try := func(pr resumeParams) (ojson.Value, []byte, bool) {
		v, b := m.render(pr)
		return v, b, ojson.UTF16LenBytes(b) <= m.maxChars
	}
	one := maxN
	if one > 1 {
		one = 1
	}
	for _, protect := range []bool{true, false} {
		for _, c := range resumeEmergencyCaps {
			pr := resumeParams{listCap: 1, longCap: c, titleCap: c, pinnedCap: c, items: one}
			if !protect {
				pr.protCap = c
			}
			for _, compact := range []bool{false, true} {
				pr.compact = compact
				if v, b, ok := try(pr); ok {
					return v, string(b), nil
				}
			}
		}
	}
	for _, compact := range []bool{false, true} {
		if v, b, ok := try(resumeParams{listCap: 1, longCap: 1, titleCap: 1, pinnedCap: 1, protCap: 1, compact: compact}); ok {
			return v, string(b), nil
		}
	}
	return ojson.Value{}, "", fmt.Errorf("resume packet cannot fit maxChars=%d", m.maxChars)
}

// resumeListCap is the pinned-list cap for a budget: floor(maxChars/900)
// clamped to 4..8. The corpus pins 4 at 4096 and 8 at 12000 and 64000;
// the values in between were measured on the reference (stage C,
// contracts §10) by sweeping maxChars across 4096..12000.
func resumeListCap(maxChars int) int {
	l := maxChars / 900
	if l < 4 {
		return 4
	}
	if l > 8 {
		return 8
	}
	return l
}

// resumePathCap bounds the listed truncatedFields paths: floor(maxChars/
// 256), at most 32 (measured on the reference like resumeListCap).
func resumePathCap(maxChars int) int {
	c := maxChars / 256
	if c > 32 {
		return 32
	}
	return c
}
