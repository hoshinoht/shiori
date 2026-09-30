package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Tests for the D.4.3 approved design changes (contracts §18). Oracle
// vector changes are covered by d4_3_vectors_test.go.

const d43Plan = "full-plan" // full-valid: a fresh checkpoint with every list set

var d43AllWithheld = []string{"summary", "nextAction", "guardrails", "references", "recentValidation"}

// nativeMutate runs one tool call on the native surface (the adapter's).
func nativeMutate(t *testing.T, e *Engine, tool, input string) (Output, error, int) {
	t.Helper()
	name := strings.TrimPrefix(tool, "workplan_")
	auth := &countingAuth{}
	data, err := ParseMutationInput(name, parseT(t, input), SurfaceNative)
	if err != nil {
		return Output{}, err, 0
	}
	p, err := e.Prepare(name, data)
	if err != nil {
		return Output{}, err, 0
	}
	out, err := e.Execute(context.Background(), p, auth, ExecOptions{})
	return out, err, auth.n
}

func d43Resume(t *testing.T, e *Engine, id string) (ojson.Value, string) {
	t.Helper()
	v, text, err := e.Resume(ResumeInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return v, text
}

func d43Stale(t *testing.T, root testutil.Root) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root.Path, ".opencode/workplan/full-plan.md"), []byte("# edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func d43Checkpoint(t *testing.T, root string) map[string]any {
	t.Helper()
	p, err := ojson.Parse(mustRead(t, filepath.Join(root, ".opencode/workplan/full-plan.checkpoint.json")))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, m := range p.Value.Members() {
		switch m.Value.Kind() {
		case ojson.String:
			out[m.Key] = m.Value.Str()
		case ojson.Array:
			out[m.Key] = strs(m.Value)
		default:
			out[m.Key] = string(ojson.Compact(m.Value))
		}
	}
	return out
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestD43ResumeWithheld is item 1: a stale or legacy checkpoint's withheld
// fields are named, the instruction points at the checkpoint file and
// merge=true, and the null/empty values stay for compatibility. Fresh,
// missing and invalid checkpoints are unchanged.
func TestD43ResumeWithheld(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	v, _ := d43Resume(t, e, d43Plan)
	if cp, _ := v.Get("checkpoint"); has3(cp, "withheld") || strMember(v, "instruction") != instructionFresh {
		t.Fatalf("fresh checkpoint: %s / %q", ojson.Compact(cp), strMember(v, "instruction"))
	}
	d43Stale(t, root)
	v, text := d43Resume(t, e, d43Plan)
	cp, _ := v.Get("checkpoint")
	w, _ := cp.Get("withheld")
	if !reflect.DeepEqual(strs(w), d43AllWithheld) {
		t.Fatalf("withheld %v", strs(w))
	}
	// Compatibility: the D.1 shapes stay (null prose, empty lists, totals).
	for _, k := range []string{"summary", "nextAction"} {
		if x, _ := cp.Get(k); x.Kind() != ojson.Null {
			t.Fatalf("%s %s", k, ojson.Compact(x))
		}
	}
	for _, k := range []string{"guardrails", "references", "recentValidation"} {
		if x, _ := cp.Get(k); len(x.Elems()) != 0 {
			t.Fatalf("%s shown: %s", k, ojson.Compact(x))
		}
	}
	if numMember(cp, "guardrailsTotal") != 1 || numMember(cp, "referencesTotal") != 1 {
		t.Fatalf("totals changed: %s", ojson.Compact(cp))
	}
	// The member sits right after freshness.
	var keys []string
	for _, m := range cp.Members() {
		keys = append(keys, m.Key)
	}
	if keys[2] != "freshness" || keys[3] != "withheld" {
		t.Fatalf("checkpoint keys %v", keys)
	}
	ins := strMember(v, "instruction")
	if ins != instructionWithheld(".opencode/workplan/full-plan.checkpoint.json") ||
		!strings.HasPrefix(ins, instructionStale+" ") || !strings.Contains(ins, "(workplan_read omits it)") ||
		!strings.Contains(ins, "merge=true") {
		t.Fatalf("instruction %q", ins)
	}
	// workplan_read indeed does not return the checkpoint (the instruction
	// names the file instead).
	rv, err := e.Read(ReadInput{ID: d43Plan})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ojson.Compact(rv)), "Phase A done; B in progress") {
		t.Fatal("workplan_read returns the checkpoint summary; update the instruction")
	}
	// The D.4.3-off packet is the same packet without withheld and with
	// the D.1 instruction.
	off := *e
	off.noD43 = true
	_, offText := d43Resume(t, &off, d43Plan)
	if err := d43CompareResumeText(v, text, offText); err != nil {
		t.Fatal(err)
	}

	// Legacy v1: named too.
	root2 := testutil.NewRoot(t, "checkpoint-legacy-v1")
	v2, _ := d43Resume(t, d31Engine(t, root2.Path), "cp-v1")
	cp2, _ := v2.Get("checkpoint")
	if w2, _ := cp2.Get("withheld"); strMember(cp2, "freshness") != FreshnessLegacy || !reflect.DeepEqual(strs(w2), []string{"summary", "nextAction", "guardrails"}) {
		t.Fatalf("legacy %s", ojson.Compact(cp2))
	}
	// Missing and invalid checkpoints withhold nothing: no member, D.1 text.
	root3 := testutil.NewRoot(t, "minimal-valid")
	v3, _ := d43Resume(t, d31Engine(t, root3.Path), "minimal")
	if cp3, _ := v3.Get("checkpoint"); has3(cp3, "withheld") || strMember(v3, "instruction") != instructionStale {
		t.Fatalf("missing checkpoint %s", ojson.Compact(cp3))
	}
	root4 := testutil.NewRoot(t, "checkpoint-corrupt")
	ids, _ := filepath.Glob(filepath.Join(root4.Path, ".opencode/workplan/*.checkpoint.json"))
	id4 := strings.TrimSuffix(filepath.Base(ids[0]), ".checkpoint.json")
	v4, _ := d43Resume(t, d31Engine(t, root4.Path), id4)
	if cp4, _ := v4.Get("checkpoint"); has3(cp4, "withheld") || strMember(v4, "instruction") != instructionStale {
		t.Fatalf("invalid checkpoint %s", ojson.Compact(cp4))
	}
}

func d43CompareResumeText(v ojson.Value, text, offText string) error {
	cp, _ := v.Get("checkpoint")
	inv := objReplace(objReplace(v, "checkpoint", objWithout(cp, "withheld")), "instruction", ojson.StringValue(instructionStale))
	s := string(ojson.Pretty(inv))
	if !strings.HasPrefix(text, "{\n") {
		s = string(ojson.Compact(inv))
	}
	if s != offText {
		return fmt.Errorf("packet minus the D.4.3 change differs from the D.4.3-off packet\n%s", firstDiff(s, offText))
	}
	return nil
}

// TestD43ResumeBudget: the withheld list is a pinned safety item. Every
// packet fits maxChars, always carries the complete list, and packets
// that differ from the D.4.3-off packet by more than the member and the
// instruction are counted (the cost of the pinned item at tight budgets).
func TestD43ResumeBudget(t *testing.T) {
	e, root := seedGraphRoadmap(t)
	mustMutate(t, e, "workplan_checkpoint", fmt.Sprintf(`{"id":"roadmap","merge":true,"guardrails":[%q,%q],"references":["docs/a.md","docs/b.md"],"recentValidation":[%q]}`,
		words("Guardrail one", 200), words("Guardrail two", 200), words("go test", 150)))
	os.WriteFile(filepath.Join(root.Path, ".opencode/workplan/roadmap.md"), []byte("# edited\n"), 0o644)
	off := *e
	off.noD43 = true
	same, costly, fewerItems := 0, 0, 0
	for max := 4096; max <= 16000; max += d31Step(max) {
		for _, limit := range []int{1, 8, 20} {
			v, text, err := e.Resume(ResumeInput{ID: "roadmap", MaxChars: &max, Limit: &limit})
			if err != nil {
				t.Fatal(err)
			}
			if ojson.UTF16Len(text) > max {
				t.Fatalf("%d/%d: %d over budget", max, limit, ojson.UTF16Len(text))
			}
			cp, _ := v.Get("checkpoint")
			if w, _ := cp.Get("withheld"); !reflect.DeepEqual(strs(w), d43AllWithheld) {
				t.Fatalf("%d/%d: withheld %v", max, limit, strs(w))
			}
			ov, offText, err := off.Resume(ResumeInput{ID: "roadmap", MaxChars: &max, Limit: &limit})
			if err != nil {
				t.Fatal(err)
			}
			if numMember(v, "page", "returned") < numMember(ov, "page", "returned") {
				fewerItems++
			}
			if d43CompareResumeText(v, text, offText) == nil {
				same++
			} else {
				costly++
			}
		}
	}
	t.Logf("withheld packets at the D.4.3-off level: %d, at a lower level (pinned item cost): %d, of which with fewer page items: %d", same, costly, fewerItems)
	if same == 0 {
		t.Fatal("no packet kept the D.4.3-off level")
	}
}

// TestD43WithheldPlaceholderRefused is item 2's refusal: on both surfaces,
// before authorization, class invalid_input, nothing written.
func TestD43WithheldPlaceholderRefused(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	h := mustHash(t, e, d43Plan)
	refused := []string{"null", "  null  ", "null DEPLOYED to staging", "undefined", "undefined next", "\tnull x"}
	accepted := []string{"nullable config", "Null value", "NULL x", "null-safe", "the null case", "nullx", "undefinedness"}
	for _, field := range []string{"summary", "nextAction"} {
		for _, val := range refused {
			in := map[string]string{"summary": "Real summary", "nextAction": "Real next"}
			in[field] = val
			body := fmt.Sprintf(`{"id":%q,"summary":%q,"nextAction":%q,"expectedHash":%q}`, d43Plan, in["summary"], in["nextAction"], h)
			for _, surface := range []string{"native", "core"} {
				before := testutil.Fingerprint(t, root.Path)
				var err error
				var n int
				if surface == "native" {
					_, err, n = nativeMutate(t, e, "workplan_checkpoint", body)
				} else {
					_, err, n = mutateErr(t, e, "workplan_checkpoint", body)
				}
				var ie *InputError
				if !errors.As(err, &ie) || ErrorClass(err) != "invalid_input" || n != 0 {
					t.Fatalf("%s %s=%q: err %v class %s auths %d", surface, field, val, err, ErrorClass(err), n)
				}
				want := field + ": Checkpoint " + field + " starts with null/undefined"
				if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "merge=true") || strings.Contains(ie.Issues[0].Message, "; ") {
					t.Fatalf("message %q", err.Error())
				}
				if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
					t.Fatalf("refusal wrote %v", d)
				}
			}
		}
	}
	for _, val := range accepted {
		h = mustHash(t, e, d43Plan)
		body := fmt.Sprintf(`{"id":%q,"summary":%q,"nextAction":"n","expectedHash":%q,"merge":true}`, d43Plan, val, h)
		if _, err, _ := nativeMutate(t, e, "workplan_checkpoint", body); err != nil {
			t.Fatalf("%q refused: %v", val, err)
		}
	}
}

// TestD43DropWarnings is item 2's warnings: counts before → after for
// each list that loses stored entries; shorter prose is not a warning.
func TestD43DropWarnings(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	// Keeps every list (and a much shorter summary): no warnings member.
	out := mustMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","summary":"s","nextAction":"n","blockers":["waiting on review"],"recentValidation":["bun test: 3 passed","go test ok"],"guardrails":["do not touch adapter"],"references":["docs/spec-a.md"]}`)
	if has3(out.Value, "warnings") {
		t.Fatalf("warnings %s", ojson.Compact(out.Value))
	}
	// Drops: guardrails 1 → 0, references 1 → 1 (replaced), recentValidation 2 → 1, blockers 1 → 0.
	out = mustMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","summary":"s","nextAction":"n","recentValidation":["go test ok"],"references":["docs/other.md"]}`)
	w, _ := out.Value.Get("warnings")
	want := []string{
		"guardrails: 1 → 0 (1 previous entry not kept; merge=true keeps omitted fields)",
		"references: 1 → 1 (1 previous entry not kept; merge=true keeps omitted fields)",
		"recentValidation: 2 → 1 (1 previous entry not kept; merge=true keeps omitted fields)",
		"blockers: 1 → 0 (1 previous entry not kept; merge=true keeps omitted fields)",
	}
	if !reflect.DeepEqual(strs(w), want) {
		t.Fatalf("warnings %q", strs(w))
	}
	var keys []string
	for _, m := range out.Value.Members() {
		keys = append(keys, m.Key)
	}
	if keys[len(keys)-1] != "warnings" || keys[len(keys)-2] != "directorySync" {
		t.Fatalf("result keys %v", keys)
	}
	// No stored checkpoint: nothing to drop.
	root2 := testutil.NewRoot(t, "minimal-valid")
	out = mustMutate(t, d31Engine(t, root2.Path), "workplan_checkpoint", `{"id":"minimal","summary":"s","nextAction":"n"}`)
	if has3(out.Value, "warnings") {
		t.Fatalf("first checkpoint warned: %s", ojson.Compact(out.Value))
	}
	// Two entries dropped from a legacy checkpoint: plural and counts.
	if got := checkpointDropWarnings(&model.Checkpoint{Guardrails: []string{"a", "b", "c"}}, &model.Checkpoint{Guardrails: []string{"c"}}); len(got) != 1 ||
		got[0] != "guardrails: 3 → 1 (2 previous entries not kept; merge=true keeps omitted fields)" {
		t.Fatalf("plural %q", got)
	}
}

// TestD43Merge is item 3: merge keeps omitted fields, given fields
// replace theirs, appendValidation appends (deduplicated), the result
// binds the current plan state, and missing/legacy stored checkpoints
// behave as documented.
func TestD43Merge(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	before := d43Checkpoint(t, root.Path)
	d43Stale(t, root)
	out, err, n := nativeMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","merge":true,"appendValidation":"go test ./... passed","expectedHash":"`+mustHash(t, e, d43Plan)+`"}`)
	if err != nil || n != 1 {
		t.Fatalf("merge refresh: %v (auths %d)", err, n)
	}
	if has3(out.Value, "warnings") {
		t.Fatalf("merge refresh warned: %s", ojson.Compact(out.Value))
	}
	after := d43Checkpoint(t, root.Path)
	for _, k := range []string{"summary", "nextAction", "blockers", "guardrails", "references", "current", "createdAt"} {
		if !reflect.DeepEqual(before[k], after[k]) {
			t.Fatalf("%s: %v → %v", k, before[k], after[k])
		}
	}
	if want := []string{"bun test: 3 passed", "go test ./... passed"}; !reflect.DeepEqual(after["recentValidation"], want) {
		t.Fatalf("recentValidation %v", after["recentValidation"])
	}
	// Fresh again: bound to the current plan state.
	v, _ := d43Resume(t, e, d43Plan)
	if cp, _ := v.Get("checkpoint"); !g2(cp, "fresh").Bool() || strMember(cp, "summary") != "Phase A done; B in progress" {
		t.Fatalf("after merge %s", ojson.Compact(cp))
	}
	// Duplicates are skipped; a string array appends in order.
	mustMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","merge":true,"appendValidation":["go test ./... passed","  ","lint ok","lint ok"]}`)
	if got := d43Checkpoint(t, root.Path)["recentValidation"]; !reflect.DeepEqual(got, []string{"bun test: 3 passed", "go test ./... passed", "lint ok"}) {
		t.Fatalf("appended %v", got)
	}
	// Given fields replace theirs; an explicit empty list clears (and warns).
	out = mustMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","merge":true,"summary":"New summary","guardrails":[]}`)
	after = d43Checkpoint(t, root.Path)
	if after["summary"] != "New summary" || after["nextAction"] != before["nextAction"] || len(after["guardrails"].([]string)) != 0 ||
		!reflect.DeepEqual(after["references"], before["references"]) {
		t.Fatalf("partial merge %v", after)
	}
	if w, _ := out.Value.Get("warnings"); !reflect.DeepEqual(strs(w), []string{"guardrails: 1 → 0 (1 previous entry not kept; merge=true keeps omitted fields)"}) {
		t.Fatalf("warnings %q", strs(w))
	}
	// The stored position no longer resolves (step completed): re-derived.
	mustMutate(t, e, "workplan_update", `{"id":"full-plan","updateSteps":[{"phaseId":"phase-b","stepId":"step-b1","status":"completed"}]}`)
	mustMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","merge":true}`)
	if cur := d43Checkpoint(t, root.Path)["current"].(string); !strings.Contains(cur, `"stepId":"step-b2"`) {
		t.Fatalf("position %s", cur)
	}
	// An explicit position wins over the stored one.
	mustMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","merge":true,"phaseId":"phase-c"}`)
	if cur := d43Checkpoint(t, root.Path)["current"].(string); !strings.Contains(cur, `"stepId":"step-c1"`) {
		t.Fatalf("position %s", cur)
	}

	// Without merge the fields stay required (the reference message).
	_, err, n = nativeMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","expectedHash":"`+mustHash(t, e, d43Plan)+`"}`)
	if err == nil || n != 0 || !strings.Contains(err.Error(), "summary: Invalid input: expected string, received undefined") {
		t.Fatalf("no merge: %v", err)
	}
	for _, bad := range []string{`"merge":false`, `"merge":"yes"`} {
		_, err, _ = nativeMutate(t, e, "workplan_checkpoint", `{"id":"full-plan",`+bad+`,"expectedHash":"`+mustHash(t, e, d43Plan)+`"}`)
		if err == nil || ErrorClass(err) != "invalid_input" {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	_, err, _ = nativeMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","merge":true,"appendValidation":3,"expectedHash":"`+mustHash(t, e, d43Plan)+`"}`)
	if err == nil || !strings.Contains(err.Error(), "appendValidation: Invalid input: expected string or array, received number") {
		t.Fatalf("appendValidation type: %v", err)
	}
	_, err, _ = nativeMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","merge":true,"appendValidation":["ok",1],"expectedHash":"`+mustHash(t, e, d43Plan)+`"}`)
	if err == nil || !strings.Contains(err.Error(), "appendValidation.1: Invalid input: expected string, received number") {
		t.Fatalf("appendValidation element: %v", err)
	}
	// appendValidation without merge appends to the given list.
	mustMutate(t, e, "workplan_checkpoint", `{"id":"full-plan","summary":"s","nextAction":"n","recentValidation":["a"],"appendValidation":"b"}`)
	if got := d43Checkpoint(t, root.Path)["recentValidation"]; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("non-merge append %v", got)
	}

	// Missing checkpoint: merges as empty; summary/nextAction then needed.
	root2 := testutil.NewRoot(t, "minimal-valid")
	e2 := d31Engine(t, root2.Path)
	before2 := testutil.Fingerprint(t, root2.Path)
	_, err, n = nativeMutate(t, e2, "workplan_checkpoint", `{"id":"minimal","merge":true,"nextAction":"n","expectedHash":"`+mustHash(t, e2, "minimal")+`"}`)
	var ie *InputError
	if !errors.As(err, &ie) || n != 0 || ErrorClass(err) != "invalid_input" ||
		err.Error() != "Invalid checkpoint input: summary: merge=true keeps the stored summary, but there is no readable stored checkpoint — pass summary" {
		t.Fatalf("merge on missing: %v (auths %d)", err, n)
	}
	if d := testutil.DiffFingerprints(before2, testutil.Fingerprint(t, root2.Path)); len(d) > 0 {
		t.Fatalf("refusal wrote %v", d)
	}
	out = mustMutate(t, e2, "workplan_checkpoint", `{"id":"minimal","merge":true,"summary":"s","nextAction":"n","appendValidation":"v"}`)
	if cp, _ := out.Value.Get("checkpoint"); strMember(cp, "summary") != "s" || !reflect.DeepEqual(strs(g2(cp, "recentValidation")), []string{"v"}) {
		t.Fatalf("merge on missing %s", ojson.Compact(cp))
	}

	// Legacy v1: fields carried over into a v2 checkpoint.
	root3 := testutil.NewRoot(t, "checkpoint-legacy-v1")
	e3 := d31Engine(t, root3.Path)
	v3, _ := d43Resume(t, e3, "cp-v1")
	_ = v3
	legacy, err := ojson.Parse(mustRead(t, filepath.Join(root3.Path, ".opencode/workplan/cp-v1.checkpoint.json")))
	if err != nil {
		t.Fatal(err)
	}
	out = mustMutate(t, e3, "workplan_checkpoint", `{"id":"cp-v1","merge":true}`)
	cp3, _ := out.Value.Get("checkpoint")
	if numMember(cp3, "schemaVersion") != 2 || strMember(cp3, "summary") != strMember(legacy.Value, "summary") ||
		strMember(cp3, "nextAction") != strMember(legacy.Value, "nextAction") ||
		!reflect.DeepEqual(strs(g2(cp3, "guardrails")), strs(g2(legacy.Value, "guardrails"))) ||
		!reflect.DeepEqual(strs(g2(cp3, "blockers")), strs(g2(legacy.Value, "blockers"))) {
		t.Fatalf("legacy merge %s", ojson.Compact(cp3))
	}
	if v, _ := d43Resume(t, e3, "cp-v1"); !g2(g2(v, "checkpoint"), "fresh").Bool() {
		t.Fatal("legacy merge is not fresh")
	}
}

// TestD43IncidentSequence replays the real-use incident: a stale
// checkpoint, the resume view, a naive rebuild from that view (refused on
// "null …", warned on the drops), and the merge-mode refresh that keeps
// every field and appends one validation line.
func TestD43IncidentSequence(t *testing.T) {
	root := testutil.NewRoot(t, "full-valid")
	e := d31Engine(t, root.Path)
	stored := d43Checkpoint(t, root.Path)
	d43Stale(t, root)
	view, _ := d43Resume(t, e, d43Plan)
	cp, _ := view.Get("checkpoint")
	if w, _ := cp.Get("withheld"); !reflect.DeepEqual(strs(w), d43AllWithheld) {
		t.Fatalf("withheld %v", strs(w))
	}
	h := strMember(view, "hashes", "stateHash")
	// The naive rebuild: the view's null summary concatenated with news,
	// one validation line, empty guardrails/references.
	naive := func(summary string) string {
		return fmt.Sprintf(`{"id":"full-plan","summary":%q,"nextAction":"Deploy","phaseId":"phase-b","stepId":"step-b1","blockers":["waiting on review"],"recentValidation":["deploy ok"],"guardrails":[],"references":[],"expectedHash":%q}`, summary, h)
	}
	summaryV, _ := cp.Get("summary")
	_, err, n := nativeMutate(t, e, "workplan_checkpoint", naive(string(ojson.Compact(summaryV))+" DEPLOYED to staging"))
	if err == nil || n != 0 || ErrorClass(err) != "invalid_input" || !strings.Contains(err.Error(), "withheld field") {
		t.Fatalf("null rebuild: %v (auths %d)", err, n)
	}
	if got := d43Checkpoint(t, root.Path); !reflect.DeepEqual(got, stored) {
		t.Fatal("refused rebuild changed the checkpoint")
	}
	// The merge-mode refresh (on a copy of the same state).
	root2 := testutil.NewRoot(t, "full-valid")
	e2 := d31Engine(t, root2.Path)
	d43Stale(t, root2)
	out, err, _ := nativeMutate(t, e2, "workplan_checkpoint", `{"id":"full-plan","merge":true,"appendValidation":"deploy ok","expectedHash":"`+mustHash(t, e2, d43Plan)+`"}`)
	if err != nil || has3(out.Value, "warnings") {
		t.Fatalf("merge refresh: %v %s", err, ojson.Compact(out.Value))
	}
	got := d43Checkpoint(t, root2.Path)
	for _, k := range []string{"summary", "nextAction", "blockers", "guardrails", "references", "current"} {
		if !reflect.DeepEqual(got[k], stored[k]) {
			t.Fatalf("%s lost: %v → %v", k, stored[k], got[k])
		}
	}
	if !reflect.DeepEqual(got["recentValidation"], append(append([]string{}, stored["recentValidation"].([]string)...), "deploy ok")) {
		t.Fatalf("recentValidation %v", got["recentValidation"])
	}
	// The naive rebuild without the null prefix is written, with warnings.
	out, err, _ = nativeMutate(t, e, "workplan_checkpoint", naive("DEPLOYED to staging"))
	if err != nil {
		t.Fatal(err)
	}
	w, _ := out.Value.Get("warnings")
	want := []string{
		"guardrails: 1 → 0 (1 previous entry not kept; merge=true keeps omitted fields)",
		"references: 1 → 0 (1 previous entry not kept; merge=true keeps omitted fields)",
		"recentValidation: 1 → 1 (1 previous entry not kept; merge=true keeps omitted fields)",
	}
	if !reflect.DeepEqual(strs(w), want) {
		t.Fatalf("warnings %q", strs(w))
	}
}

func g2(v ojson.Value, key string) ojson.Value { x, _ := v.Get(key); return x }
