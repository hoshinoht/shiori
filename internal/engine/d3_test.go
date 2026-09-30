package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/storage"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Tests for the D.3 approved design changes (contracts §13): safety and
// robustness fixes. Oracle-vector changes are covered by
// d3_vectors_test.go.

func init() {
	// D.3 writers under fault injection at every commit stage (S04/S08):
	// the wipe (archive + plan + Markdown + two sidecar deletions), the
	// status-only draft reset that removes a checkpoint, and the repair of
	// an unreadable plan by create overwrite.
	faultScenarios = append(faultScenarios,
		faultScenario{"reset-wipe", "full-valid", "full-plan", func(t *testing.T, e *Engine) (string, string) {
			sh := stateHashFor(t, e, "full-plan")
			tok := wipeToken(t, e, "full-plan", sh, "")
			return "reset", `{"id":"full-plan","mode":"wipe","expectedHash":"` + sh + `","previewToken":"` + tok + `","confirmation":"WIPE_PLAN_CONTENT"}`
		}},
		faultScenario{"reset-draft-checkpoint", "checkpoint-stale-v2", "cp-stale", fixed("reset", `{"id":"cp-stale"}`)},
		faultScenario{"create-overwrite-unreadable", "invalid-schema", "not-json", func(t *testing.T, e *Engine) (string, string) {
			return "create", `{"id":"not-json","goal":"repaired","overwrite":true,"expectedHash":"` + stateHashFor(t, e, "not-json") + `"}`
		}},
	)
}

func mustEngineD3(t *testing.T, root string) *Engine {
	t.Helper()
	e, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mutateErr(t *testing.T, e *Engine, tool, input string) (Output, error, int) {
	t.Helper()
	auth := &countingAuth{}
	out, err := runMutation(context.Background(), e, tool, parseT(t, input), auth)
	return out, err, auth.n
}

// wipeToken runs the read-only wipe preview and returns its token.
func wipeToken(t *testing.T, e *Engine, id, hash, extra string) string {
	t.Helper()
	out, err, n := mutateErr(t, e, "workplan_reset", `{"id":"`+id+`","mode":"wipe","expectedHash":"`+hash+`"`+extra+`}`)
	if err != nil {
		t.Fatalf("wipe preview: %v", err)
	}
	if n != 0 {
		t.Fatalf("wipe preview asked for authorization")
	}
	return strMember(out.Value, "previewToken")
}

func readHashT(t *testing.T, e *Engine, id string) string {
	t.Helper()
	v, err := e.Validate(ValidateInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return strMember(v, "stateHash")
}

// seedD3 is the synthetic roadmap with statuses in progress, the D.2
// dependency graph, a fresh checkpoint, findings and notes.
func seedD3(t *testing.T) (*Engine, testutil.Root) {
	t.Helper()
	e, root := seedGraphRoadmap(t)
	mustMutate(t, e, "workplan_update", `{"id":"roadmap","status":"in_progress","updatePhases":[{"phaseId":"p2","status":"in_progress"}],
		"updateSteps":[{"phaseId":"p2","stepId":"s7","status":"in_progress"},{"phaseId":"p2","stepId":"s8","status":"review"}],
		"addReviewFindings":[{"severity":"major","title":"Open finding","status":"open"},{"severity":"note","title":"Done finding","status":"resolved"}],
		"appendNotes":["n3 kept"]}`)
	mustMutate(t, e, "workplan_checkpoint", `{"id":"roadmap","summary":"s","nextAction":"n","phaseId":"p2","stepId":"s7"}`)
	return e, root
}

func planFile(t *testing.T, root, id string) *model.Plan {
	t.Helper()
	s, err := snapshot.Load(root, id, snapshot.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	return s.Plan
}

// TestD3DraftResetKeepsStructure is item 1 (draft): statuses only.
func TestD3DraftResetKeepsStructure(t *testing.T) {
	e, root := seedD3(t)
	wp := filepath.Join(root.Path, ".opencode", "workplan")
	before := planFile(t, root.Path, "roadmap")
	depsBefore, _ := os.ReadFile(filepath.Join(wp, "roadmap.dependencies.json"))
	out := mustMutate(t, e, "workplan_reset", `{"id":"roadmap","expectedHash":"`+readHashT(t, e, "roadmap")+`"}`)
	if strMember(out.Value, "mode") != "draft" {
		t.Fatalf("mode %s", strMember(out.Value, "mode"))
	}
	if v, _ := out.Value.Get("checkpointRemoved"); !v.Bool() {
		t.Fatal("checkpointRemoved not reported")
	}
	after := planFile(t, root.Path, "roadmap")
	if after.Status != "draft" || len(after.Phases) != len(before.Phases) || after.StepCount() != before.StepCount() {
		t.Fatalf("structure changed: %d phases, %d steps", len(after.Phases), after.StepCount())
	}
	for i := range after.Phases {
		if after.Phases[i].ID != before.Phases[i].ID || after.Phases[i].Status != "draft" {
			t.Fatalf("phase %d: %s %s", i, after.Phases[i].ID, after.Phases[i].Status)
		}
		for j := range after.Phases[i].Steps {
			st, bst := after.Phases[i].Steps[j], before.Phases[i].Steps[j]
			if st.Status != "draft" || st.ID != bst.ID || *st.Action != *bst.Action || *st.Validation != *bst.Validation {
				t.Fatalf("step %s/%s not kept with status draft", after.Phases[i].ID, st.ID)
			}
		}
	}
	if strings.Join(after.Notes, "|") != strings.Join(before.Notes, "|") || len(after.Findings) != len(before.Findings) ||
		strings.Join(after.Scope, "|") != strings.Join(before.Scope, "|") || strings.Join(after.Constraints, "|") != strings.Join(before.Constraints, "|") {
		t.Fatal("notes/findings/scope/constraints not kept")
	}
	if _, err := os.Stat(filepath.Join(wp, "roadmap.checkpoint.json")); !os.IsNotExist(err) {
		t.Fatal("checkpoint not removed")
	}
	depsAfter, _ := os.ReadFile(filepath.Join(wp, "roadmap.dependencies.json"))
	if string(depsAfter) != string(depsBefore) {
		t.Fatal("dependency sidecar changed")
	}
	v, _ := e.Validate(ValidateInput{ID: "roadmap"})
	if valid, _ := v.Get("valid"); !valid.Bool() {
		t.Fatalf("plan invalid after draft reset: %s", ojson.Compact(v))
	}
	if strings.Contains(string(ojson.Compact(v)), "dependencies:") {
		t.Fatalf("dependency sidecar is not consistent: %s", ojson.Compact(v))
	}
	if strMember(v, "stateHash") == "" || !strings.Contains(string(ojson.Compact(v)), `"dependenciesRecorded":true`) {
		t.Fatal("dependencies not recorded")
	}
	// Generated Markdown follows the JSON.
	s, _ := snapshot.Load(root.Path, "roadmap", snapshot.DefaultLimits)
	if gen, _ := e.generatedMarkdown(s); !gen {
		t.Fatal("Markdown not regenerated")
	}

	// Handwritten Markdown is kept unless replaceMarkdown=true.
	md := filepath.Join(wp, "roadmap.md")
	os.WriteFile(md, []byte("# Handwritten roadmap\n"), 0o644)
	mustMutate(t, e, "workplan_update", `{"id":"roadmap","status":"in_progress"}`)
	mustMutate(t, e, "workplan_reset", `{"id":"roadmap"}`)
	if data, _ := os.ReadFile(md); string(data) != "# Handwritten roadmap\n" {
		t.Fatal("handwritten Markdown replaced without replaceMarkdown")
	}
	mustMutate(t, e, "workplan_reset", `{"id":"roadmap","replaceMarkdown":true}`)
	s, _ = snapshot.Load(root.Path, "roadmap", snapshot.DefaultLimits)
	if gen, _ := e.generatedMarkdown(s); !gen {
		t.Fatal("replaceMarkdown did not regenerate")
	}
}

// TestD3WipePreviewConfirm is item 1 (wipe): preview, exact token and
// confirmation, archive first, sidecars removed in the same transaction.
func TestD3WipePreviewConfirm(t *testing.T) {
	e, root := seedD3(t)
	wp := filepath.Join(root.Path, ".opencode", "workplan")
	orig := map[string][]byte{}
	for _, n := range []string{"roadmap.json", "roadmap.md", "roadmap.checkpoint.json", "roadmap.dependencies.json"} {
		orig[n], _ = os.ReadFile(filepath.Join(wp, n))
	}
	sh := readHashT(t, e, "roadmap")
	fp := testutil.Fingerprint(t, root.Path)
	pv, err, n := mutateErr(t, e, "workplan_reset", `{"id":"roadmap","mode":"wipe","expectedHash":"`+sh+`"}`)
	if err != nil || n != 0 {
		t.Fatalf("preview: %v (authorizations %d)", err, n)
	}
	if d := testutil.DiffFingerprints(fp, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("preview changed files: %v", d)
	}
	tok := strMember(pv.Value, "previewToken")
	if !strings.HasPrefix(tok, "v1-") || strMember(pv.Value, "confirmationRequiredForApply") != "WIPE_PLAN_CONTENT" {
		t.Fatalf("preview %s", ojson.Compact(pv.Value))
	}
	if numMember(pv.Value, "removals", "phaseCount") != 13 || numMember(pv.Value, "removals", "stepCount") != 38 || numMember(pv.Value, "removals", "noteCount") != 3 {
		t.Fatalf("removals %s", ojson.Compact(pv.Value))
	}
	dels := strs(func() ojson.Value { v, _ := pv.Value.Get("writeIntent"); d, _ := v.Get("deletePaths"); return d }())
	if len(dels) != 2 || !strings.HasSuffix(dels[0], "roadmap.checkpoint.json") || !strings.HasSuffix(dels[1], "roadmap.dependencies.json") {
		t.Fatalf("deletePaths %v", dels)
	}
	base := `{"id":"roadmap","mode":"wipe","expectedHash":"` + sh + `"`
	for _, tc := range []struct{ input, want string }{
		{base + `,"previewToken":"` + tok + `"}`, "Wipe apply requires confirmation=WIPE_PLAN_CONTENT"},
		{base + `,"previewToken":"` + tok + `","confirmation":"yes"}`, "Wipe apply requires confirmation=WIPE_PLAN_CONTENT"},
		{base + `,"confirmation":"WIPE_PLAN_CONTENT"}`, "Wipe apply requires the previewToken"},
		{base + `,"previewToken":"v1-00","confirmation":"WIPE_PLAN_CONTENT"}`, "previewToken does not match"},
		{base + `,"previewToken":"` + tok + `","confirmation":"WIPE_PLAN_CONTENT","preserveNotes":true}`, "previewToken does not match"},
		{`{"id":"roadmap","previewToken":"` + tok + `","confirmation":"WIPE_PLAN_CONTENT"}`, "apply only to mode=wipe"},
	} {
		_, err, n := mutateErr(t, e, "workplan_reset", tc.input)
		if err == nil || !strings.Contains(err.Error(), tc.want) || n != 0 {
			t.Fatalf("%s: err %v (authorizations %d), want %q", tc.input, err, n, tc.want)
		}
	}
	if d := testutil.DiffFingerprints(fp, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("refusals changed files: %v", d)
	}
	// Native surface: the same rules are input errors.
	if _, err := ParseMutationInput("reset", parseT(t, `{"id":"roadmap","expectedHash":"`+sh+`","confirmation":"WIPE_PLAN_CONTENT"}`), SurfaceNative); err == nil ||
		!strings.Contains(err.Error(), "confirmation: previewToken and confirmation apply only to mode=wipe") {
		t.Fatalf("native refine: %v", err)
	}
	out, err, n := mutateErr(t, e, "workplan_reset", base+`,"previewToken":"`+tok+`","confirmation":"WIPE_PLAN_CONTENT"}`)
	if err != nil || n != 1 {
		t.Fatalf("apply: %v (authorizations %d)", err, n)
	}
	archive := strMember(out.Value, "archivePath")
	st, err := os.Stat(archive)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("archive %s: %v %v", archive, err, st)
	}
	var arch struct {
		ArchiveVersion int    `json:"archiveVersion"`
		WorkplanID     string `json:"workplanId"`
		Operation      string `json:"operation"`
		PreviewToken   string `json:"previewToken"`
		Removed        struct {
			Phases []json.RawMessage `json:"phases"`
			Notes  []string          `json:"notes"`
		} `json:"removed"`
		Source map[string]*string `json:"source"`
	}
	data, _ := os.ReadFile(archive)
	if err := json.Unmarshal(data, &arch); err != nil {
		t.Fatal(err)
	}
	if arch.ArchiveVersion != 1 || arch.WorkplanID != "roadmap" || arch.Operation != "reset:wipe" || arch.PreviewToken != tok || len(arch.Removed.Phases) != 13 || len(arch.Removed.Notes) != 3 {
		t.Fatalf("archive header %+v", arch)
	}
	for key, name := range map[string]string{"workplanJson": "roadmap.json", "linkedMarkdown": "roadmap.md", "checkpoint": "roadmap.checkpoint.json", "dependencies": "roadmap.dependencies.json"} {
		if arch.Source[key] == nil || *arch.Source[key] != string(orig[name]) {
			t.Fatalf("archive source %s is not the complete original", key)
		}
	}
	p := planFile(t, root.Path, "roadmap")
	if len(p.Phases) != 0 || len(p.Findings) != 0 || len(p.Notes) != 0 || p.Status != "draft" || len(p.Scope) != 6 || len(p.Constraints) != 8 {
		t.Fatalf("wiped plan: %d phases %d findings %d notes %s", len(p.Phases), len(p.Findings), len(p.Notes), p.Status)
	}
	for _, n := range []string{"roadmap.checkpoint.json", "roadmap.dependencies.json"} {
		if _, err := os.Stat(filepath.Join(wp, n)); !os.IsNotExist(err) {
			t.Fatalf("%s not removed", n)
		}
	}
	v, _ := e.Validate(ValidateInput{ID: "roadmap"})
	if strings.Contains(string(ojson.Compact(v)), "dependencies:") || !strings.Contains(string(ojson.Compact(v)), `"dependenciesRecorded":false`) {
		t.Fatalf("validate after wipe: %s", ojson.Compact(v))
	}
	// The same archive stays; a second wipe of the empty plan is a new token.
	if tok2 := wipeToken(t, e, "roadmap", readHashT(t, e, "roadmap"), ""); tok2 == tok {
		t.Fatal("token did not change with the state")
	}

	// preserveNotes keeps notes; handwritten Markdown needs replaceMarkdown.
	e2, root2 := seedD3(t)
	os.WriteFile(filepath.Join(root2.Path, ".opencode/workplan/roadmap.md"), []byte("# hand\n"), 0o644)
	sh2 := readHashT(t, e2, "roadmap")
	if _, err, _ := mutateErr(t, e2, "workplan_reset", `{"id":"roadmap","mode":"wipe","expectedHash":"`+sh2+`"}`); err == nil || !strings.Contains(err.Error(), "handwritten") {
		t.Fatalf("handwritten wipe preview: %v", err)
	}
	extra := `,"preserveNotes":true,"replaceMarkdown":true`
	tok2 := wipeToken(t, e2, "roadmap", sh2, extra)
	mustMutate(t, e2, "workplan_reset", `{"id":"roadmap","mode":"wipe","expectedHash":"`+sh2+`","previewToken":"`+tok2+`","confirmation":"WIPE_PLAN_CONTENT"`+extra+`}`)
	if p := planFile(t, root2.Path, "roadmap"); len(p.Notes) != 3 || len(p.Phases) != 0 {
		t.Fatalf("preserveNotes: %d notes, %d phases", len(p.Notes), len(p.Phases))
	}
}

// TestD3UnreadablePlanRepair is item 2: a truncated plan gets raw-byte
// hashes from doctor/validate/list, and create overwrite with that hash
// repairs it (keeping the Markdown unless replaceMarkdown=true).
func TestD3UnreadablePlanRepair(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := mustEngineD3(t, root.Path)
	jp := filepath.Join(root.Path, ".opencode/workplan/minimal.json")
	md := filepath.Join(root.Path, ".opencode/workplan/minimal.md")
	data, _ := os.ReadFile(jp)
	os.WriteFile(jp, data[:len(data)/2], 0o644) // truncated
	mdBefore, _ := os.ReadFile(md)

	doc, _ := e.Doctor(DoctorInput{})
	plans, _ := doc.Get("plans")
	entry := plans.Elems()[0]
	sh := strMember(entry, "stateHash")
	ph := strMember(entry, "planHash")
	if len(sh) != 64 || len(ph) != 64 {
		t.Fatalf("doctor gives no raw hashes: %s", ojson.Compact(entry))
	}
	v, _ := e.Validate(ValidateInput{ID: "minimal"})
	if strMember(v, "stateHash") != sh || strMember(v, "planHash") != ph {
		t.Fatalf("validate hashes differ: %s", ojson.Compact(v))
	}
	l, _ := e.List(ListInput{})
	ws, _ := l.Get("workplans")
	le := ws.Elems()[0]
	if strMember(le, "stateHash") != sh || len(strs(func() ojson.Value { x, _ := le.Get("issues"); return x }())) != 1 {
		t.Fatalf("list entry: %s", ojson.Compact(le))
	}
	if _, ok := le.Get("issue"); ok {
		t.Fatal("list entry still carries the single issue string")
	}
	// A stale hash is refused; reads stayed read-only.
	if _, err, n := mutateErr(t, e, "workplan_create", `{"id":"minimal","goal":"g","overwrite":true,"expectedHash":"`+strings.Repeat("0", 64)+`"}`); err == nil || !strings.Contains(err.Error(), "current stateHash is "+sh) || n != 0 {
		t.Fatalf("stale overwrite: %v", err)
	}
	out, err, n := mutateErr(t, e, "workplan_create", `{"id":"minimal","goal":"Repaired goal","overwrite":true,"expectedHash":"`+sh+`"}`)
	if err != nil || n != 1 {
		t.Fatalf("repair: %v (authorizations %d)", err, n)
	}
	if v, _ := out.Value.Get("overwritten"); !v.Bool() {
		t.Fatal("not reported as overwritten")
	}
	if got, _ := os.ReadFile(md); string(got) != string(mdBefore) {
		t.Fatal("existing Markdown replaced without replaceMarkdown")
	}
	if p := planFile(t, root.Path, "minimal"); p.Goal != "Repaired goal" {
		t.Fatalf("goal %q", p.Goal)
	}
	// With replaceMarkdown the Markdown is regenerated.
	os.WriteFile(jp, []byte("{"), 0o644)
	sh = stateHashFor(t, e, "minimal")
	mustMutate(t, e, "workplan_create", `{"id":"minimal","goal":"Again","overwrite":true,"replaceMarkdown":true,"expectedHash":"`+sh+`"}`)
	s, _ := snapshot.Load(root.Path, "minimal", snapshot.DefaultLimits)
	if gen, _ := e.generatedMarkdown(s); !gen {
		t.Fatal("replaceMarkdown did not regenerate")
	}
}

// TestD3StaleMarkdownCopy is item 3: the old <id>.md left by a planFile
// move is reported by doctor; nothing is moved or deleted.
func TestD3StaleMarkdownCopy(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := mustEngineD3(t, root.Path)
	mustMutate(t, e, "workplan_update", `{"id":"minimal","planFile":".opencode/workplan/docs/minimal-plan.md"}`)
	os.WriteFile(filepath.Join(root.Path, ".opencode/workplan/orphan-notes.md"), []byte("# notes\n"), 0o644)
	fp := testutil.Fingerprint(t, root.Path)
	doc, _ := e.Doctor(DoctorInput{})
	if d := testutil.DiffFingerprints(fp, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("doctor changed files: %v", d)
	}
	strays, _ := doc.Get("strayArtifacts")
	kinds := map[string]string{}
	for _, s := range strays.Elems() {
		kinds[strMember(s, "name")] = strMember(s, "kind")
	}
	if kinds["minimal.md"] != "stale-markdown" || kinds["orphan-notes.md"] != "unclassified" || len(kinds) != 2 {
		t.Fatalf("strays %v", kinds)
	}
	w, _ := doc.Get("warnings")
	if !containsPrefix(strs(w), "Stale Markdown copy .opencode/workplan/minimal.md: plan minimal now links .opencode/workplan/docs/minimal-plan.md") {
		t.Fatalf("warnings %v", strs(w))
	}
}

// TestD3StepMarkers is item 4: handwritten Markdown without step markers
// warns (never fails, never rewritten).
func TestD3StepMarkers(t *testing.T) {
	e, root := seedRoadmap(t)
	md := filepath.Join(root.Path, ".opencode/workplan/roadmap.md")
	v, _ := e.Validate(ValidateInput{ID: "roadmap"})
	if strings.Contains(string(ojson.Compact(v)), d3MarkerNeedle) {
		t.Fatal("generated Markdown warned")
	}
	data, _ := os.ReadFile(md)
	hand := strings.Replace(string(data), "<!-- workplan-step-id: s7 -->", "", 1)
	hand = strings.Replace(hand, "<!-- workplan-step-id: s9 -->", "", 1)
	os.WriteFile(md, []byte(hand), 0o644)
	v, _ = e.Validate(ValidateInput{ID: "roadmap"})
	w, _ := v.Get("warnings")
	if valid, _ := v.Get("valid"); !valid.Bool() || !containsPrefix(strs(w), "has no step marker (<!-- workplan-step-id: <stepId> -->) for 2 of 38 steps: p2/s7, p2/s9.") {
		t.Fatalf("validate: %s", ojson.Compact(v))
	}
	if got, _ := os.ReadFile(md); string(got) != hand {
		t.Fatal("Markdown rewritten")
	}
	// Doctor and patch validation carry the same warning.
	doc, _ := e.Doctor(DoctorInput{})
	if !strings.Contains(string(ojson.Compact(doc)), "for 2 of 38 steps") {
		t.Fatal("doctor lacks the marker warning")
	}
	// More than ten missing are counted, not listed.
	os.WriteFile(md, []byte("# roadmap\n"), 0o644)
	v, _ = e.Validate(ValidateInput{ID: "roadmap"})
	if !strings.Contains(string(ojson.Compact(v)), "for 38 of 38 steps: p0/s1") || !strings.Contains(string(ojson.Compact(v)), " and 28 more.") {
		t.Fatalf("validate: %s", ojson.Compact(v))
	}
}

// TestD3StaleCheckpointDetail is item 5: doctor names what changed.
func TestD3StaleCheckpointDetail(t *testing.T) {
	e, root := seedRoadmap(t)
	os.MkdirAll(filepath.Join(root.Path, "docs"), 0o755)
	os.WriteFile(filepath.Join(root.Path, "docs", "spec.md"), []byte("# spec\n"), 0o644)
	mustMutate(t, e, "workplan_update", `{"id":"roadmap","addSpecFiles":["docs/spec.md"]}`)
	mustMutate(t, e, "workplan_checkpoint", `{"id":"roadmap","summary":"s","nextAction":"n"}`)
	os.WriteFile(filepath.Join(root.Path, "docs", "spec.md"), []byte("# spec v2\n"), 0o644)
	doc, _ := e.Doctor(DoctorInput{})
	plans, _ := doc.Get("plans")
	issues := strs(func() ojson.Value { x, _ := plans.Elems()[0].Get("issues"); return x }())
	if !containsPrefix(issues, "checkpoint: "+msgStale+". Changed since the checkpoint: docs/spec.md changed (checkpoint sha256 ") ||
		containsPrefix(issues, "roadmap.json changed") {
		t.Fatalf("issues %v", issues)
	}
	os.Remove(filepath.Join(root.Path, "docs", "spec.md"))
	doc, _ = e.Doctor(DoctorInput{})
	if !strings.Contains(string(ojson.Compact(doc)), "docs/spec.md changed (checkpoint sha256 ") || !strings.Contains(string(ojson.Compact(doc)), ", now missing)") {
		t.Fatalf("doctor: %s", ojson.Compact(doc))
	}
}

// TestD3GeneratedIDs is item 5: readable title slugs, a short suffix only
// on collision, generated step ids unique across the plan, existing ids
// never change.
func TestD3GeneratedIDs(t *testing.T) {
	root := testutil.NewRoot(t, "empty-workspace")
	e := mustEngineD3(t, root.Path)
	long := strings.Repeat("Very long title words ", 8)
	mustMutate(t, e, "workplan_create", `{"id":"ids","goal":"g","phases":[
		{"title":"Core bot","steps":[{"title":"Write tests"},{"title":"Write tests"},{"id":"write-tests-3","title":"x"},{"title":"日本語"}]},
		{"title":"Core bot","steps":[{"title":"Write tests"},{"title":"`+long+`"}]},
		{"id":"core-bot-2","title":"Explicit"}]}`)
	p := planFile(t, root.Path, "ids")
	var got []string
	for _, ph := range p.Phases {
		var st []string
		for _, s := range ph.Steps {
			st = append(st, s.ID)
		}
		got = append(got, ph.ID+":"+strings.Join(st, ","))
	}
	if got[0] != "core-bot:write-tests,write-tests-2,write-tests-3,"+p.Phases[0].Steps[3].ID || !strings.HasPrefix(p.Phases[0].Steps[3].ID, "step-") {
		t.Fatalf("phase 0: %v", got)
	}
	if !strings.HasPrefix(got[1], "core-bot-3:write-tests-4,very-long-title-words") || len(p.Phases[1].Steps[1].ID) > maxSlugUnits || got[2] != "core-bot-2:" {
		t.Fatalf("ids %v", got)
	}
	// addSteps/addPhases: slugs avoid every existing id; existing ids stay.
	mustMutate(t, e, "workplan_update", `{"id":"ids","addPhases":[{"phase":{"title":"Core bot"}}],"addSteps":[{"phaseId":"core-bot-2","step":{"title":"Write tests"}}]}`)
	p2 := planFile(t, root.Path, "ids")
	if p2.Phases[3].ID != "core-bot-4" || p2.Phases[2].Steps[0].ID != "write-tests-5" {
		t.Fatalf("added ids %s %s", p2.Phases[3].ID, p2.Phases[2].Steps[0].ID)
	}
	for i := range p.Phases {
		for j := range p.Phases[i].Steps {
			if p2.Phases[i].Steps[j].ID != p.Phases[i].Steps[j].ID {
				t.Fatal("existing ids changed")
			}
		}
	}
}

// TestD3SpecFilesMustExist is item 5: missing spec links are refused at
// write time, before authorization.
func TestD3SpecFilesMustExist(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := mustEngineD3(t, root.Path)
	fp := testutil.Fingerprint(t, root.Path)
	for _, tc := range []struct{ tool, input, want string }{
		{"workplan_create", `{"id":"n","goal":"g","specFiles":["docs/none.md"]}`, "specFiles.0: Linked spec file does not exist: docs/none.md"},
		{"workplan_update", `{"id":"minimal","specFiles":["docs/none.md"]}`, "specFiles.0: Linked spec file does not exist: docs/none.md"},
		{"workplan_update", `{"id":"minimal","addSpecFiles":["","docs/none.md"]}`, "addSpecFiles.1: Linked spec file does not exist: docs/none.md"},
	} {
		_, err, n := mutateErr(t, e, tc.tool, tc.input)
		if err == nil || !strings.HasPrefix(err.Error(), tc.want) || n != 0 || ErrorClass(err) != "invalid_input" {
			t.Fatalf("%s: %v (authorizations %d)", tc.input, err, n)
		}
	}
	if d := testutil.DiffFingerprints(fp, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("refusals changed files: %v", d)
	}
	os.MkdirAll(filepath.Join(root.Path, "docs"), 0o755)
	os.WriteFile(filepath.Join(root.Path, "docs", "none.md"), []byte("# now exists\n"), 0o644)
	mustMutate(t, e, "workplan_update", `{"id":"minimal","addSpecFiles":["docs/none.md"]}`)
}

// TestD3NoteLimit is item 5: 16 KiB per new note.
func TestD3NoteLimit(t *testing.T) {
	root := testutil.NewRoot(t, "minimal-valid")
	e := mustEngineD3(t, root.Path)
	ok := strings.Repeat("a", MaxNoteBytes)
	mustMutate(t, e, "workplan_update", `{"id":"minimal","appendNotes":["`+ok+`"]}`)
	_, err, n := mutateErr(t, e, "workplan_update", `{"id":"minimal","appendNotes":["short","`+ok+`b"]}`)
	if err == nil || err.Error() != "appendNotes.1: Note is 16385 bytes, above the 16384-byte (16 KiB) per-note limit. Keep notes short; put long evidence in a file and reference its path." || n != 0 || ErrorClass(err) != "invalid_input" {
		t.Fatalf("appendNotes: %v", err)
	}
	if _, err, _ := mutateErr(t, e, "workplan_create", `{"id":"n2","goal":"g","notes":["`+ok+`é"]}`); err == nil || !strings.HasPrefix(err.Error(), "notes.0: Note is 16386 bytes") {
		t.Fatalf("create notes: %v", err)
	}
	// A stored oversized note (hand-edited) does not block other writes.
	mustMutate(t, e, "workplan_update", `{"id":"minimal","title":"still writable"}`)
}

// TestD3FileModes is item 5: new plan/Markdown files take the mode of the
// existing plans (owner rw added), else 0644 minus the umask; sidecars
// and archives stay 0600.
func TestD3FileModes(t *testing.T) {
	mode := func(p string) fs.FileMode {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return st.Mode().Perm()
	}
	root := testutil.NewRoot(t, "empty-workspace")
	e := mustEngineD3(t, root.Path)
	wp := filepath.Join(root.Path, ".opencode", "workplan")
	mustMutate(t, e, "workplan_create", `{"id":"first","goal":"g"}`)
	want := fs.FileMode(0o644 &^ processUmask)
	if mode(filepath.Join(wp, "first.json")) != want || mode(filepath.Join(wp, "first.md")) != want {
		t.Fatalf("first plan modes %v %v, want %v", mode(filepath.Join(wp, "first.json")), mode(filepath.Join(wp, "first.md")), want)
	}
	os.Chmod(filepath.Join(wp, "first.json"), 0o640)
	mustMutate(t, e, "workplan_create", `{"id":"second","goal":"g","phases":[{"id":"p","title":"P","status":"completed","steps":[{"id":"s","title":"S","status":"completed"}]}]}`)
	if mode(filepath.Join(wp, "second.json")) != 0o640 || mode(filepath.Join(wp, "second.md")) != 0o640 {
		t.Fatalf("second plan modes %v", mode(filepath.Join(wp, "second.json")))
	}
	mustMutate(t, e, "workplan_checkpoint", `{"id":"second","summary":"s","nextAction":"n"}`)
	if mode(filepath.Join(wp, "second.checkpoint.json")) != 0o600 {
		t.Fatalf("checkpoint mode %v", mode(filepath.Join(wp, "second.checkpoint.json")))
	}
	// A read-only plan mode still yields owner-writable new files.
	os.Chmod(filepath.Join(wp, "first.json"), 0o444)
	os.Chmod(filepath.Join(wp, "second.json"), 0o444)
	mustMutate(t, e, "workplan_create", `{"id":"third","goal":"g"}`)
	if mode(filepath.Join(wp, "third.json")) != 0o644 {
		t.Fatalf("third plan mode %v", mode(filepath.Join(wp, "third.json")))
	}
	// Existing files keep their mode; the wipe archive is 0600.
	sh := readHashT(t, e, "third")
	tok := wipeToken(t, e, "third", sh, "")
	out := mustMutate(t, e, "workplan_reset", `{"id":"third","mode":"wipe","expectedHash":"`+sh+`","previewToken":"`+tok+`","confirmation":"WIPE_PLAN_CONTENT"}`)
	if mode(filepath.Join(wp, "third.json")) != 0o644 || mode(strMember(out.Value, "archivePath")) != 0o600 {
		t.Fatalf("modes after wipe: %v %v", mode(filepath.Join(wp, "third.json")), mode(strMember(out.Value, "archivePath")))
	}
}

// ---- item 6: real interrupted-write recovery (SIGKILL) ----

// killChild runs one mutation in a separate process and blocks at the
// named commit point after announcing it, until the parent kills it.
func killChild() int {
	root, tool, input, point, marker := os.Getenv("K_ROOT"), os.Getenv("K_TOOL"), os.Getenv("K_INPUT"), os.Getenv("K_POINT"), os.Getenv("K_MARKER")
	Clock = func() time.Time { return frozen }
	fail := func(err error) int {
		os.WriteFile(marker+".err", []byte(err.Error()), 0o644)
		return 3
	}
	e, err := New(root)
	if err != nil {
		return fail(err)
	}
	data, err := ParseMutationInput(tool, mustJSONPlain(input), SurfaceCore)
	if err != nil {
		return fail(err)
	}
	p, err := e.Prepare(tool, data)
	if err != nil {
		return fail(err)
	}
	hooks := storage.Hooks{Fault: func(pt string) error {
		if pt == point {
			os.WriteFile(marker, []byte(pt), 0o644)
			select {} // killed here
		}
		return nil
	}}
	if _, err := e.Execute(context.Background(), p, AllowAll{}, ExecOptions{Hooks: hooks}); err != nil {
		return fail(err)
	}
	return fail(errors.New("commit finished without reaching " + point))
}

// killAt runs tool/input in a child process, SIGKILLs it at point and
// returns once the child is gone.
func killAt(t *testing.T, root, tool, input, point string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "at")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "SHIORI_KILL_CHILD=1", "K_ROOT="+root, "K_TOOL="+tool, "K_INPUT="+input, "K_POINT="+point, "K_MARKER="+marker)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := false
	t.Cleanup(func() {
		if !done {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if data, err := os.ReadFile(marker + ".err"); err == nil {
			t.Fatalf("child failed before %s: %s", point, data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("child did not reach %s", point)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil { // SIGKILL
		t.Fatal(err)
	}
	err := cmd.Wait()
	done = true
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ProcessState.String() != "signal: killed" {
		t.Fatalf("child exit: %v", err)
	}
}

type journalImage struct {
	TransactionID string `json:"transactionId"`
	Targets       []struct {
		Path          string  `json:"path"`
		BeforeHash    *string `json:"beforeHash"`
		AfterHash     *string `json:"afterHash"`
		BeforeContent *string `json:"beforeContent"`
	} `json:"targets"`
}

// TestD3KillRecovery SIGKILLs a separate process at deterministic commit
// points (journal published; mid-publication) and recovers both ways
// (spec 05 S04/S08/S11, D.3 item 6).
func TestD3KillRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}
	freezeClock(t)
	wipeInput := func(t *testing.T, e *Engine) string {
		sh := readHashT(t, e, "full-plan")
		return `{"id":"full-plan","mode":"wipe","expectedHash":"` + sh + `","previewToken":"` + wipeToken(t, e, "full-plan", sh, "") + `","confirmation":"WIPE_PLAN_CONTENT"}`
	}
	cases := []struct {
		name, fixture, id, tool, point string
		input                          func(*testing.T, *Engine) string
		staged                         bool // staging files are left behind
	}{
		{"wipe-after-journal", "full-valid", "full-plan", "reset", storage.FaultJournalLink, wipeInput, true},
		{"wipe-mid-publication", "full-valid", "full-plan", "reset", storage.FaultPublish + ":2", wipeInput, true},
		{"update-mid-publication", "full-valid", "full-plan", "update", storage.FaultPublish + ":1", func(t *testing.T, e *Engine) string {
			return `{"id":"full-plan","appendNotes":["killed"],"expectedHash":"` + readHashT(t, e, "full-plan") + `"}`
		}, true},
	}
	for _, tc := range cases {
		for _, mode := range []string{"resume", "rollback"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				root := testutil.NewRoot(t, tc.fixture)
				e := mustEngineD3(t, root.Path)
				old := semanticFiles(t, root.Path)
				killAt(t, root.Path, tc.tool, tc.input(t, e), tc.point)
				jpath := filepath.Join(root.Path, ".opencode/workplan", tc.id+".transaction.json")
				data, err := os.ReadFile(jpath)
				if err != nil {
					t.Fatalf("no journal after SIGKILL at %s: %v", tc.point, err)
				}
				var j journalImage
				if err := json.Unmarshal(data, &j); err != nil {
					t.Fatal(err)
				}
				if m := machinery(t, root.Path); len(m) == 0 || (tc.staged && !containsPrefix(m, "."+j.TransactionID+".")) {
					t.Fatalf("expected the killed writer's locks/staging, got %v", m)
				}
				// Read-only diagnosis: recovery required, with a state hash.
				doc, _ := e.Doctor(DoctorInput{ID: &tc.id})
				plans, _ := doc.Get("plans")
				entry := plans.Elems()[0]
				if v, _ := entry.Get("recoveryRequired"); !v.Bool() || len(strMember(entry, "stateHash")) != 64 {
					t.Fatalf("doctor: %s", ojson.Compact(entry))
				}
				sh := strMember(entry, "stateHash")
				in := fmt.Sprintf(`{"id":%q,"recovery":%q,"expectedHash":%q}`, tc.id, mode, sh)
				data2, err := ParseMutationInput("update", parseT(t, in), SurfaceCore)
				if err != nil {
					t.Fatal(err)
				}
				// The dead owner's locks are not reclaimed within the grace
				// period (S11): recovery waits and reports the lock.
				p, err := e.Prepare("update", data2)
				if err != nil {
					t.Fatal(err)
				}
				_, err = e.Execute(context.Background(), p, AllowAll{}, ExecOptions{Hooks: storage.Hooks{Lock: storage.LockConfig{Wait: 50 * time.Millisecond}}})
				var lu *storage.LockUnavailableError
				if !errors.As(err, &lu) || !strings.Contains(err.Error(), "abandonment grace") {
					t.Fatalf("recovery inside the grace period: %v", err)
				}
				// Past the grace period the proven-dead owner is reclaimed.
				later := func() time.Time { return time.Now().Add(storage.DefaultLockGrace + time.Minute) }
				p, err = e.Prepare("update", data2)
				if err != nil {
					t.Fatal(err)
				}
				out, err := e.Execute(context.Background(), p, AllowAll{}, ExecOptions{Hooks: storage.Hooks{Lock: storage.LockConfig{Now: later}}})
				if err != nil {
					t.Fatalf("%s: %v", mode, err)
				}
				for _, tg := range j.Targets {
					want := tg.AfterHash
					if mode == "rollback" {
						want = tg.BeforeHash
					}
					got, ok := fileSHA(filepath.Join(root.Path, tg.Path))
					if want == nil && ok || want != nil && (!ok || got != *want) {
						t.Fatalf("%s: %s is not at its %s image", mode, tg.Path, mode)
					}
					if mode == "rollback" && tg.BeforeContent != nil {
						b, _ := base64.StdEncoding.DecodeString(*tg.BeforeContent)
						if cur, _ := os.ReadFile(filepath.Join(root.Path, tg.Path)); string(cur) != string(b) {
							t.Fatalf("rollback bytes of %s differ", tg.Path)
						}
					}
				}
				if mode == "rollback" {
					if got := semanticFiles(t, root.Path); !equalMaps(got, old) {
						t.Fatalf("rollback is not the old state")
					}
				}
				if _, err := os.Stat(jpath); !os.IsNotExist(err) {
					t.Fatal("journal not removed")
				}
				if m := machinery(t, root.Path); len(m) > 0 {
					t.Fatalf("stray locks/staging after recovery: %v", m)
				}
				s, err := snapshot.Load(root.Path, tc.id, snapshot.DefaultLimits)
				if err != nil {
					t.Fatal(err)
				}
				if strMember(out.Value, "stateHash") != s.StateHash {
					t.Fatal("recovery result hash differs from disk")
				}
				t.Logf("SIGKILL at %s, %s: journal %s, %d targets restored, state %s", tc.point, mode, j.TransactionID[:8], len(j.Targets), s.StateHash[:12])
			})
		}
	}
}
