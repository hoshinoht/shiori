package engine

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Tests for the D.3.1 approved design changes (contracts §14). Oracle
// vector changes are covered by d3_1_vectors_test.go.

func d31Engine(t *testing.T, root string) *Engine {
	t.Helper()
	e, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mustHash(t *testing.T, e *Engine, id string) string {
	t.Helper()
	return stateHashFor(t, e, id)
}

// TestD31StepStatusGate is item 1: a step cannot be set to in_progress,
// review or completed while it lacks its own required structure; the
// refusal comes before authorization with field paths and class
// invalid_structure. draft/blocked/cancelled stay allowed, completing the
// fields in the same call is accepted, and unchanged gated steps are not
// re-checked.
func TestD31StepStatusGate(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := d31Engine(t, root.Path)
	// A step without action/validation (allowed while draft).
	mustMutate(t, e, "workplan_update", `{"id":"minimal","addSteps":[{"phaseId":"phase-one","step":{"id":"bare","title":"Bare step"}}]}`)
	for _, st := range []string{"in_progress", "review", "completed"} {
		before := testutil.Fingerprint(t, root.Path)
		_, err, n := mutateErr(t, e, "workplan_update", `{"id":"minimal","updateSteps":[{"phaseId":"phase-one","stepId":"bare","status":"`+st+`"}]}`)
		var gate *StepStatusGateError
		if !errors.As(err, &gate) || n != 0 {
			t.Fatalf("%s: err %v (authorizations %d), want a step gate refusal before authorization", st, err, n)
		}
		if ErrorClass(err) != "invalid_structure" {
			t.Fatalf("class %s", ErrorClass(err))
		}
		var paths []string
		for _, is := range gate.StructuredIssues() {
			paths = append(paths, is.PathString())
		}
		if strings.Join(paths, ",") != "phases.0.steps.1.action,phases.0.steps.1.validation" {
			t.Fatalf("issue paths %v", paths)
		}
		if !strings.Contains(err.Error(), "phase-one/bare -> "+st) {
			t.Fatalf("message %q", err.Error())
		}
		if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
			t.Fatalf("refusal wrote: %v", d)
		}
	}
	for _, st := range []string{"blocked", "cancelled", "draft"} {
		mustMutate(t, e, "workplan_update", `{"id":"minimal","updateSteps":[{"phaseId":"phase-one","stepId":"bare","status":"`+st+`"}]}`)
	}
	// Completing the fields in the same call is accepted.
	mustMutate(t, e, "workplan_update", `{"id":"minimal","updateSteps":[{"phaseId":"phase-one","stepId":"bare","status":"in_progress","action":"Do it","validation":"Check it"}]}`)
	// A new step or phase with a gated status is checked too.
	for _, in := range []string{
		`{"id":"minimal","addSteps":[{"phaseId":"phase-one","step":{"id":"new","title":"New","status":"in_progress","action":"a"}}]}`,
		`{"id":"minimal","addPhases":[{"phase":{"id":"p2","title":"P2","steps":[{"id":"s","title":"S","status":"completed","validation":"v"}]}}]}`,
		`{"id":"minimal","phases":[{"id":"phase-one","title":"Phase one","steps":[{"id":"step-one","title":"Step one","status":"review"}]}]}`,
	} {
		_, err, n := mutateErr(t, e, "workplan_update", in)
		var gate *StepStatusGateError
		if !errors.As(err, &gate) || n != 0 {
			t.Fatalf("%s: err %v (authorizations %d)", in, err, n)
		}
	}
	// A gated step whose status does not change is not re-checked: an
	// existing in-progress step without structure stays editable.
	p := planFile(t, root.Path, "minimal")
	p.Phases[0].Steps[0].Action = nil
	p.Phases[0].Steps[0].Status = "in_progress"
	os.WriteFile(filepath.Join(root.Path, ".opencode/workplan/minimal.json"), p.EncodeStored(), 0o644)
	mustMutate(t, e, "workplan_update", `{"id":"minimal","appendNotes":["still editable"],"updateSteps":[{"phaseId":"phase-one","stepId":"step-one","title":"Renamed"}]}`)
	// Create applies the same gate to the steps it creates.
	before := testutil.Fingerprint(t, root.Path)
	_, err, n := mutateErr(t, e, "workplan_create", `{"id":"made","goal":"g","phases":[{"id":"p","title":"P","steps":[{"id":"ok","title":"OK","status":"completed","action":"a","validation":"v"},{"id":"s","title":"S","status":"review","action":"a"}]}]}`)
	var cgate *StepStatusGateError
	if !errors.As(err, &cgate) || n != 0 || ErrorClass(err) != "invalid_structure" || len(cgate.Issues) != 1 || cgate.Issues[0].PathString() != "phases.0.steps.1.validation" || !strings.Contains(err.Error(), "p/s -> review") {
		t.Fatalf("create step gate: %v (authorizations %d)", err, n)
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("create refusal wrote: %v", d)
	}
	mustMutate(t, e, "workplan_create", `{"id":"made","goal":"g","phases":[{"id":"p","title":"P","steps":[{"id":"s","title":"S","status":"blocked"},{"id":"t","title":"T","status":"in_progress","action":"a","validation":"v"}]}]}`)
	// With the D.3.1 changes off the gate does not apply (earlier stages).
	off := *e
	off.noD31 = true
	mustMutate(t, &off, "workplan_update", `{"id":"minimal","addSteps":[{"phaseId":"phase-one","step":{"id":"legacy","title":"Legacy","status":"in_progress"}}]}`)
}

// planWithLink writes an unreadable plan whose raw bytes still carry a
// planFile member, linked Markdown at that path, and returns the paths.
func planWithLink(t *testing.T, root string, raw string) (string, string) {
	t.Helper()
	jp := filepath.Join(root, ".opencode/workplan/minimal.json")
	md := filepath.Join(root, ".opencode/workplan/docs/minimal-notes.md")
	os.MkdirAll(filepath.Dir(md), 0o755)
	os.WriteFile(md, []byte("# Handwritten notes\n"), 0o644)
	os.WriteFile(jp, []byte(raw), 0o644)
	return jp, md
}

// TestD31UnreadableRepair is item 2: doctor recovers the planFile from
// the damaged bytes, create overwrite keeps using it and archives the
// exact damaged bytes first in the same transaction, an explicit planFile
// wins, and writers name the repair.
func TestD31UnreadableRepair(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := d31Engine(t, root.Path)
	raw := `{"schemaVersion":2,"id":"minimal","kind":"general","goal":"g","planFile":".opencode/workplan/docs/minimal-notes.md","phases":[{"id":"p","title":"P","ste`
	jp, md := planWithLink(t, root.Path, raw)
	doc, _ := e.Doctor(DoctorInput{})
	plans, _ := doc.Get("plans")
	entry := plans.Elems()[0]
	if got := strMember(entry, "recoveredPlanFile"); got != ".opencode/workplan/docs/minimal-notes.md" {
		t.Fatalf("recoveredPlanFile %q in %s", got, ojson.Compact(entry))
	}
	sh := strMember(entry, "stateHash")

	// (c) writers name the repair and the doctor hash.
	for _, c := range []struct{ tool, in string }{
		{"workplan_update", `{"id":"minimal","appendNotes":["x"]}`},
		{"workplan_reset", `{"id":"minimal"}`},
		{"workplan_checkpoint", `{"id":"minimal","summary":"s","nextAction":"n"}`},
	} {
		_, err, n := mutateErr(t, e, c.tool, c.in)
		var ue *UnreadablePlanError
		if !errors.As(err, &ue) || n != 0 || !strings.Contains(err.Error(), "workplan_create overwrite=true and expectedHash="+sh) {
			t.Fatalf("%s: %v", c.tool, err)
		}
		if ErrorClass(err) != "invalid_structure" {
			t.Fatalf("%s class %s", c.tool, ErrorClass(err))
		}
	}

	// (a)+(b) the repair keeps the recovered link and archives the bytes.
	out := mustMutate(t, e, "workplan_create", `{"id":"minimal","goal":"Repaired","overwrite":true,"expectedHash":"`+sh+`"}`)
	if pp := strMember(out.Value, "planPath"); pp != e.absRel(".opencode/workplan/docs/minimal-notes.md") {
		t.Fatalf("planPath %s", pp)
	}
	if got, _ := os.ReadFile(md); string(got) != "# Handwritten notes\n" {
		t.Fatal("handwritten Markdown at the recovered link was replaced")
	}
	if _, err := os.Stat(filepath.Join(root.Path, ".opencode/workplan/minimal.md")); err != nil {
		t.Fatal("the old default Markdown must stay untouched")
	}
	ap := strMember(out.Value, "archivePath")
	if !regexp.MustCompile(`/\.opencode/workplan/archive/minimal/state-` + sh[:12] + `-[0-9a-f]{12}\.json$`).MatchString(ap) {
		t.Fatalf("archivePath %q", ap)
	}
	st, err := os.Stat(ap)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("archive %v mode %v", err, st)
	}
	var arc struct {
		ArchiveVersion int    `json:"archiveVersion"`
		WorkplanID     string `json:"workplanId"`
		Operation      string `json:"operation"`
		StateHash      string `json:"stateHash"`
		Source         struct {
			WorkplanJSON       *string `json:"workplanJson"`
			WorkplanJSONSha256 string  `json:"workplanJsonSha256"`
			LinkedMarkdownPath string  `json:"linkedMarkdownPath"`
			LinkedMarkdown     *string `json:"linkedMarkdown"`
		} `json:"source"`
	}
	data, _ := os.ReadFile(ap)
	if err := json.Unmarshal(data, &arc); err != nil {
		t.Fatal(err)
	}
	if arc.ArchiveVersion != 1 || arc.WorkplanID != "minimal" || arc.Operation != "create:overwrite" || arc.StateHash != sh ||
		arc.Source.WorkplanJSON == nil || *arc.Source.WorkplanJSON != raw || arc.Source.WorkplanJSONSha256 != sha(raw) ||
		arc.Source.LinkedMarkdownPath != ".opencode/workplan/docs/minimal-notes.md" || arc.Source.LinkedMarkdown == nil {
		t.Fatalf("archive %s", data)
	}
	if p := planFile(t, root.Path, "minimal"); p.PlanFile != ".opencode/workplan/docs/minimal-notes.md" || p.Goal != "Repaired" {
		t.Fatalf("repaired plan %+v", p)
	}

	// Invalid UTF-8 bytes are archived exactly as base64; an explicit
	// planFile wins over the recovered one.
	bad := append([]byte(`{"planFile":".opencode/workplan/docs/minimal-notes.md",`), 0xff, 0xfe)
	os.WriteFile(jp, bad, 0o644)
	sh = mustHash(t, e, "minimal")
	out = mustMutate(t, e, "workplan_create", `{"id":"minimal","goal":"Again","overwrite":true,"planFile":".opencode/workplan/minimal.md","replaceMarkdown":true,"expectedHash":"`+sh+`"}`)
	if pp := strMember(out.Value, "planPath"); pp != e.absRel(".opencode/workplan/minimal.md") {
		t.Fatalf("explicit planFile lost: %s", pp)
	}
	var arc2 struct {
		Source struct {
			WorkplanJSON       *string `json:"workplanJson"`
			WorkplanJSONBase64 string  `json:"workplanJsonBase64"`
		} `json:"source"`
	}
	data, _ = os.ReadFile(strMember(out.Value, "archivePath"))
	json.Unmarshal(data, &arc2)
	dec, _ := base64.StdEncoding.DecodeString(arc2.Source.WorkplanJSONBase64)
	if arc2.Source.WorkplanJSON != nil || string(dec) != string(bad) {
		t.Fatalf("non-UTF-8 archive %s", data)
	}

	// A link another readable plan owns is not recovered.
	mustMutate(t, e, "workplan_create", `{"id":"other","goal":"g","planFile":".opencode/workplan/docs/other.md"}`)
	if pf := e.recoverPlanFile("minimal", []byte(`{"planFile":".opencode/workplan/docs/other.md",`)); pf != "" {
		t.Fatalf("recovered another plan's link %q", pf)
	}
	// No or unsafe planFile members: nothing is recovered.
	for _, raw := range []string{`{"id":"minimal"`, `{"planFile":"../outside.md",`, `{"planFile":"a.md","x":{"planFile":"b.md"}`, `{"planFile":"notes.txt",`} {
		os.WriteFile(jp, []byte(raw), 0o644)
		if pf := e.recoverPlanFile("minimal", []byte(raw)); pf != "" {
			t.Fatalf("%s: recovered %q", raw, pf)
		}
		doc, _ := e.Doctor(DoctorInput{})
		plans, _ := doc.Get("plans")
		if has3(plans.Elems()[0], "recoveredPlanFile") {
			t.Fatalf("%s: doctor reports a recovered planFile", raw)
		}
	}
}

// TestD31ResumeStaleDiagnostic is item 3: a stale checkpoint's resume
// diagnostic names what changed (at most three paths, then "+N more").
func TestD31ResumeStaleDiagnostic(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	resume := func(max int) (string, ojson.Value) {
		v, text, err := e.Resume(ResumeInput{ID: "full-plan", MaxChars: &max})
		if err != nil {
			t.Fatal(err)
		}
		cp, _ := v.Get("checkpoint")
		d, _ := cp.Get("diagnostic")
		return text, d
	}
	if _, d := resume(12000); d.Kind() != ojson.Null {
		t.Fatalf("fresh checkpoint diagnostic %s", ojson.Compact(d))
	}
	os.WriteFile(filepath.Join(root.Path, ".opencode/workplan/full-plan.md"), []byte("# edited\n"), 0o644)
	if _, d := resume(12000); d.Str() != "changed: .opencode/workplan/full-plan.md" {
		t.Fatalf("diagnostic %q", d.Str())
	}
	// More than three changed links: three paths and "+N more".
	for _, f := range []string{"a", "b", "c"} {
		os.WriteFile(filepath.Join(root.Path, f+".md"), []byte(f), 0o644)
	}
	s, _ := snapshot.Load(root.Path, "full-plan", snapshot.DefaultLimits)
	p := s.Plan
	p.SpecFiles = []string{"a.md", "b.md", "c.md"}
	os.WriteFile(filepath.Join(root.Path, ".opencode/workplan/full-plan.json"), p.EncodeStored(), 0o644)
	for _, max := range []int{4096, 12000} {
		text, d := resume(max)
		if !regexp.MustCompile(`^changed: [^,]+, [^,]+, [^,]+ \+[1-9][0-9]* more$`).MatchString(d.Str()) {
			t.Fatalf("diagnostic %q", d.Str())
		}
		if ojson.UTF16Len(text) > max {
			t.Fatalf("packet %d over budget %d", ojson.UTF16Len(text), max)
		}
	}
	if got := staleDiagnostic([]string{strings.Repeat("x", 300)}); ojson.UTF16Len(got) != maxDiagnosticUnits || !strings.HasSuffix(got, "…") {
		t.Fatalf("uncapped diagnostic (%d units)", ojson.UTF16Len(got))
	}
	// Legacy v1 checkpoints keep a null diagnostic.
	root2 := testutil.NewRoot(t, "checkpoint-legacy-v1")
	e2 := d31Engine(t, root2.Path)
	ids, _ := filepath.Glob(filepath.Join(root2.Path, ".opencode/workplan/*.checkpoint.json"))
	id := strings.TrimSuffix(filepath.Base(ids[0]), ".checkpoint.json")
	v, _, err := e2.Resume(ResumeInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if cp, _ := v.Get("checkpoint"); strMember(cp, "freshness") != FreshnessLegacy || func() bool { d, _ := cp.Get("diagnostic"); return d.Kind() != ojson.Null }() {
		t.Fatalf("legacy checkpoint %s", ojson.Compact(cp))
	}
}

// TestD31ResumeCriticalPath is item 4: resume carries the compact
// critical path only with a valid sidecar and a path of two or more open
// steps; otherwise its bytes equal the D.3.1-off packet.
func TestD31ResumeCriticalPath(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	v, _, err := e.Resume(ResumeInput{ID: "full-plan"})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := v.Get("criticalPath")
	if !ok {
		t.Fatal("no criticalPath")
	}
	insp, _ := e.Inspect(InspectInput{ID: "full-plan"})
	full, _ := insp.Get("criticalPath")
	steps, _ := full.Get("steps")
	l, _ := c.Get("length")
	fl, _ := full.Get("length")
	if l.NumberLiteral() != fl.NumberLiteral() || strMember(c, "nextStep", "stepId") != strMember(steps.Elems()[0], "stepId") {
		t.Fatalf("resume %s, inspect %s", ojson.Compact(c), ojson.Compact(full))
	}
	if len(c.Members()) != 2 {
		t.Fatalf("resume critical path must stay compact: %s", ojson.Compact(c))
	}
	// Without a sidecar the packet is byte-identical to D.3.1-off.
	for _, fx := range []string{"minimal-valid", "large-paging", "resume-stress"} {
		r := testutil.NewRoot(t, fx)
		e := d31Engine(t, r.Path)
		off := *e
		off.noD31 = true
		ls, _ := e.List(ListInput{})
		ws, _ := ls.Get("workplans")
		for _, w := range ws.Elems() {
			id := strMember(w, "id")
			for _, max := range []int{4096, 12000} {
				_, a, err1 := e.Resume(ResumeInput{ID: id, MaxChars: &max})
				_, b, err2 := off.Resume(ResumeInput{ID: id, MaxChars: &max})
				if err1 != nil || err2 != nil || a != b {
					t.Fatalf("%s/%s at %d differs without a sidecar", fx, id, max)
				}
			}
		}
	}
}

// TestD31WipedNote is item 5a: doctor explains an empty draft plan left
// by a wipe; validate keeps its issue.
func TestD31WipedNote(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	sh := mustHash(t, e, "full-plan")
	tok := wipeToken(t, e, "full-plan", sh, "")
	out := mustMutate(t, e, "workplan_reset", `{"id":"full-plan","mode":"wipe","expectedHash":"`+sh+`","previewToken":"`+tok+`","confirmation":"WIPE_PLAN_CONTENT"}`)
	arch := e.relPath(strMember(out.Value, "archivePath"))
	id := "full-plan"
	doc, _ := e.Doctor(DoctorInput{ID: &id})
	plans, _ := doc.Get("plans")
	w, _ := plans.Elems()[0].Get("warnings")
	if !containsPrefix(strs(w), d31WipedNeedle+"; the removed content is archived at "+arch+". Add phases") {
		t.Fatalf("warnings %v (archive %s)", strs(w), arch)
	}
	v, _ := e.Validate(ValidateInput{ID: "full-plan"})
	is, _ := v.Get("issues")
	if !containsPrefix(strs(is), "phases: At least one phase is required") {
		t.Fatalf("validate issues %v", strs(is))
	}
	// Adding a phase removes the note.
	mustMutate(t, e, "workplan_update", `{"id":"full-plan","addPhases":[{"phase":{"id":"p","title":"P","steps":[{"id":"s","title":"S","action":"a","validation":"v"}]}}]}`)
	doc, _ = e.Doctor(DoctorInput{ID: &id})
	plans, _ = doc.Get("plans")
	w, _ = plans.Elems()[0].Get("warnings")
	if containsPrefix(strs(w), d31WipedNeedle) {
		t.Fatal("note stays after phases were added")
	}
	// An empty draft plan without a wipe archive gets no note.
	root2 := testutil.NewRoot(t, "draft-empty")
	e2 := d31Engine(t, root2.Path)
	doc, _ = e2.Doctor(DoctorInput{})
	if strings.Contains(string(ojson.Compact(doc)), d31WipedNeedle) {
		t.Fatal("note without a wipe archive")
	}
}

// TestD31LegacyGeneratedMarkdown is item 5b: Markdown generated with the
// old finding rendering ("title(status)", as the reference and earlier
// stages wrote it) is still generated: no drift warning, and writes
// refresh it to the new rendering instead of refusing it as handwritten.
func TestD31LegacyGeneratedMarkdown(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	mdPath := filepath.Join(root.Path, ".opencode/workplan/full-plan.md")
	old, _ := os.ReadFile(mdPath)
	if !strings.Contains(string(old), "] Blocker finding(open)") {
		t.Fatal("fixture Markdown is not in the old rendering")
	}
	s, _ := snapshot.Load(root.Path, "full-plan", snapshot.DefaultLimits)
	if gen, err := e.generatedMarkdown(s); err != nil || !gen {
		t.Fatalf("old rendering not generated: %v %v", gen, err)
	}
	if gen, present, err := e.MarkdownGenerated("full-plan"); err != nil || !gen || !present {
		t.Fatal("MarkdownGenerated misclassifies the old rendering")
	}
	v, _ := e.Validate(ValidateInput{ID: "full-plan"})
	if has3(v, "warnings") {
		t.Fatalf("drift warning on the old rendering: %s", ojson.Compact(v))
	}
	// An update refreshes it to the new rendering.
	mustMutate(t, e, "workplan_update", `{"id":"full-plan","appendNotes":["n"]}`)
	now, _ := os.ReadFile(mdPath)
	if !strings.Contains(string(now), "] Blocker finding (open)— why") || strings.Contains(string(now), "finding(open)") {
		t.Fatalf("Markdown not refreshed to the new rendering:\n%s", now)
	}
	p := planFile(t, root.Path, "full-plan")
	if cur, _ := model.RenderMarkdown(p); string(cur) != string(now) {
		t.Fatal("refreshed Markdown is not the current rendering")
	}
	// markdown-only reset of old-rendering Markdown refreshes it too.
	legacy, _ := model.RenderMarkdownLegacy(p)
	os.WriteFile(mdPath, legacy, 0o644)
	mustMutate(t, e, "workplan_reset", `{"id":"full-plan","mode":"markdown-only"}`)
	if now, _ := os.ReadFile(mdPath); !strings.Contains(string(now), "finding (open)") {
		t.Fatal("markdown-only did not refresh the old rendering")
	}
	// Real hand edits are still handwritten.
	os.WriteFile(mdPath, append(legacy, []byte("\nhand edit\n")...), 0o644)
	_, err, n := mutateErr(t, e, "workplan_reset", `{"id":"full-plan","mode":"markdown-only"}`)
	if err == nil || !strings.HasPrefix(err.Error(), "Refusing to replace handwritten") || n != 0 {
		t.Fatalf("hand-edited Markdown: %v", err)
	}
	v, _ = e.Validate(ValidateInput{ID: "full-plan"})
	if w, _ := v.Get("warnings"); !containsPrefix(strs(w), "planFile: Linked Markdown is not the generated rendering") {
		t.Fatal("no drift warning on hand-edited Markdown")
	}
}

// TestD31Timestamps is item 5b: new writes record whole-second UTC; stored
// millisecond (and other accepted) timestamps stay valid and untouched.
func TestD31Timestamps(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := d31Engine(t, root.Path)
	mustMutate(t, e, "workplan_create", `{"id":"fresh","goal":"g","phases":[{"id":"p","title":"P","steps":[{"id":"s","title":"S","action":"a","validation":"v"}]}]}`)
	whole := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	p := planFile(t, root.Path, "fresh")
	if !whole.MatchString(p.CreatedAt) || !whole.MatchString(p.UpdatedAt) {
		t.Fatalf("timestamps %s / %s", p.CreatedAt, p.UpdatedAt)
	}
	sh := mustHash(t, e, "fresh")
	mustMutate(t, e, "workplan_checkpoint", `{"id":"fresh","expectedHash":"`+sh+`","summary":"s","nextAction":"n"}`)
	var cp map[string]any
	data, _ := os.ReadFile(filepath.Join(root.Path, ".opencode/workplan/fresh.checkpoint.json"))
	json.Unmarshal(data, &cp)
	for k, x := range cp {
		if s, ok := x.(string); ok && strings.HasSuffix(k, "At") && !whole.MatchString(s) {
			t.Fatalf("checkpoint %s = %s", k, s)
		}
	}
	// The minimal plan keeps its stored millisecond createdAt; it stays
	// valid, and so do the other accepted forms.
	mustMutate(t, e, "workplan_update", `{"id":"minimal","appendNotes":["n"]}`)
	m := planFile(t, root.Path, "minimal")
	if m.CreatedAt != "2026-01-02T03:04:05.000Z" || !whole.MatchString(m.UpdatedAt) {
		t.Fatalf("minimal %s / %s", m.CreatedAt, m.UpdatedAt)
	}
	for _, ts := range []string{"2026-01-02T03:04:05.000Z", "2026-01-02T03:04:05Z", "2026-01-02T03:04Z", "2026-01-02T03:04:05.123456Z"} {
		if !model.ValidDatetime(ts) {
			t.Fatalf("%s no longer valid", ts)
		}
	}
	if v, _ := e.Validate(ValidateInput{ID: "minimal"}); !func() bool { x, _ := v.Get("valid"); return x.Bool() }() {
		t.Fatalf("validate %s", ojson.Compact(v))
	}
}

// belowMinimums reports a display string cut below every D.1 floor (80
// code units), i.e. a packet that needed the emergency caps.
func belowMinimums(v ojson.Value) bool {
	below := false
	walkStrings(v, "", func(_, s string) {
		if strings.HasSuffix(s, "…") && ojson.UTF16Len(s) < resumeMinTitle {
			below = true
		}
	})
	return below
}

// TestD31ResumeBudget: every packet fits its budget, the stale
// diagnostic is always present, and the advisory critical path is dropped
// before any text goes below the D.1 minimums.
func TestD31ResumeBudget(t *testing.T) {
	e, root := seedGraphRoadmap(t)
	// Make the checkpoint stale so both D.3.1 members are present.
	os.WriteFile(filepath.Join(root.Path, ".opencode/workplan/roadmap.md"), []byte("# edited\n"), 0o644)
	off := *e
	off.noD31 = true
	withCP, dropped, costly := 0, 0, 0
	for max := 4096; max <= 16000; max += d31Step(max) {
		for _, limit := range []int{1, 8, 20} {
			on, text, err := e.Resume(ResumeInput{ID: "roadmap", MaxChars: &max, Limit: &limit})
			if err != nil {
				t.Fatal(err)
			}
			if ojson.UTF16Len(text) > max {
				t.Fatalf("%d/%d: %d over budget", max, limit, ojson.UTF16Len(text))
			}
			o, _, _ := off.Resume(ResumeInput{ID: "roadmap", MaxChars: &max, Limit: &limit})
			// The advisory critical path is kept only while no text goes
			// below the D.1 minimums.
			if has3(on, "criticalPath") && belowMinimums(on) {
				t.Fatalf("%d/%d: critical path kept in a packet below the D.1 minimums", max, limit)
			}
			if belowMinimums(on) && !belowMinimums(o) {
				costly++ // the stale diagnostic alone (a pinned safety item)
			}
			cp, _ := on.Get("checkpoint")
			if d, _ := cp.Get("diagnostic"); !strings.HasPrefix(d.Str(), "changed: ") {
				t.Fatalf("%d/%d: diagnostic %s", max, limit, ojson.Compact(d))
			}
			if has3(on, "criticalPath") {
				withCP++
			} else {
				dropped++
			}
		}
	}
	if withCP == 0 {
		t.Fatal("the critical path never appears")
	}
	t.Logf("critical path shown in %d packets, dropped to keep the minimums in %d; stale diagnostic alone needed the emergency caps in %d", withCP, dropped, costly)
	if dropped == 0 {
		t.Fatal("no budget exercised dropping the critical path")
	}
}

func d31Step(max int) int {
	if max < 6000 {
		return 8
	}
	return 128
}
