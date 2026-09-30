package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Tests for the D.2 approved design changes (contracts §12): the
// dependency graph drives resume, and order checks warn without refusing.
// Oracle-vector changes are covered by d2_vectors_test.go.

// d2RoadmapDeps is a 12-entry cross-phase graph over the synthetic
// roadmap (seedRoadmap): milestone M1 is p2-p5, M2 is p6-p9 and M3 is
// p10-p12, and each milestone waits on the previous one's gate step.
// Steps are numbered s1..s38 across phases (p0: s1-s3, p1: s4-s6, ...).
const d2RoadmapDeps = `[
	{"phaseId":"p3","stepId":"s10","dependsOn":[{"phaseId":"p2","stepId":"s7"},{"phaseId":"p1","stepId":"s4"}]},
	{"phaseId":"p4","stepId":"s13","dependsOn":[{"phaseId":"p3","stepId":"s10"},{"phaseId":"p2","stepId":"s8"}]},
	{"phaseId":"p5","stepId":"s16","dependsOn":[{"phaseId":"p4","stepId":"s13"}]},
	{"phaseId":"p6","stepId":"s19","dependsOn":[{"phaseId":"p5","stepId":"s16"}]},
	{"phaseId":"p7","stepId":"s22","dependsOn":[{"phaseId":"p5","stepId":"s16"}]},
	{"phaseId":"p8","stepId":"s25","dependsOn":[{"phaseId":"p6","stepId":"s19"},{"phaseId":"p7","stepId":"s22"}]},
	{"phaseId":"p9","stepId":"s28","dependsOn":[{"phaseId":"p8","stepId":"s25"}]},
	{"phaseId":"p10","stepId":"s31","dependsOn":[{"phaseId":"p9","stepId":"s28"}]},
	{"phaseId":"p10","stepId":"s32","dependsOn":[{"phaseId":"p5","stepId":"s16"}]},
	{"phaseId":"p11","stepId":"s34","dependsOn":[{"phaseId":"p10","stepId":"s31"}]},
	{"phaseId":"p12","stepId":"s37","dependsOn":[{"phaseId":"p11","stepId":"s34"},{"phaseId":"p10","stepId":"s32"}]},
	{"phaseId":"p12","stepId":"s38","dependsOn":[{"phaseId":"p12","stepId":"s37"}]}]`

// seedGraphRoadmap is the synthetic roadmap plus d2RoadmapDeps and a
// checkpoint refreshed after the dependency write (so it stays fresh).
func seedGraphRoadmap(t *testing.T) (*Engine, testutil.Root) {
	t.Helper()
	e, root := seedRoadmap(t)
	mustMutate(t, e, "workplan_update", `{"id":"roadmap","dependencies":`+d2RoadmapDeps+`}`)
	mustMutate(t, e, "workplan_checkpoint", fmt.Sprintf(`{"id":"roadmap","summary":%q,"nextAction":%q,"phaseId":"p2","stepId":"s7"}`,
		words("Summary", 700), words("Next action", 300)))
	return e, root
}

func strMember(v ojson.Value, keys ...string) string {
	for _, k := range keys {
		v, _ = v.Get(k)
	}
	return v.Str()
}

func numMember(v ojson.Value, keys ...string) int {
	for _, k := range keys {
		v, _ = v.Get(k)
	}
	f, _ := v.Float()
	return int(f)
}

func strs(v ojson.Value) []string {
	var out []string
	for _, x := range v.Elems() {
		out = append(out, x.Str())
	}
	return out
}

func containsPrefix(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// checkD2Order asserts the G1/G6 page order: ready active work first by
// non-increasing unblocks, then blocked work; blockedBy within the cap.
func checkD2Order(t *testing.T, label string, v ojson.Value, listCap int) {
	t.Helper()
	items, _ := v.Get("page")
	items, _ = items.Get("items")
	blocked := false
	last := -1
	for i, it := range items.Elems() {
		if strMember(it, "kind") != "active-work" {
			continue
		}
		switch strMember(it, "readiness") {
		case "ready":
			u := numMember(it, "unblocks")
			if blocked || (last >= 0 && u > last) {
				t.Fatalf("%s: item %d breaks the ready ranking", label, i)
			}
			last = u
		case "blocked":
			blocked = true
			bb, _ := it.Get("blockedBy")
			if n := len(bb.Elems()); n == 0 || n > listCap {
				t.Fatalf("%s: item %d blockedBy %d entries (cap %d)", label, i, n, listCap)
			}
		default:
			t.Fatalf("%s: item %d has no readiness", label, i)
		}
	}
}

// TestD2ResumeReadiness is G1/G6 on the 12-entry cross-phase graph.
func TestD2ResumeReadiness(t *testing.T) {
	e, root := seedGraphRoadmap(t)
	before := testutil.Fingerprint(t, root.Path)
	max, limit := 64000, 100
	v, text, err := e.Resume(ResumeInput{ID: "roadmap", MaxChars: &max, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := v.Get("checkpoint")
	cur, _ = cur.Get("current")
	if strMember(cur, "stepId") != "s7" || strMember(cur, "readiness") != "ready" || numMember(cur, "unblocks") != 12 {
		t.Fatalf("current %s", ojson.Compact(cur))
	}
	items, _ := v.Get("page")
	items, _ = items.Get("items")
	var order []string
	for _, it := range items.Elems() {
		if strMember(it, "kind") == "active-work" {
			order = append(order, strMember(it, "stepId")+":"+strMember(it, "readiness"))
		}
	}
	// s8 unblocks 11 open steps; the other ready steps unblock none and
	// keep plan order; blocked steps follow in plan order.
	want := "s8:ready,s9:ready,s11:ready,s12:ready,s14:ready,s15:ready,s17:ready,s18:ready,s20:ready,s21:ready,s23:ready,s24:ready," +
		"s26:ready,s27:ready,s29:ready,s30:ready,s33:ready,s35:ready,s36:ready," +
		"s10:blocked,s13:blocked,s16:blocked,s19:blocked,s22:blocked,s25:blocked,s28:blocked,s31:blocked,s32:blocked,s34:blocked,s37:blocked,s38:blocked"
	if strings.Join(order, ",") != want {
		t.Fatalf("order\n got %s\nwant %s", strings.Join(order, ","), want)
	}
	if numMember(items.Elems()[0], "unblocks") != 11 {
		t.Fatalf("s8 unblocks %d", numMember(items.Elems()[0], "unblocks"))
	}
	// s10 is blocked by the current step and nothing else: the completed
	// p1/s4 is met.
	for _, it := range items.Elems() {
		if strMember(it, "stepId") == "s13" {
			bb, _ := it.Get("blockedBy")
			if got := string(ojson.Compact(bb)); got != `[{"phaseId":"p3","stepId":"s10","status":"draft"},{"phaseId":"p2","stepId":"s8","status":"draft"}]` {
				t.Fatalf("s13 blockedBy %s", got)
			}
		}
		if strMember(it, "stepId") == "s10" {
			bb, _ := it.Get("blockedBy")
			if len(bb.Elems()) != 1 || strMember(bb.Elems()[0], "stepId") != "s7" {
				t.Fatalf("s10 blockedBy %s", ojson.Compact(bb))
			}
		}
	}
	checkD2Order(t, "64000", v, 8)
	if ojson.UTF16Len(text) > max {
		t.Fatal("over budget")
	}
	// The graph additions are the only difference from the D.2-off packet
	// apart from ordering: same totals, same items as a set.
	off := *e
	off.noGraph = true
	ov, _, err := off.Resume(ResumeInput{ID: "roadmap", MaxChars: &max, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if numMember(ov, "page", "total") != numMember(v, "page", "total") || numMember(ov, "counts", "activeWorkTotal") != numMember(v, "counts", "activeWorkTotal") {
		t.Fatal("totals differ from the D.2-off packet")
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("resume wrote: %v", d)
	}
}

// TestD2ResumeBudgets pages the graph roadmap at the D.1 budgets: every
// page fits, keeps the D.1 readability invariants and the D.2 order, and
// the default budget still returns a useful page.
func TestD2ResumeBudgets(t *testing.T) {
	e, _ := seedGraphRoadmap(t)
	off := *e
	off.noGraph = true
	for _, b := range []int{64000, 20000, 12000, 8000, 6000, 4096} {
		for _, limit := range []int{7, 20} {
			b, limit := b, limit
			in := ResumeInput{ID: "roadmap", MaxChars: &b, Limit: &limit}
			seen, pages, first := 0, 0, 0
			for page := 0; page < 200; page++ {
				v, text, err := e.Resume(in)
				if err != nil {
					t.Fatalf("budget %d: %v", b, err)
				}
				if ojson.UTF16Len(text) > b {
					t.Fatalf("budget %d over", b)
				}
				label := fmt.Sprintf("budget %d limit %d page %d", b, limit, page)
				if r := pageReturned(v); r > 1 {
					checkReadable(t, label, v)
				}
				checkTargetOrder(t, label, v, b, limit)
				checkD2Order(t, label, v, resumeListCap(b))
				if page == 0 {
					first = pageReturned(v)
				}
				seen += pageReturned(v)
				pages++
				next, _ := v.Get("page")
				next, _ = next.Get("nextCursor")
				if next.Kind() != ojson.String {
					if seen != numMember(v, "page", "total") {
						t.Fatalf("%s: paged %d of %d", label, seen, numMember(v, "page", "total"))
					}
					break
				}
				c := next.Str()
				in.Cursor = &c
			}
			ov, _, _ := off.Resume(ResumeInput{ID: "roadmap", MaxChars: &b, Limit: &limit})
			t.Logf("budget %5d limit %2d: first page %d items (D.2-off %d), %d pages", b, limit, first, pageReturned(ov), pages)
			if b >= 12000 && first < 4 {
				t.Fatalf("budget %d: first page only %d items", b, first)
			}
		}
	}
}

// TestD2OrderWarnings is G2: out-of-order status changes succeed with a
// warning; validate and doctor report the violation without failing.
func TestD2OrderWarnings(t *testing.T) {
	e, root := seedGraphRoadmap(t)
	p, _ := ojson.Parse([]byte(`{"id":"roadmap","updateSteps":[{"phaseId":"p4","stepId":"s13","status":"completed"}]}`))
	auth := &countingAuth{}
	out, err := runMutation(context.Background(), e, "workplan_update", p.Value, auth)
	if err != nil || auth.n != 1 {
		t.Fatalf("order checks must never refuse: %v (auth %d)", err, auth.n)
	}
	w, _ := out.Value.Get("warnings")
	if ws := strs(w); len(ws) != 1 || !strings.Contains(ws[0], "Order warning: step p4/s13 was set to completed while its prerequisites are not completed: p3/s10 (draft), p2/s8 (draft)") {
		t.Fatalf("update warnings %v", ws)
	}
	s, _ := e.load("roadmap")
	if s.Plan.Phases[4].Steps[0].Status != "completed" {
		t.Fatal("status not applied")
	}
	v, _ := e.Validate(ValidateInput{ID: "roadmap"})
	valid, _ := v.Get("valid")
	vw, _ := v.Get("warnings")
	if !valid.Bool() || !containsPrefix(strs(vw), "dependencies: Order warning: step p4/s13 is completed but its prerequisites are not completed: p3/s10 (draft), p2/s8 (draft)") {
		t.Fatalf("validate valid=%v warnings %v", valid.Bool(), strs(vw))
	}
	id := "roadmap"
	d, _ := e.Doctor(DoctorInput{ID: &id})
	pl, _ := d.Get("plans")
	pw, _ := pl.Elems()[0].Get("warnings")
	pv, _ := pl.Elems()[0].Get("valid")
	off := *e
	off.noGraph = true
	od, _ := off.Doctor(DoctorInput{ID: &id})
	opl, _ := od.Get("plans")
	opv, _ := opl.Elems()[0].Get("valid")
	// valid is unchanged by D.2 (here false only because the checkpoint
	// went stale with the update).
	if pv.Bool() != opv.Bool() || !containsPrefix(strs(pw), "Order warning: step p4/s13") {
		t.Fatalf("doctor valid=%v warnings %v", pv.Bool(), strs(pw))
	}
	// review warns too.
	out = mustMutate(t, e, "workplan_update", `{"id":"roadmap","updateSteps":[{"phaseId":"p6","stepId":"s19","status":"review"}]}`)
	w, _ = out.Value.Get("warnings")
	if ws := strs(w); len(ws) != 1 || !strings.Contains(ws[0], "step p6/s19 was set to review while its prerequisites are not completed: p5/s16 (draft)") {
		t.Fatalf("review warnings %v", ws)
	}
	v, _ = e.Validate(ValidateInput{ID: "roadmap"})
	vw, _ = v.Get("warnings")
	if !containsPrefix(strs(vw), "Order warning: step p6/s19 is review") {
		t.Fatalf("validate review %v", strs(vw))
	}
	mustMutate(t, e, "workplan_update", `{"id":"roadmap","updateSteps":[{"phaseId":"p6","stepId":"s19","status":"blocked"}]}`)
	// in_progress warns too; a step whose prerequisites are met does not.
	out = mustMutate(t, e, "workplan_update", `{"id":"roadmap","updateSteps":[{"phaseId":"p3","stepId":"s10","status":"in_progress"},{"phaseId":"p2","stepId":"s9","status":"in_progress"}]}`)
	w, _ = out.Value.Get("warnings")
	if ws := strs(w); len(ws) != 1 || !strings.Contains(ws[0], "step p3/s10 was set to in_progress") {
		t.Fatalf("in_progress warnings %v", ws)
	}
	// Setting the prerequisites complete clears nothing retroactively in
	// the update result, but validate no longer reports s10.
	out = mustMutate(t, e, "workplan_update", `{"id":"roadmap","updateSteps":[{"phaseId":"p2","stepId":"s7","status":"completed"}]}`)
	if _, ok := out.Value.Get("warnings"); ok {
		t.Fatal("completing a ready step must not warn")
	}
	v, _ = e.Validate(ValidateInput{ID: "roadmap"})
	vw, _ = v.Get("warnings")
	if containsPrefix(strs(vw), "step p3/s10 is") {
		t.Fatalf("met prerequisites still reported: %v", strs(vw))
	}
	_ = root
}

// TestD2CancelledPrerequisite is G3, including archived terminal summaries.
func TestD2CancelledPrerequisite(t *testing.T) {
	e, root := seedGraphRoadmap(t)
	out := mustMutate(t, e, "workplan_update", `{"id":"roadmap","updateSteps":[{"phaseId":"p5","stepId":"s16","status":"cancelled"}]}`)
	w, _ := out.Value.Get("warnings")
	if ws := strs(w); len(ws) != 1 || !strings.Contains(ws[0], "Step p5/s16 was cancelled; its dependents are now blocked by a cancelled prerequisite: p6/s19, p7/s22, p10/s32") {
		t.Fatalf("cancel warnings %v", ws)
	}
	v, _ := e.Validate(ValidateInput{ID: "roadmap"})
	vw := strs(func() ojson.Value { x, _ := v.Get("warnings"); return x }())
	for _, d := range []string{"p6/s19", "p7/s22", "p10/s32"} {
		if !containsPrefix(vw, "dependencies: Step "+d+" is blocked by cancelled prerequisite p5/s16") {
			t.Fatalf("validate lacks %s: %v", d, vw)
		}
	}
	if valid, _ := v.Get("valid"); !valid.Bool() {
		t.Fatal("cancelled prerequisites must not invalidate the plan")
	}
	max, limit := 64000, 100
	r, _, err := e.Resume(ResumeInput{ID: "roadmap", MaxChars: &max, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	var sw []string
	for _, x := range strs(func() ojson.Value { x, _ := r.Get("safety"); x, _ = x.Get("unverifiedWarnings"); return x }()) {
		if strings.HasPrefix(x, "Dependency order warning: ") {
			sw = append(sw, x)
		}
	}
	if len(sw) != 3 || !strings.HasPrefix(sw[0], "Dependency order warning: Step p6/s19 is blocked by cancelled prerequisite p5/s16") {
		t.Fatalf("resume warnings %v", sw)
	}
	items, _ := r.Get("page")
	items, _ = items.Get("items")
	for _, it := range items.Elems() {
		if strMember(it, "stepId") == "s22" {
			bb, _ := it.Get("blockedBy")
			if string(ojson.Compact(bb)) != `[{"phaseId":"p5","stepId":"s16","status":"cancelled"}]` {
				t.Fatalf("s22 blockedBy %s", ojson.Compact(bb))
			}
		}
	}

	// Archived prerequisites resolve through terminalSummaries: a
	// cancelled summary blocks, a completed one is met.
	small := testutil.NewRoot(t, "empty-workspace")
	se := mustEngine(t, small.Path)
	mustMutate(t, se, "workplan_create", `{"id":"arch","goal":"g","phases":[{"id":"b","title":"B","steps":[{"id":"b1","title":"B1"},{"id":"b2","title":"B2"}]}]}`)
	dep := `{"schemaVersion":1,"id":"arch","updatedAt":"2026-01-01T00:00:00.000Z","dependencies":[
		{"phaseId":"b","stepId":"b1","dependsOn":[{"phaseId":"a","stepId":"a1"}]},
		{"phaseId":"b","stepId":"b2","dependsOn":[{"phaseId":"a","stepId":"a2"}]}],
		"terminalSummaries":[{"phaseId":"a","stepId":"a1","title":"A1","status":"cancelled"},{"phaseId":"a","stepId":"a2","title":"A2","status":"completed"}]}`
	if err := os.WriteFile(filepath.Join(small.Path, ".opencode", "workplan", "arch.dependencies.json"), []byte(dep), 0o600); err != nil {
		t.Fatal(err)
	}
	v, _ = se.Validate(ValidateInput{ID: "arch"})
	vw = strs(func() ojson.Value { x, _ := v.Get("warnings"); return x }())
	if len(vw) != 1 || !strings.HasPrefix(vw[0], "dependencies: Step b/b1 is blocked by cancelled prerequisite a/a1") {
		t.Fatalf("terminal summary warnings %v", vw)
	}
	r, _, _ = se.Resume(ResumeInput{ID: "arch", MaxChars: &max, Limit: &limit})
	cur, _ := r.Get("checkpoint")
	cur, _ = cur.Get("current")
	if strMember(cur, "stepId") != "b1" || strMember(cur, "readiness") != "blocked" {
		t.Fatalf("current %s", ojson.Compact(cur))
	}
	items, _ = r.Get("page")
	items, _ = items.Get("items")
	if first := items.Elems()[0]; strMember(first, "stepId") != "b2" || strMember(first, "readiness") != "ready" {
		t.Fatalf("archived completed prerequisite must be met: %s", ojson.Compact(first))
	}
	_ = root
}

// TestD2PhaseReplacement is G4: replacing phases re-validates the stored
// sidecar in the same prepared write.
func TestD2PhaseReplacement(t *testing.T) {
	freezeClock(t)
	root := testutil.NewRoot(t, "empty-workspace")
	e := mustEngine(t, root.Path)
	mustMutate(t, e, "workplan_create", `{"id":"rep","goal":"g","phases":[
		{"id":"a","title":"A","steps":[{"id":"a1","title":"A1"},{"id":"a2","title":"A2"}]},
		{"id":"b","title":"B","steps":[{"id":"b1","title":"B1"}]}]}`)
	mustMutate(t, e, "workplan_update", `{"id":"rep","dependencies":[{"phaseId":"b","stepId":"b1","dependsOn":[{"phaseId":"a","stepId":"a2"}]}]}`)
	before := testutil.Fingerprint(t, root.Path)
	for _, in := range []struct{ input, want string }{
		{`{"id":"rep","phases":[{"id":"a","title":"A","steps":[{"id":"a1","title":"A1"}]},{"id":"b","title":"B","steps":[{"id":"b1","title":"B1"}]}]}`,
			"Invalid dependency metadata: dependencies.0.dependsOn.0: Step a/a2 does not exist; the phase replacement would leave these dependency links dangling. Replace the dependencies in the same update."},
		{`{"id":"rep","phases":[{"id":"a","title":"A","steps":[{"id":"a1","title":"A1"},{"id":"a2","title":"A2"}]}]}`,
			"Invalid dependency metadata: dependencies.0: Source step b/b1 does not exist; the phase replacement would leave these dependency links dangling. Replace the dependencies in the same update."},
	} {
		p, _ := ojson.Parse([]byte(in.input))
		auth := &countingAuth{}
		_, err := runMutation(context.Background(), e, "workplan_update", p.Value, auth)
		if err == nil || err.Error() != in.want || ErrorClass(err) != "invalid_structure" {
			t.Fatalf("got %v (class %s)\nwant %s", err, ErrorClass(err), in.want)
		}
		if auth.n != 0 {
			t.Fatal("refusal must come before authorization")
		}
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("refusal wrote: %v", d)
	}
	// Keeping every linked step, or replacing the dependencies in the same
	// call, is accepted.
	mustMutate(t, e, "workplan_update", `{"id":"rep","phases":[{"id":"a","title":"A2 first","steps":[{"id":"a2","title":"A2"},{"id":"a1","title":"A1"}]},{"id":"b","title":"B","steps":[{"id":"b1","title":"B1"}]}]}`)
	mustMutate(t, e, "workplan_update", `{"id":"rep","phases":[{"id":"a","title":"A","steps":[{"id":"a1","title":"A1"}]}],"dependencies":[]}`)
}

// TestD2DependencyWrites is G5: empty entries are refused, backward links
// warn, compaction preview lists dependents of archived steps, and
// inspect shows prerequisites and dependents.
func TestD2DependencyWrites(t *testing.T) {
	freezeClock(t)
	root := testutil.NewRoot(t, "empty-workspace")
	e := mustEngine(t, root.Path)
	mustMutate(t, e, "workplan_create", `{"id":"dw","goal":"g","phases":[
		{"id":"a","title":"A","status":"completed","steps":[{"id":"a1","title":"A1","status":"completed"},{"id":"a2","title":"A2","status":"completed"}]},
		{"id":"b","title":"B","steps":[{"id":"b1","title":"B1"},{"id":"b2","title":"B2"}]}]}`)
	p, _ := ojson.Parse([]byte(`{"id":"dw","dependencies":[{"phaseId":"b","stepId":"b1","dependsOn":[]}]}`))
	auth := &countingAuth{}
	_, err := runMutation(context.Background(), e, "workplan_update", p.Value, auth)
	if err == nil || err.Error() != "Invalid dependency metadata: dependencies.0.dependsOn: Dependency entry must list at least one prerequisite" || auth.n != 0 || ErrorClass(err) != "invalid_structure" {
		t.Fatalf("empty dependsOn: %v (auth %d)", err, auth.n)
	}
	// Existing dependency refusals (dangling target, cycle) share the
	// class; their text is unchanged.
	for _, in := range []string{
		`{"id":"dw","dependencies":[{"phaseId":"b","stepId":"b1","dependsOn":[{"phaseId":"ghost","stepId":"x"}]}]}`,
		`{"id":"dw","dependencies":[{"phaseId":"b","stepId":"b1","dependsOn":[{"phaseId":"b","stepId":"b2"}]},{"phaseId":"b","stepId":"b2","dependsOn":[{"phaseId":"b","stepId":"b1"}]}]}`,
	} {
		p, _ := ojson.Parse([]byte(in))
		_, err := runMutation(context.Background(), e, "workplan_update", p.Value, &countingAuth{})
		if err == nil || !strings.HasPrefix(err.Error(), "Invalid dependency metadata: dependencies") || ErrorClass(err) != "invalid_structure" {
			t.Fatalf("dependency refusal %v (class %s)", err, ErrorClass(err))
		}
	}
	out := mustMutate(t, e, "workplan_update", `{"id":"dw","dependencies":[
		{"phaseId":"b","stepId":"b1","dependsOn":[{"phaseId":"b","stepId":"b2"},{"phaseId":"a","stepId":"a2"}]}]}`)
	w, _ := out.Value.Get("warnings")
	if ws := strs(w); len(ws) != 1 || ws[0] != "dependencies.0.dependsOn.0: Backward link: b/b1 depends on b/b2, which comes later in plan order." {
		t.Fatalf("backward warnings %v", ws)
	}
	v, _ := e.Validate(ValidateInput{ID: "dw"})
	if vw, _ := v.Get("warnings"); !containsPrefix(strs(vw), "Backward link: b/b1 depends on b/b2") {
		t.Fatalf("validate %v", strs(vw))
	}
	// Inspect: prerequisites (with status) and dependents per step.
	ins, err := e.Inspect(InspectInput{ID: "dw"})
	if err != nil {
		t.Fatal(err)
	}
	steps, _ := ins.Get("steps")
	byID := map[string]ojson.Value{}
	for _, st := range steps.Elems() {
		byID[strMember(st, "id")] = st
	}
	pre, _ := byID["b1"].Get("prerequisites")
	if string(ojson.Compact(pre)) != `[{"phaseId":"b","stepId":"b2","status":"draft"},{"phaseId":"a","stepId":"a2","status":"completed"}]` {
		t.Fatalf("b1 prerequisites %s", ojson.Compact(pre))
	}
	dep, _ := byID["a2"].Get("dependents")
	if string(ojson.Compact(dep)) != `[{"phaseId":"b","stepId":"b1"}]` {
		t.Fatalf("a2 dependents %s", ojson.Compact(dep))
	}
	if _, ok := byID["a2"].Get("readiness"); ok {
		t.Fatal("closed steps carry no readiness")
	}
	if strMember(byID["b1"], "readiness") != "blocked" || numMember(byID["b2"], "unblocks") != 1 {
		t.Fatalf("b1/b2 %s %s", ojson.Compact(byID["b1"]), ojson.Compact(byID["b2"]))
	}
	cp, _ := ins.Get("criticalPath")
	if numMember(cp, "length") != 2 {
		t.Fatalf("criticalPath %s", ojson.Compact(cp))
	}
	// Compaction preview: archiving phase a lists b/b1 behind a/a2.
	pv, err := e.CompactPreview(parseT(t, `{"id":"dw","archiveReason":"tidy","completedPhaseIds":["a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	ap, ok := pv.Get("archivedPrerequisites")
	if !ok || string(ojson.Compact(ap)) != `[{"phaseId":"a","stepId":"a2","status":"completed","dependents":[{"phaseId":"b","stepId":"b1"}]}]` {
		t.Fatalf("archivedPrerequisites %s", ojson.Compact(ap))
	}
	off := *e
	off.noGraph = true
	opv, _ := off.CompactPreview(parseT(t, `{"id":"dw","archiveReason":"tidy","completedPhaseIds":["a"]}`))
	if got := string(ojson.Pretty(dropMembers(pv, "archivedPrerequisites"))); got != string(ojson.Pretty(opv)) {
		t.Fatalf("preview differs beyond archivedPrerequisites\n%s", firstDiff(got, string(ojson.Pretty(opv))))
	}
}

func parseT(t *testing.T, s string) ojson.Value {
	t.Helper()
	p, err := ojson.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return p.Value
}

// TestD2CriticalPath is G6/X6 on the roadmap graph: inspect and doctor
// show the same advisory path; slack is zero on it.
func TestD2CriticalPath(t *testing.T) {
	e, root := seedGraphRoadmap(t)
	before := testutil.Fingerprint(t, root.Path)
	limit := 500
	ins, err := e.Inspect(InspectInput{ID: "roadmap", Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	cp, _ := ins.Get("criticalPath")
	var path []string
	ps, _ := cp.Get("steps")
	for _, s := range ps.Elems() {
		path = append(path, strMember(s, "stepId"))
	}
	if strings.Join(path, ",") != "s7,s10,s13,s16,s19,s25,s28,s31,s34,s37,s38" || numMember(cp, "length") != 11 {
		t.Fatalf("critical path %v", path)
	}
	if _, ok := cp.Get("estimate"); ok {
		t.Fatal("no estimates: no estimate member")
	}
	id := "roadmap"
	d, _ := e.Doctor(DoctorInput{ID: &id})
	pl, _ := d.Get("plans")
	dcp, _ := pl.Elems()[0].Get("criticalPath")
	if string(ojson.Compact(dcp)) != string(ojson.Compact(cp)) {
		t.Fatal("doctor and inspect disagree")
	}
	steps, _ := ins.Get("steps")
	slack := map[string]string{}
	for _, st := range steps.Elems() {
		if x, ok := st.Get("slack"); ok {
			slack[strMember(st, "id")] = x.NumberLiteral()
		}
	}
	for id, want := range map[string]string{"s7": "0", "s22": "0", "s32": "4", "s9": "10", "s8": "1"} {
		if slack[id] != want {
			t.Fatalf("slack %s = %s want %s", id, slack[id], want)
		}
	}
	// No sidecar: no graph members anywhere.
	plain := testutil.NewRoot(t, "minimal-valid")
	pe := mustEngine(t, plain.Path)
	pi, _ := pe.Inspect(InspectInput{ID: "minimal"})
	if _, ok := pi.Get("criticalPath"); ok {
		t.Fatal("criticalPath without a sidecar")
	}
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("reads wrote: %v", d)
	}
}
