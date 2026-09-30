package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Tests for the D.1 approved design changes (contracts §11). Oracle-vector
// divergences are covered by d1_vectors_test.go.

func mustMutate(t *testing.T, e *Engine, tool, input string) Output {
	t.Helper()
	p, err := ojson.Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	out, err := runMutation(context.Background(), e, tool, p.Value, &countingAuth{})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return out
}

func words(prefix string, n int) string {
	var b strings.Builder
	b.WriteString(prefix)
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, " clause-%d keeps the text readable", i)
	}
	return b.String()[:n]
}

// seedRoadmap creates a synthetic 13-phase/38-step roadmap with long prose
// and a fresh checkpoint (the shape of the owner's real plan, invented
// content only).
func seedRoadmap(t *testing.T) (*Engine, testutil.Root) {
	t.Helper()
	root := testutil.NewRoot(t, "empty-workspace")
	e := mustEngine(t, root.Path)
	var phases []string
	step := 0
	for p := 0; p < 13; p++ {
		n := 3
		if p == 12 {
			n = 2
		}
		var steps []string
		for s := 0; s < n; s++ {
			step++
			status := "draft"
			if p < 2 {
				status = "completed"
			}
			steps = append(steps, fmt.Sprintf(`{"id":"s%d","title":%q,"target":%q,"action":%q,"validation":%q,"status":%q}`,
				step, words(fmt.Sprintf("Step %d title", step), 70), words("src/module/, tests/", 220),
				words(fmt.Sprintf("R%02d action", step), 330), words("Validation", 300), status))
		}
		status := "draft"
		if p < 2 {
			status = "completed"
		}
		phases = append(phases, fmt.Sprintf(`{"id":"p%d","title":%q,"status":%q,"steps":[%s]}`, p, words(fmt.Sprintf("Phase %d title", p), 90), status, strings.Join(steps, ",")))
	}
	list := func(prefix string, n, size int) string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("%q", words(fmt.Sprintf("%s %d", prefix, i), size)))
		}
		return "[" + strings.Join(out, ",") + "]"
	}
	var files []string
	for i := 0; i < 8; i++ {
		files = append(files, fmt.Sprintf("%q", fmt.Sprintf(".opencode/workplan/some-long-related-plan-name-%d.json", i)))
	}
	mustMutate(t, e, "workplan_create", fmt.Sprintf(`{"id":"roadmap","title":"Synthetic staged roadmap","goal":%q,
		"scope":%s,"nonGoals":%s,"constraints":%s,"relevantFiles":[%s],"phases":[%s],
		"reviewFindings":[{"severity":"minor","title":%q,"detail":%q}],"notes":["n1","n2"]}`,
		words("Goal", 290), list("Scope", 6, 180), list("Non-goal", 5, 150), list("Constraint", 8, 200), strings.Join(files, ","),
		strings.Join(phases, ","), words("Finding", 90), words("Detail", 200)))
	mustMutate(t, e, "workplan_checkpoint", fmt.Sprintf(`{"id":"roadmap","summary":%q,"nextAction":%q,"phaseId":"p2","stepId":"s7",
		"guardrails":%s,"references":[".opencode/workplan/roadmap.json",".opencode/workplan/roadmap.md","docs/spec-a.md","docs/spec-b.md"],
		"recentValidation":%s}`, words("Summary", 700), words("Next action", 300), list("Guardrail", 3, 200), list("Validation", 3, 200)))
	return e, root
}

// walkStrings visits every string with its path.
func walkStrings(v ojson.Value, path string, f func(path, s string)) {
	switch v.Kind() {
	case ojson.Object:
		for _, m := range v.Members() {
			walkStrings(m.Value, path+"."+m.Key, f)
		}
	case ojson.Array:
		for _, el := range v.Elems() {
			walkStrings(el, path+"[]", f)
		}
	case ojson.String:
		f(path, v.Str())
	}
}

func pageReturned(v ojson.Value) int {
	pg, _ := v.Get("page")
	r, _ := pg.Get("returned")
	n, _ := r.Float()
	return int(n)
}

// checkReadable asserts the D.1 readability invariants for a packet that
// is not at an emergency level (more than one page item): no display
// string below the minimums, and no protected string shortened.
func checkReadable(t *testing.T, label string, v ojson.Value) {
	t.Helper()
	walkStrings(v, "", func(p, s string) {
		if !strings.HasSuffix(s, "…") {
			return
		}
		n := ojson.UTF16Len(s)
		protected := p == ".path" || p == ".planFile" || p == ".instruction" ||
			strings.HasSuffix(p, ".relevantFiles[]") || strings.HasSuffix(p, ".references[]") || strings.HasSuffix(p, ".reference")
		title := strings.HasSuffix(p, "itle")
		switch {
		case protected:
			t.Errorf("%s: protected %s truncated", label, p)
		case title && n < resumeMinTitle-1:
			t.Errorf("%s: title %s shortened to %d", label, p, n)
		case !title && n < resumeMinLong-1:
			t.Errorf("%s: %s shortened to %d", label, p, n)
		}
	})
}

// checkTargetOrder: a page smaller than the target page (while enough
// items remain) only carries page-item prose at the 120 floor, i.e. text
// shrank before the page did.
func checkTargetOrder(t *testing.T, label string, v ojson.Value, budget, limit int) {
	t.Helper()
	pgv, _ := v.Get("page")
	omv, _ := pgv.Get("omitted")
	om, _ := omv.Float()
	ret := pageReturned(v)
	tg := resumeTargetPage(budget, limit)
	if ret == 0 || ret >= tg || ret+int(om) < tg {
		return
	}
	items, _ := pgv.Get("items")
	walkStrings(items, "page.items", func(p, s string) {
		if strings.HasSuffix(s, "…") && !strings.HasSuffix(p, "itle") && ojson.UTF16Len(s) > resumeMinLong {
			t.Errorf("%s: page %d below target %d with %s at %d units", label, ret, tg, p, ojson.UTF16Len(s))
		}
	})
}

// TestD1ResumeReadability is item A on a roadmap-shaped plan: the default
// budget carries full text with fewer items; smaller budgets keep the
// minimums; every page fits and the cursor covers every item.
func TestD1ResumeReadability(t *testing.T) {
	e, _ := seedRoadmap(t)
	for _, b := range []int{64000, 20000, 12000, 8000, 6000} {
		b := b
		in := ResumeInput{ID: "roadmap", MaxChars: &b}
		seen, pages := 0, 0
		for {
			v, text, err := e.Resume(in)
			if err != nil {
				t.Fatalf("budget %d: %v", b, err)
			}
			if n := ojson.UTF16Len(text); n > b {
				t.Fatalf("budget %d: %d code units", b, n)
			}
			ret := pageReturned(v)
			if ret == 0 {
				t.Fatalf("budget %d: empty page", b)
			}
			checkReadable(t, fmt.Sprintf("budget %d page %d", b, pages), v)
			cp, _ := v.Get("checkpoint")
			sum, _ := cp.Get("summary")
			next, _ := cp.Get("nextAction")
			if ojson.UTF16Len(sum.Str()) < resumeMinLong-1 || ojson.UTF16Len(next.Str()) < resumeMinLong-1 {
				t.Fatalf("budget %d: summary/nextAction unreadable", b)
			}
			if b >= 20000 {
				// Room for full text at the target page.
				tc, _ := v.Get("truncatedFieldCount")
				if n, _ := tc.Float(); n != 0 {
					t.Errorf("budget %d: %v fields truncated, want none", b, n)
				}
			}
			// Text shrinks before the page drops below the target: a page
			// smaller than the target only carries floor-level item prose.
			checkTargetOrder(t, fmt.Sprintf("budget %d", b), v, b, DefaultResumeLimit)
			if b >= 12000 && (ojson.UTF16Len(sum.Str()) < resumePinnedMin-1 && !strings.HasPrefix(words("Summary", 700), sum.Str())) {
				t.Errorf("budget %d: current-work summary at %d units, want >= %d", b, ojson.UTF16Len(sum.Str()), resumePinnedMin)
			}
			seen += ret
			pages++
			pg, _ := v.Get("page")
			nc, _ := pg.Get("nextCursor")
			if nc.Kind() != ojson.String {
				tot, _ := pg.Get("total")
				if n, _ := tot.Float(); int(n) != seen {
					t.Fatalf("budget %d: paged %d of %v", b, seen, n)
				}
				break
			}
			c := nc.Str()
			in.Cursor = &c
		}
		t.Logf("budget %d: %d pages", b, pages)
	}
	// The default budget surveys several items with readable text.
	v, _, err := e.Resume(ResumeInput{ID: "roadmap"})
	if err != nil {
		t.Fatal(err)
	}
	if r := pageReturned(v); r < 4 {
		t.Fatalf("default budget returned %d items", r)
	}
}

func TestD1ResumeTargetPage(t *testing.T) {
	for _, c := range []struct{ max, limit, want int }{
		{4096, 20, 2}, {5999, 20, 2}, {6000, 20, 4}, {11999, 20, 4}, {12000, 20, 8}, {64000, 20, 8}, {64000, 3, 3}, {4096, 1, 1},
	} {
		if got := resumeTargetPage(c.max, c.limit); got != c.want {
			t.Errorf("target(%d, %d) = %d want %d", c.max, c.limit, got, c.want)
		}
	}
}

// TestD1ResumeMinimumBudget: at the smallest accepted budget a packet
// still fits, and anything shortened below the minimums is explicit.
func TestD1ResumeMinimumBudget(t *testing.T) {
	e, _ := seedRoadmap(t)
	b := MinResumeMaxChars
	v, text, err := e.Resume(ResumeInput{ID: "roadmap", MaxChars: &b})
	if err != nil {
		t.Fatal(err)
	}
	if ojson.UTF16Len(text) > b || pageReturned(v) != 1 {
		t.Fatalf("len %d returned %d", ojson.UTF16Len(text), pageReturned(v))
	}
	sf, _ := v.Get("safety")
	ov, _ := sf.Get("overflow")
	tc, _ := v.Get("truncatedFieldCount")
	n, _ := tc.Float()
	cut := 0
	walkStrings(v, "", func(p, s string) {
		if strings.HasSuffix(s, "…") {
			cut++
		}
	})
	if cut > 0 && (!ov.Bool() || int(n) < cut) {
		t.Fatalf("truncation not marked: cut %d count %v overflow %v", cut, n, ov.Bool())
	}
	// Machine fields are intact.
	h, _ := v.Get("hashes")
	ph, _ := h.Get("planHash")
	if len(ph.Str()) != 64 {
		t.Fatal("planHash missing")
	}
}

// TestD1FilteredRead is item B.
func TestD1FilteredRead(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := mustEngine(t, root.Path)
	before := testutil.Fingerprint(t, root.Path)
	full, err := e.Read(ReadInput{ID: "full-plan"})
	if err != nil {
		t.Fatal(err)
	}
	tr := true
	fullNotes, _ := e.Read(ReadInput{ID: "full-plan", IncludeNotes: &tr})
	if string(ojson.Pretty(full)) != string(ojson.Pretty(fullNotes)) {
		t.Fatal("includeNotes must not change an unfiltered read")
	}
	wp, _ := full.Get("workplan")
	phases, _ := wp.Get("phases")
	ph0, _ := phases.Elems()[0].Get("id")
	pid := ph0.Str()
	slice, err := e.Read(ReadInput{ID: "full-plan", PhaseID: &pid})
	if err != nil {
		t.Fatal(err)
	}
	sw, _ := slice.Get("workplan")
	for _, k := range []string{"phases", "reviewFindings", "notes"} {
		if _, ok := sw.Get(k); ok {
			t.Errorf("slice workplan has %s", k)
		}
	}
	for _, k := range []string{"id", "goal", "planFile", "status", "scope"} {
		if _, ok := sw.Get(k); !ok {
			t.Errorf("slice workplan lacks %s", k)
		}
	}
	pl, _ := slice.Get("plan")
	if _, ok := pl.Get("content"); ok {
		t.Error("filtered read must omit Markdown by default")
	}
	if len(ojson.Pretty(slice)) >= len(ojson.Pretty(full)) {
		t.Errorf("slice (%d) not smaller than full read (%d)", len(ojson.Pretty(slice)), len(ojson.Pretty(full)))
	}
	withNotes, _ := e.Read(ReadInput{ID: "full-plan", PhaseID: &pid, IncludeNotes: &tr})
	nw, _ := withNotes.Get("workplan")
	for _, k := range []string{"reviewFindings", "notes"} {
		got, ok := nw.Get(k)
		want, _ := wp.Get(k)
		if !ok || string(ojson.Compact(got)) != string(ojson.Compact(want)) {
			t.Errorf("includeNotes: %s missing or different", k)
		}
	}
	withMD, _ := e.Read(ReadInput{ID: "full-plan", PhaseID: &pid, IncludeMarkdown: &tr})
	mp, _ := withMD.Get("plan")
	if _, ok := mp.Get("content"); !ok {
		t.Error("explicit includeMarkdown=true must include Markdown")
	}
	for _, v := range []ojson.Value{slice, withNotes, withMD} {
		for _, k := range []string{"planHash", "stateHash", "selection", "path", "dependencies", "slice"} {
			if _, ok := v.Get(k); !ok {
				t.Errorf("slice lacks %s", k)
			}
		}
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("read wrote: %v", d)
	}
	// Input contract: both surfaces accept the boolean, reject others.
	for _, s := range []Surface{SurfaceCore, SurfaceNative} {
		p, _ := ojson.Parse([]byte(`{"id":"full-plan","phaseId":"x","includeNotes":true}`))
		in, err := ParseReadInput(p.Value, s)
		if err != nil || in.IncludeNotes == nil || !*in.IncludeNotes {
			t.Fatalf("includeNotes not accepted: %v", err)
		}
		p, _ = ojson.Parse([]byte(`{"id":"full-plan","includeNotes":"yes"}`))
		if _, err := ParseReadInput(p.Value, s); err == nil || err.Error() != "Invalid read input: includeNotes: Invalid input: expected boolean, received string" {
			t.Fatalf("bad includeNotes: %v", err)
		}
	}
}

// TestD1MarkdownDrift is item C.
func TestD1MarkdownDrift(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := mustEngine(t, root.Path)
	v, _ := e.Validate(ValidateInput{ID: "minimal"})
	if _, ok := v.Get("warnings"); ok {
		t.Fatal("generated Markdown must not warn")
	}
	md := filepath.Join(root.Path, ".opencode", "workplan", "minimal.md")
	data, _ := os.ReadFile(md)
	os.WriteFile(md, append(data, []byte("\nHand-written note.\n")...), 0o600)
	before := testutil.Fingerprint(t, root.Path)
	v, _ = e.Validate(ValidateInput{ID: "minimal"})
	valid, _ := v.Get("valid")
	w, ok := v.Get("warnings")
	if !valid.Bool() || !ok || len(w.Elems()) != 1 || !strings.Contains(w.Elems()[0].Str(), ".opencode/workplan/minimal.md") {
		t.Fatalf("validate: valid %v warnings %s", valid.Bool(), ojson.Compact(w))
	}
	id := "minimal"
	d, _ := e.Doctor(DoctorInput{ID: &id})
	plans, _ := d.Get("plans")
	pw, ok := plans.Elems()[0].Get("warnings")
	pv, _ := plans.Elems()[0].Get("valid")
	if !ok || len(pw.Elems()) != 1 || !pv.Bool() {
		t.Fatalf("doctor plan warnings %s valid %v", ojson.Compact(pw), pv.Bool())
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("diagnostics wrote: %v", d)
	}
}

func gateErr(t *testing.T, err error, status string, paths ...string) {
	t.Helper()
	var g *StatusGateError
	if !errors.As(err, &g) || g.Status != status {
		t.Fatalf("want status gate for %s, got %v", status, err)
	}
	if ErrorClass(err) != "invalid_structure" {
		t.Fatalf("class %s", ErrorClass(err))
	}
	var got []string
	for _, is := range g.Issues {
		got = append(got, is.PathString())
		if !strings.Contains(err.Error(), is.String()) {
			t.Fatalf("message lacks %q: %s", is.String(), err)
		}
	}
	if strings.Join(got, ",") != strings.Join(paths, ",") {
		t.Fatalf("issue paths %v want %v", got, paths)
	}
}

// TestD1StatusGate is item D (create/update) and the patch issue list.
func TestD1StatusGate(t *testing.T) {
	freezeClock(t)
	root := testutil.NewRoot(t, "empty-workspace")
	e := mustEngine(t, root.Path)
	incomplete := `"phases":[{"id":"p","title":"P","steps":[{"id":"a","title":"A","action":"do"},{"id":"b","title":"B"}]}]`
	for _, st := range []string{"in_progress", "review", "completed"} {
		before := testutil.Fingerprint(t, root.Path)
		p, _ := ojson.Parse([]byte(`{"id":"gate-` + strings.ReplaceAll(st, "_", "-") + `","goal":"g","status":"` + st + `",` + incomplete + `}`))
		auth := &countingAuth{}
		_, err := runMutation(context.Background(), e, "workplan_create", p.Value, auth)
		gateErr(t, err, st, "phases.0.steps.0.validation", "phases.0.steps.1.action", "phases.0.steps.1.validation")
		if auth.n != 0 {
			t.Fatal("gate must refuse before authorization")
		}
		if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
			t.Fatalf("refusal wrote: %v", d)
		}
	}
	// No phases at all.
	p, _ := ojson.Parse([]byte(`{"id":"empty-gate","goal":"g","status":"in_progress"}`))
	_, err := runMutation(context.Background(), e, "workplan_create", p.Value, &countingAuth{})
	gateErr(t, err, "in_progress", "phases")
	// draft, blocked and cancelled stay allowed.
	for _, st := range []string{"draft", "blocked", "cancelled"} {
		mustMutate(t, e, "workplan_create", `{"id":"ok-`+st+`","goal":"g","status":"`+st+`",`+incomplete+`}`)
	}
	// Update: gated statuses refused while incomplete, allowed once the
	// same call completes the structure.
	p, _ = ojson.Parse([]byte(`{"id":"ok-draft","status":"review"}`))
	_, err = runMutation(context.Background(), e, "workplan_update", p.Value, &countingAuth{})
	gateErr(t, err, "review", "phases.0.steps.0.validation", "phases.0.steps.1.action", "phases.0.steps.1.validation")
	mustMutate(t, e, "workplan_update", `{"id":"ok-draft","status":"blocked"}`)
	mustMutate(t, e, "workplan_update", `{"id":"ok-draft","appendNotes":["unrelated updates stay allowed"]}`)
	mustMutate(t, e, "workplan_update", `{"id":"ok-draft","status":"in_progress","updateSteps":[
		{"phaseId":"p","stepId":"a","validation":"check a"},{"phaseId":"p","stepId":"b","action":"do b","validation":"check b"}]}`)
	mustMutate(t, e, "workplan_create", `{"id":"ok-valid","goal":"g","status":"in_progress","phases":[{"id":"p","title":"P","steps":[{"id":"a","title":"A","action":"do","validation":"check"}]}]}`)

	// Patch validate returns the issue list.
	id := "ok-blocked"
	s, _ := e.load(id)
	pf := s.Plan.PlanFile
	patch := "*** Begin Patch\n*** Update File: " + pf + "\n@@\n-# " + id + "\n+# " + id + " (edited)\n*** End Patch"
	p, _ = ojson.Parse([]byte(fmt.Sprintf(`{"id":%q,"patchText":%q,"validate":true}`, id, patch)))
	out, err := runMutation(context.Background(), e, "workplan_patch", p.Value, &countingAuth{})
	if err != nil {
		t.Fatal(err)
	}
	val, _ := out.Metadata.Get("validation")
	issues, _ := val.Get("issues")
	cnt, _ := val.Get("issueCount")
	n, _ := cnt.Float()
	if int(n) != 3 || len(issues.Elems()) != 3 || issues.Elems()[0].Str() != "phases.0.steps.0.validation: Required for executable workplans" {
		t.Fatalf("patch validation %s", ojson.Compact(val))
	}
	if w, ok := val.Get("warnings"); !ok || len(w.Elems()) != 1 {
		t.Fatalf("patched Markdown must carry the drift warning: %s", ojson.Compact(val))
	}
}

// TestD1DoctorStrays is item F.
func TestD1DoctorStrays(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	dir := filepath.Join(root.Path, ".opencode", "workplan")
	for name, body := range map[string]string{
		"ghost.checkpoint.json":   "{}",
		"ghost.dependencies.json": "{}",
		"old-change.patch":        "*** Begin Patch\n",
		"scratch.md":              "# notes\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := mustEngine(t, root.Path)
	before := testutil.Fingerprint(t, root.Path)
	d, err := e.Doctor(DoctorInput{})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := d.Get("strayArtifacts")
	var got []string
	for _, x := range st.Elems() {
		n, _ := x.Get("name")
		k, _ := x.Get("kind")
		got = append(got, n.Str()+":"+k.Str())
	}
	want := "ghost.checkpoint.json:orphaned-sidecar,ghost.dependencies.json:orphaned-sidecar,old-change.patch:unclassified,scratch.md:unclassified"
	if strings.Join(got, ",") != want {
		t.Fatalf("strays %v", got)
	}
	w, _ := d.Get("warnings")
	if len(w.Elems()) != 4 || !strings.Contains(w.Elems()[0].Str(), "archive/") {
		t.Fatalf("warnings %s", ojson.Compact(w))
	}
	plans, _ := d.Get("plans")
	if v, _ := plans.Elems()[0].Get("valid"); !v.Bool() {
		t.Fatal("strays must not invalidate plans")
	}
	one := 1
	d, _ = e.Doctor(DoctorInput{Limit: &one})
	st, _ = d.Get("strayArtifacts")
	om, _ := d.Get("omittedStrayArtifacts")
	cnt, _ := d.Get("strayArtifactCount")
	if n, _ := om.Float(); len(st.Elems()) != 1 || n != 3 {
		t.Fatalf("limit: listed %d omitted %v", len(st.Elems()), n)
	}
	if n, _ := cnt.Float(); n != 4 {
		t.Fatalf("count %v", n)
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("doctor moved or changed files: %v", d)
	}
	// A clean root reports no stray members at all.
	clean := testutil.NewRoot(t, "minimal-valid")
	d, _ = mustEngine(t, clean.Path).Doctor(DoctorInput{})
	for _, k := range []string{"strayArtifacts", "strayArtifactCount", "warnings"} {
		if _, ok := d.Get(k); ok {
			t.Fatalf("clean root has %s", k)
		}
	}
}
