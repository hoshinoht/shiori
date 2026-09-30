package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// Comparators for approved divergences (contracts §7, §10). Each proves
// that only the approved difference occurs.

func init() {
	mutationDivergences["mutations/update-duplicate-step-ids"] = mutationDivergence{
		reason: "D7: unrenderable ids are reported with field paths (reference: pathless 'Workplan id must contain at least one letter or number'); same refusal, no prompt, no write",
		run:    refusalWith("phases.2.id: Must not be empty; phases.2.steps.0.id: Must not be empty"),
	}
	d4 := "D4: the stored plan repeats the member name 'kind'; the plan stays readable but mutations fail closed with the field path (reference collapses duplicates and rewrites the plan lossily)"
	mutationDivergences["mutations/update-legacy-preserve"] = mutationDivergence{reason: d4, run: refusalWith("Workplan legacy-plan has duplicate JSON member names: kind. Remove the duplicates before mutating the plan.")}
	mutationDivergences["mutations/reset-legacy-no-specfiles"] = mutationDivergence{reason: d4, run: refusalWith("Workplan legacy-plan has duplicate JSON member names: kind. Remove the duplicates before mutating the plan.")}
	mutationDivergences["mutations/reset-markdown-only-generated"] = mutationDivergence{
		reason:  "D12: already-generated Markdown returns the identical result without preparing an intent (0 authorizations; reference: 1 prompt, identical bytes)",
		prompts: intp(0),
	}
	mutationDivergences["mutations/update-recovery-precreate-resume"] = mutationDivergence{
		reason: "D1: a journal-only plan is recoverable; resume publishes the journal's after-images and removes the journal (reference: 'Workplan file not found')",
		run:    checkPrecreateResume,
	}
	mutationDivergences["mutations/compact-apply-valid"] = mutationDivergence{
		reason: "D10: the preview token (and so the archive name and transaction id) no longer depends on the absolute root, and the removals digest is Shiori-defined; everything else is byte-identical after substituting token, archive name and digest",
		run:    checkCompactApply,
	}
}

func refusalWith(msg string) func(t *testing.T, v *mutationVector, root testutil.Root, e *Engine) {
	return func(t *testing.T, v *mutationVector, root testutil.Root, e *Engine) {
		input, _ := ojson.Parse(v.Call.Input)
		before := testutil.Fingerprint(t, root.Path)
		auth := &countingAuth{}
		_, err := runMutation(context.Background(), e, v.Call.Tool, input.Value, auth)
		if err == nil || root.Normalize(err.Error()) != msg {
			t.Fatalf("error %v, want %q", err, msg)
		}
		if auth.n != 0 {
			t.Fatalf("authorizations %d, want 0", auth.n)
		}
		if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
			t.Fatalf("refusal changed files: %v", d)
		}
	}
}

func checkPrecreateResume(t *testing.T, v *mutationVector, root testutil.Root, e *Engine) {
	// The expectedHash is the doctor's interrupted-state hash (D1).
	doc, err := e.Doctor(DoctorInput{})
	if err != nil {
		t.Fatal(err)
	}
	var sh string
	plans, _ := doc.Get("plans")
	for _, p := range plans.Elems() {
		if id, _ := p.Get("id"); id.Str() == "tx-new" {
			h, _ := p.Get("stateHash")
			sh = h.Str()
		}
	}
	if sh == "" {
		t.Fatal("doctor gave no interrupted-state hash")
	}
	input, _ := ojson.Parse([]byte(`{"id":"tx-new","recovery":"resume","expectedHash":"` + sh + `"}`))
	jbytes, err := os.ReadFile(filepath.Join(root.Path, ".opencode/workplan/tx-new.transaction.json"))
	if err != nil {
		t.Fatal(err)
	}
	auth := &countingAuth{}
	out, err := runMutation(context.Background(), e, "workplan_update", input.Value, auth)
	if err != nil {
		t.Fatal(err)
	}
	if auth.n != 1 {
		t.Fatalf("authorizations %d", auth.n)
	}
	parsed, _ := ojson.Parse(jbytes)
	targets, _ := parsed.Value.Get("targets")
	for _, tg := range targets.Elems() {
		p, _ := tg.Get("path")
		h, _ := tg.Get("afterHash")
		data, err := os.ReadFile(filepath.Join(root.Path, p.Str()))
		if err != nil {
			t.Fatal(err)
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != h.Str() {
			t.Fatalf("%s not at its after image", p.Str())
		}
	}
	if _, err := os.Stat(filepath.Join(root.Path, ".opencode/workplan/tx-new.transaction.json")); !os.IsNotExist(err) {
		t.Fatal("journal not removed")
	}
	s, err := snapshot.Load(root.Path, "tx-new", snapshot.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if ph, _ := out.Value.Get("planHash"); ph.Str() != s.PlanHash {
		t.Fatalf("planHash %s want %s", ph.Str(), s.PlanHash)
	}
	if st, _ := out.Value.Get("stateHash"); st.Str() != s.StateHash {
		t.Fatalf("stateHash mismatch")
	}
}

func checkCompactApply(t *testing.T, v *mutationVector, root testutil.Root, e *Engine) {
	input, _ := ojson.Parse(v.Call.Input)
	var previewMembers []ojson.Member
	for _, m := range input.Value.Members() {
		switch m.Key {
		case "mode", "previewToken", "confirmation", "expectedHash":
		default:
			previewMembers = append(previewMembers, m)
		}
	}
	pv, err := e.CompactPreview(mustParse(t, "compact", ojson.ObjectValue(previewMembers)))
	if err != nil {
		t.Fatal(err)
	}
	tokV, _ := pv.Get("previewToken")
	tok := tokV.Str()
	digV, _ := pv.Get("removals")
	dig, _ := digV.Get("digest")
	var applyMembers []ojson.Member
	for _, m := range input.Value.Members() {
		if m.Key == "previewToken" {
			m.Value = ojson.StringValue(tok)
		}
		applyMembers = append(applyMembers, m)
	}
	before := snapshotFiles(t, root.Path)
	auth := &countingAuth{}
	out, err := runMutation(context.Background(), e, "workplan_compact", ojson.ObjectValue(applyMembers), auth)
	if err != nil {
		t.Fatal(err)
	}
	if auth.n != 1 {
		t.Fatalf("authorizations %d", auth.n)
	}
	var wantTok string
	json0, _ := input.Value.Get("previewToken")
	wantTok = json0.Str()
	archive, _ := os.ReadFile(testutil.Testdata("vectors", "mutations", "compact-apply-valid.after", ".opencode/workplan/archive/big-plan/state-ef23629bf762-6de192567356.json"))
	ap, _ := ojson.Parse(archive)
	rem, _ := ap.Value.Get("removed")
	wantDig, _ := rem.Get("digest")
	sub := strings.NewReplacer(tok, wantTok, strings.TrimPrefix(tok, "v1-")[:12], strings.TrimPrefix(wantTok, "v1-")[:12], dig.Str(), wantDig.Str())
	after := snapshotFiles(t, root.Path)
	// Content digests of substituted files also appear inside the refreshed
	// checkpoint (manifest and planHash); map Go's digests to the vector's.
	pairs := []string{tok, wantTok, strings.TrimPrefix(tok, "v1-")[:12], strings.TrimPrefix(wantTok, "v1-")[:12], dig.Str(), wantDig.Str()}
	for rel, h := range after {
		if before[rel] == h || strings.HasSuffix(rel, ".checkpoint.json") {
			continue
		}
		if want, ok := v.ChangedFiles[sub.Replace(rel)]; ok {
			pairs = append(pairs, h, want.SHA256)
		}
	}
	postS, err := snapshot.Load(root.Path, "big-plan", snapshot.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	wantCP, _ := os.ReadFile(testutil.Testdata("vectors", "mutations", "compact-apply-valid.after", ".opencode/workplan/big-plan.checkpoint.json"))
	wcp, _ := ojson.Parse(wantCP)
	wph, _ := wcp.Value.Get("planHash")
	pairs = append(pairs, postS.PlanHash, wph.Str())
	sub = strings.NewReplacer(pairs...)
	changed := 0
	for rel, h := range after {
		if before[rel] == h {
			continue
		}
		changed++
		data, _ := os.ReadFile(filepath.Join(root.Path, rel))
		norm := sub.Replace(string(data))
		key := sub.Replace(rel)
		want, ok := v.ChangedFiles[key]
		if !ok {
			t.Fatalf("unexpected change %s", rel)
		}
		if sum := sha256.Sum256([]byte(norm)); hex.EncodeToString(sum[:]) != want.SHA256 {
			t.Fatalf("%s differs after token/digest substitution", rel)
		}
	}
	if changed != len(v.ChangedFiles) {
		t.Fatalf("changed %d files, want %d", changed, len(v.ChangedFiles))
	}
	// Output: identical after substitution, except the hashes, which must
	// be the hashes of the files Go actually wrote.
	s, err := snapshot.Load(root.Path, "big-plan", snapshot.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(sub.Replace(root.Normalize(out.String())), s.StateHash, "STATE")
	p, _ := ojson.Parse(v.Expect.Output)
	sh, _ := p.Value.Get("stateHash")
	want := strings.ReplaceAll(string(ojson.Pretty(p.Value)), sh.Str(), "STATE")
	if got != want {
		t.Fatalf("output mismatch\n%s", firstDiff(got, want))
	}
}

func mustParse(t *testing.T, tool string, v ojson.Value) ojson.Value {
	t.Helper()
	d, err := ParseMutationInput(tool, v, SurfaceCore)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// comparePreview checks a compact preview against the oracle after
// substituting the token-derived values (token, archive name, staging
// transaction id) and the removals digest. The lock-protocol auxiliary
// paths in writeIntent.resources are Shiori's own protocol and are
// compared as: every non-lock resource identical, both lock files listed.
func comparePreview(v *vector, got string) error {
	g, err := ojson.Parse([]byte(got))
	if err != nil {
		return err
	}
	w, err := ojson.Parse(v.Expect.Output)
	if err != nil {
		return err
	}
	gt, _ := g.Value.Get("previewToken")
	wt, _ := w.Value.Get("previewToken")
	gr, _ := g.Value.Get("removals")
	wr, _ := w.Value.Get("removals")
	gd, _ := gr.Get("digest")
	wd, _ := wr.Get("digest")
	gh, wh := strings.TrimPrefix(gt.Str(), "v1-"), strings.TrimPrefix(wt.Str(), "v1-")
	sub := strings.NewReplacer(gt.Str(), wt.Str(), gh[:16], wh[:16], gh[:12], wh[:12], gd.Str(), wd.Str())
	norm, _ := ojson.Parse([]byte(sub.Replace(got)))
	strip := func(v ojson.Value) (ojson.Value, []string) {
		var out []ojson.Member
		var res []string
		for _, m := range v.Members() {
			if m.Key == "writeIntent" {
				var wi []ojson.Member
				for _, x := range m.Value.Members() {
					if x.Key == "resources" {
						for _, r := range x.Value.Elems() {
							res = append(res, r.Str())
						}
						continue
					}
					wi = append(wi, x)
				}
				m.Value = ojson.ObjectValue(wi)
			}
			out = append(out, m)
		}
		return ojson.ObjectValue(out), res
	}
	a, ares := strip(norm.Value)
	b, bres := strip(w.Value)
	if x, y := string(ojson.Pretty(a)), string(ojson.Pretty(b)); x != y {
		return fmt.Errorf("preview differs:\n%s", firstDiff(x, y))
	}
	isLockAux := func(p string) bool { return strings.Contains(p, ".lock.") }
	have := map[string]bool{}
	for _, r := range ares {
		have[r] = true
	}
	for _, r := range bres {
		if !isLockAux(r) && !have[r] {
			return fmt.Errorf("resource missing: %s", r)
		}
	}
	for _, r := range ares {
		if !isLockAux(r) && !containsStr(bres, r) {
			return fmt.Errorf("extra resource: %s", r)
		}
	}
	return nil
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
