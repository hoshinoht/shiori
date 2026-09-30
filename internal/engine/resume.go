package engine

import (
	"fmt"
	"strconv"

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

// resumeParams is one degradation level.
type resumeParams struct {
	listCap int // pinned list length cap
	strCap  int // display string cap (UTF-16 code units incl. ellipsis)
	items   int // page items to include
	compact bool
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
				target: st.Target, action: st.Action, validation: st.Validation})
		}
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

// truncState records truncated display fields in traversal order.
type truncState struct {
	cap         int
	danger      []string
	other       []string
	dangerCount int
}

func (t *truncState) str(path, s string, danger bool) ojson.Value {
	out, cut := ojson.TruncateUTF16(s, t.cap)
	if cut {
		t.record(path, danger)
	}
	return ojson.StringValue(out)
}

func (t *truncState) ptr(path string, s *string, danger bool) ojson.Value {
	if s == nil {
		return ojson.NullValue()
	}
	return t.str(path, *s, danger)
}

func (t *truncState) record(path string, danger bool) {
	if danger {
		t.danger = append(t.danger, path)
		t.dangerCount++
	} else {
		t.other = append(t.other, path)
	}
}

func (t *truncState) list(path string, items []string, cap int, danger bool) ojson.Value {
	n := len(items)
	if n > cap {
		n = cap
	}
	out := make([]ojson.Value, n)
	for i := 0; i < n; i++ {
		out[i] = t.str(path+"["+strconv.Itoa(i)+"]", items[i], danger)
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
		b.Set("kind", t.str(path+".kind", "finding", false))
	}
	b.Set("index", ojson.IntValue(int64(f.index))).
		Set("severity", t.str(path+".severity", f.severity, danger)).
		Set("title", t.str(path+".title", f.title, danger)).
		Set("detail", t.ptr(path+".detail", f.detail, danger)).
		Set("source", t.ptr(path+".source", f.source, danger)).
		Set("status", ojson.StringValue(f.status)).
		Set("metadataOmitted", ojson.BoolValue(f.metadataOmitted))
	if f.metadataOmitted {
		t.record(path+".customMetadata", danger)
	}
	return b.Value()
}

// build renders the packet for one degradation level.
func (m *resumeModel) build(pr resumeParams) ojson.Value {
	t := &truncState{cap: pr.strCap}
	L := pr.listCap

	pathV := t.str("path", m.path, false)
	planFileV := t.str("planFile", m.planFile, false)
	// The summary is truncated (and listed) before the current position.
	summaryV := t.ptr("checkpoint.summary", m.summary, false)
	var current ojson.Value
	if m.current == nil {
		current = ojson.NullValue()
	} else {
		c := m.current
		current = ojson.NewObject(9).
			Set("phaseId", ojson.StringValue(c.phaseID)).
			Set("phaseTitle", t.str("checkpoint.current.phaseTitle", c.phaseTitle, false)).
			Set("phaseStatus", ojson.StringValue(c.phaseStatus)).
			Set("stepId", ojson.StringValue(c.stepID)).
			Set("stepTitle", t.str("checkpoint.current.stepTitle", c.stepTitle, false)).
			Set("stepStatus", ojson.StringValue(c.stepStatus)).
			Set("target", t.ptr("checkpoint.current.target", c.target, false)).
			Set("action", t.ptr("checkpoint.current.action", c.action, false)).
			Set("validation", t.ptr("checkpoint.current.validation", c.validation, false)).Value()
	}
	checkpoint := ojson.NewObject(20).
		Set("exists", ojson.BoolValue(m.cpExists)).
		Set("fresh", ojson.BoolValue(m.cpFresh)).
		Set("freshness", ojson.StringValue(m.freshness)).
		Set("sourceUpdatedAt", ojson.NullableString(m.sourceUpdatedAt)).
		Set("summary", summaryV).
		Set("current", current).
		Set("nextAction", t.ptr("checkpoint.nextAction", m.nextAction, false)).
		Set("blockers", t.list("checkpoint.blockers", m.blockers, L, true)).
		Set("blockersTotal", ojson.IntValue(int64(m.blockersTotal))).
		Set("guardrails", t.list("checkpoint.guardrails", m.guardrails, L, true)).
		Set("guardrailsTotal", ojson.IntValue(int64(m.guardrailsTotal))).
		Set("references", t.list("checkpoint.references", m.references, L, true)).
		Set("referencesTotal", ojson.IntValue(int64(m.referencesTotal))).
		Set("recentValidation", t.list("checkpoint.recentValidation", m.recentValidation, L, true)).
		Set("evidenceStatus", ojson.StringValue("unverified")).
		Set("diagnostic", t.ptr("checkpoint.diagnostic", m.diagnostic, false)).Value()

	var titleV ojson.Value
	if m.title == nil {
		titleV = ojson.NullValue()
	} else {
		titleV = t.str("workplan.title", *m.title, false)
	}
	workplan := ojson.NewObject(12).
		Set("id", ojson.StringValue(m.id)).
		Set("title", titleV).
		Set("goal", t.str("workplan.goal", m.goal, false)).
		Set("status", ojson.StringValue(m.status)).
		Set("scope", t.list("workplan.scope", m.scope, L, true)).
		Set("scopeTotal", ojson.IntValue(int64(len(m.scope)))).
		Set("nonGoals", t.list("workplan.nonGoals", m.nonGoals, L, true)).
		Set("nonGoalsTotal", ojson.IntValue(int64(len(m.nonGoals)))).
		Set("constraints", t.list("workplan.constraints", m.constraints, L, true)).
		Set("constraintsTotal", ojson.IntValue(int64(len(m.constraints)))).
		Set("relevantFiles", t.list("workplan.relevantFiles", m.relevantFiles, L, false)).
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

	high := make([]ojson.Value, 0, shown(len(m.high), L))
	for i := 0; i < shown(len(m.high), L); i++ {
		high = append(high, t.finding("safety.highFindings["+strconv.Itoa(i)+"]", m.high[i], false, true))
	}
	warnings := t.list("safety.unverifiedWarnings", m.warnings, L, true)

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
			items = append(items, ojson.NewObject(10).
				Set("kind", t.str(base+".kind", it.kind, false)).
				Set("phaseId", ojson.StringValue(it.phaseID)).
				Set("phaseTitle", t.str(base+".phaseTitle", it.phaseTitle, false)).
				Set("phaseStatus", ojson.StringValue(it.phaseStatus)).
				Set("stepId", ojson.StringValue(it.stepID)).
				Set("title", t.str(base+".title", it.title, false)).
				Set("status", ojson.StringValue(it.status)).
				Set("target", t.ptr(base+".target", it.target, false)).
				Set("action", t.ptr(base+".action", it.action, false)).
				Set("validation", t.ptr(base+".validation", it.validation, false)).Value())
		case "finding":
			items = append(items, t.finding(base, it.finding, true, false))
		default:
			items = append(items, ojson.NewObject(3).
				Set("kind", t.str(base+".kind", it.kind, false)).
				Set("reference", t.str(base+".reference", it.reference, false)).
				Set("source", t.str(base+".source", it.source, false)).Value())
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

	instruction := t.str("instruction", m.instruction, false)

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

	return ojson.NewObject(15).
		Set("path", pathV).
		Set("planFile", planFileV).
		Set("hashes", ojson.NewObject(2).
			Set("planHash", ojson.StringValue(m.planHash)).
			Set("stateHash", ojson.StringValue(m.stateHash)).Value()).
		Set("planFresh", ojson.BoolValue(m.planFresh)).
		Set("checkpoint", checkpoint).
		Set("workplan", workplan).
		Set("currentDependencies", currentDependencies).
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

// chooseResume applies the budget policy: the first degradation level
// whose complete text fits maxChars (UTF-16 code units).
func chooseResume(m *resumeModel) (ojson.Value, string, error) {
	fits := func(b []byte) bool { return ojson.UTF16LenBytes(b) <= m.maxChars }
	for _, pr := range resumeLadder(m) {
		v, b := m.render(pr)
		if fits(b) {
			return v, string(b), nil
		}
	}
	return ojson.Value{}, "", fmt.Errorf("resume packet cannot fit maxChars=%d", m.maxChars)
}

// resumeStringCaps is the display-string cap ladder. The golden corpus
// requires 32, 21, 10, 2 and 1 and forbids 11-20, 22-31 and 33-58; a
// stage C sweep of the reference over maxChars 4096..12000 (contracts
// §10) additionally requires 4 and excludes 5. With this ladder every
// sampled budget of the large-paging and full-valid fixtures is
// byte-identical to the reference; plans with very many truncated strings
// (resume-stress) can still select a different display cap (open issue).
var resumeStringCaps = []int{512, 256, 128, 64, 32, 21, 10, 4, 2, 1}

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

// resumeLadder lists degradation levels from richest to smallest: each
// string cap first pretty then compact with the full page; then, at the
// smallest cap, progressively fewer page items.
func resumeLadder(m *resumeModel) []resumeParams {
	L := resumeListCap(m.maxChars)
	var out []resumeParams
	for _, c := range resumeStringCaps {
		out = append(out, resumeParams{L, c, m.limit, false}, resumeParams{L, c, m.limit, true})
	}
	for items := m.limit - 1; items >= 0; items-- {
		out = append(out, resumeParams{L, 1, items, false}, resumeParams{L, 1, items, true})
	}
	return out
}
