package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// countingAuth records authorization requests (the corpus' permission
// prompts) and allows them.
type countingAuth struct{ n int }

func (c *countingAuth) Authorize(context.Context, AuthRequest) error { c.n++; return nil }

type mutationVector struct {
	ID      string `json:"id"`
	Fixture string `json:"fixture"`
	Call    struct {
		Tool  string          `json:"tool"`
		Input json.RawMessage `json:"input"`
	} `json:"call"`
	Expect struct {
		Kind         string          `json:"kind"`
		Message      string          `json:"message"`
		OutputSha256 string          `json:"outputSha256"`
		Output       json.RawMessage `json:"output"`
		OutputText   *string         `json:"outputText"`
		Metadata     json.RawMessage `json:"metadata"`
	} `json:"expect"`
	ChangedFiles map[string]struct {
		SHA256  string `json:"sha256"`
		Deleted bool   `json:"deleted"`
	} `json:"changedFiles"`
	Prompts []json.RawMessage `json:"permissionPrompts"`
}

var frozen = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func freezeClock(t testing.TB) {
	old := Clock
	Clock = func() time.Time { return frozen }
	t.Cleanup(func() { Clock = old })
}

// runMutation parses core input and prepares/executes one tool call.
func runMutation(ctx context.Context, e *Engine, tool string, input ojson.Value, auth Authorizer) (Output, error) {
	name := strings.TrimPrefix(tool, "workplan_")
	data, err := ParseMutationInput(name, input, SurfaceCore)
	if err != nil {
		return Output{}, err
	}
	p, err := e.Prepare(name, data)
	if err != nil {
		return Output{}, err
	}
	return e.Execute(ctx, p, auth, ExecOptions{})
}

// mutationDivergence is an approved difference for one mutation vector.
type mutationDivergence struct {
	reason string
	// run replaces the default comparison entirely when set.
	run func(t *testing.T, v *mutationVector, root testutil.Root, e *Engine)
	// prompts overrides the expected authorization count.
	prompts *int
}

func intp(i int) *int { return &i }

// TestMutationVectors runs every mutation vector with a frozen clock at a
// root of the generation root's length and compares the output (or error
// text), the authorization count and the exact changed-file set and bytes.
func TestMutationVectors(t *testing.T) {
	freezeClock(t)
	files, _ := filepath.Glob(testutil.Testdata("vectors", "mutations", "*.json"))
	if len(files) != 70 {
		t.Fatalf("expected 70 mutation vectors, got %d", len(files))
	}
	counts := map[string]int{}
	var notes []string
	for _, f := range files {
		var v mutationVector
		data, _ := os.ReadFile(f)
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		ok := t.Run(v.ID, func(t *testing.T) {
			root := testutil.NewRoot(t, v.Fixture)
			e, err := New(root.Path)
			if err != nil {
				t.Fatal(err)
			}
			if ids := seededIDs[v.ID]; ids != nil {
				i := 0
				IDSource = func(string) string { i++; return ids[i-1] }
				defer func() { IDSource = nil }()
			}
			if d, ok := mutationDivergences[v.ID]; ok && d.run != nil {
				d.run(t, &v, root, e)
				return
			}
			checkMutationVector(t, &v, root, e)
		})
		switch {
		case !ok:
			counts["fail"]++
		case mutationDivergences[v.ID].reason != "":
			counts["divergence"]++
			notes = append(notes, v.ID+": "+mutationDivergences[v.ID].reason)
		default:
			counts["pass"]++
		}
	}
	t.Logf("mutation vectors: pass=%d divergence=%d fail=%d", counts["pass"], counts["divergence"], counts["fail"])
	sort.Strings(notes)
	for _, n := range notes {
		t.Log(n)
	}
	d3Write(t, "mutations/")
}

func checkMutationVector(t *testing.T, v *mutationVector, root testutil.Root, e *Engine) {
	t.Helper()
	input, err := ojson.Parse(v.Call.Input)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, root.Path)
	auth := &countingAuth{}
	out, runErr := runMutation(context.Background(), e, v.Call.Tool, input.Value, auth)
	wantPrompts := len(v.Prompts)
	if d, ok := mutationDivergences[v.ID]; ok && d.prompts != nil {
		wantPrompts = *d.prompts
	}
	if auth.n != wantPrompts {
		t.Errorf("authorizations %d, want %d", auth.n, wantPrompts)
	}
	if v.Expect.Kind == "error" {
		if runErr == nil {
			t.Fatalf("expected error %q, got output %s", v.Expect.Message, out.String())
		}
		if got := root.Normalize(runErr.Error()); got != v.Expect.Message {
			t.Fatalf("error\n got %q\nwant %q", got, v.Expect.Message)
		}
	} else {
		if runErr != nil {
			t.Fatalf("unexpected error: %v", runErr)
		}
		got := root.Normalize(out.String())
		var want string
		if v.Expect.OutputText != nil {
			want = *v.Expect.OutputText
		} else {
			p, _ := ojson.Parse(v.Expect.Output)
			want = string(ojson.Pretty(p.Value))
		}
		if got != want || sha(got) != v.Expect.OutputSha256 {
			t.Fatalf("output mismatch\n%s", firstDiff(got, want))
		}
		if len(v.Expect.Metadata) > 0 {
			p, _ := ojson.Parse(v.Expect.Metadata)
			if g, w := string(ojson.Compact(out.Metadata)), string(ojson.Compact(p.Value)); g != w {
				t.Fatalf("metadata\n got %s\nwant %s", g, w)
			}
		}
	}
	after := snapshotFiles(t, root.Path)
	checkChanged(t, v, before, after)
}

// snapshotFiles maps relative path -> sha256 for every regular file.
func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for rel, st := range testutil.Fingerprint(t, root) {
		if st.Mode.IsRegular() {
			out[filepath.ToSlash(rel)] = st.SHA
		}
	}
	return out
}

func checkChanged(t *testing.T, v *mutationVector, before, after map[string]string) {
	t.Helper()
	changed := map[string]string{}
	for k, b := range before {
		if a, ok := after[k]; !ok {
			changed[k] = "deleted"
		} else if a != b {
			changed[k] = a
		}
	}
	for k, a := range after {
		if _, ok := before[k]; !ok {
			changed[k] = a
		}
	}
	var diffs []string
	for k, want := range v.ChangedFiles {
		got, ok := changed[k]
		switch {
		case !ok:
			diffs = append(diffs, "unchanged but expected change: "+k)
		case want.Deleted && got != "deleted":
			diffs = append(diffs, "expected deletion: "+k)
		case !want.Deleted && got != want.SHA256:
			diffs = append(diffs, fmt.Sprintf("bytes differ: %s", k))
		}
	}
	for k := range changed {
		if _, ok := v.ChangedFiles[k]; !ok {
			diffs = append(diffs, "unexpected change: "+k)
		}
	}
	sort.Strings(diffs)
	if len(diffs) > 0 {
		t.Fatalf("file changes: %v", diffs)
	}
}

// seededIDs replays the generated ids the oracle's seeded random source
// produced; only the id format is contractual.
var seededIDs = map[string][]string{
	"mutations/create-generated-ids": {"phase-ember-path-597056", "step-river-path-987091"},
}

var mutationDivergences = map[string]mutationDivergence{
	"mutations/create-over-precreate-journal": {
		reason:  "S06/D1: the pending pre-create journal is refused during preparation (same message), so no authorization is requested (oracle: 1 prompt, then refusal under the lock)",
		prompts: intp(0),
	},
}
