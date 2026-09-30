package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

// vector is the common corpus vector shape (contracts §8).
type vector struct {
	ID             string `json:"id"`
	Fixture        string `json:"fixture"`
	GenerationRoot string `json:"generationRoot"`
	Call           struct {
		Tool  string          `json:"tool"`
		Input json.RawMessage `json:"input"`
	} `json:"call"`
	Expect struct {
		Kind            string          `json:"kind"`
		Message         string          `json:"message"`
		RawOutputLength int             `json:"rawOutputLength"`
		OutputSha256    string          `json:"outputSha256"`
		Output          json.RawMessage `json:"output"`
		OutputText      *string         `json:"outputText"`
	} `json:"expect"`
	ReadOnly *struct {
		Unchanged bool `json:"bytesAndMtimesUnchanged"`
	} `json:"readOnly"`
}

// Outcome of one vector.
type outcome int

const (
	outPass outcome = iota
	outDivergence
	outDeferred
	outFail
)

type tally struct {
	mu     sync.Mutex
	counts map[string]map[outcome]int
	notes  []string
}

func (t *tally) add(cat string, o outcome, note string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.counts == nil {
		t.counts = map[string]map[outcome]int{}
	}
	if t.counts[cat] == nil {
		t.counts[cat] = map[outcome]int{}
	}
	t.counts[cat][o]++
	if note != "" {
		t.notes = append(t.notes, note)
	}
}

// jscDetail matches engine-specific JavaScriptCore parse text and the Go
// parser's own detail; both are replaced by one placeholder so the stable
// prefix and everything around it still compare byte-exactly (§6.2 rule 3).
var jscDetail = regexp.MustCompile(`JSON [Pp]arse error: [^"\\]*`)

func normEngineText(s string) string {
	return jscDetail.ReplaceAllString(s, "JSON parse error: <engine detail>")
}

func runTool(t *testing.T, e *Engine, tool string, input ojson.Value) (string, error) {
	t.Helper()
	pretty := func(v ojson.Value, err error) (string, error) {
		if err != nil {
			return "", err
		}
		return string(ojson.Pretty(v)), nil
	}
	switch tool {
	case "workplan_read":
		in, err := ParseReadInput(input, SurfaceCore)
		if err != nil {
			return "", err
		}
		return pretty(e.Read(in))
	case "workplan_list":
		in, err := ParseListInput(input, SurfaceCore)
		if err != nil {
			return "", err
		}
		return pretty(e.List(in))
	case "workplan_inspect":
		in, err := ParseInspectInput(input, SurfaceCore)
		if err != nil {
			return "", err
		}
		return pretty(e.Inspect(in))
	case "workplan_validate":
		in, err := ParseValidateInput(input, SurfaceCore)
		if err != nil {
			return "", err
		}
		return pretty(e.Validate(in))
	case "workplan_doctor":
		in, err := ParseDoctorInput(input, SurfaceCore)
		if err != nil {
			return "", err
		}
		return pretty(e.Doctor(in))
	case "workplan_resume":
		in, err := ParseResumeInput(input, SurfaceCore)
		if err != nil {
			return "", err
		}
		_, text, err := e.Resume(in)
		return text, err
	case "workplan_compact":
		data, err := ParseMutationInput("compact", input, SurfaceCore)
		if err != nil {
			return "", err
		}
		return pretty(e.CompactPreview(data))
	}
	return "", errDeferred
}

var errDeferred = fmt.Errorf("deferred to stage C")

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func loadVectors(t *testing.T, dirs ...string) []string {
	var files []string
	for _, d := range dirs {
		err := filepath.WalkDir(testutil.Testdata("vectors", d), func(p string, de os.DirEntry, err error) error {
			if err == nil && !de.IsDir() && strings.HasSuffix(p, ".json") {
				files = append(files, p)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(files)
	return files
}

// TestCorpusParity runs every tools/resume/paging vector against the Go
// engine at a root of the generation root's length, checks the read-only
// gate (no byte, mode or mtime change anywhere in the root), and compares
// outputs byte-exactly, except for declared divergences which are checked
// by dedicated comparators.
func TestCorpusParity(t *testing.T) {
	var tl tally
	for _, f := range loadVectors(t, "tools", "resume", "paging") {
		var v vector
		data, _ := os.ReadFile(f)
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		cat := strings.SplitN(v.ID, "/", 2)[0]
		t.Run(v.ID, func(t *testing.T) {
			recorded := false
			defer func() {
				if !recorded {
					tl.add(cat, outFail, v.ID+": FAIL")
				}
			}()
			o, note := runVector(t, &v)
			tl.add(cat, o, note)
			recorded = true
		})
	}
	names := []string{"pass", "intentional-divergence", "deferred", "fail"}
	cats := make([]string, 0, len(tl.counts))
	for c := range tl.counts {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		var parts []string
		for o, n := range names {
			parts = append(parts, fmt.Sprintf("%s=%d", n, tl.counts[c][outcome(o)]))
		}
		t.Logf("parity %-8s %s", c, strings.Join(parts, " "))
	}
	for _, n := range tl.notes {
		t.Log(n)
	}
	d1Write(t, "tools/", "resume/", "paging/")
	t.Logf("D.1 listed vectors: %s", d1Summary(d1Expectations(t)))
	d2Write(t, "tools/", "resume/", "paging/")
	t.Logf("D.2 listed vectors: %s", d1Summary(d2Expectations(t)))
	d3Write(t, "tools/", "resume/", "paging/")
	t.Logf("D.3 listed vectors: %s", d1Summary(d3Expectations(t)))
	d31Write(t, "tools/", "resume/", "paging/")
	t.Logf("D.3.1 listed vectors: %s", d1Summary(d31Expectations(t)))
}

func runVector(t *testing.T, v *vector) (outcome, string) {
	root := testutil.NewRoot(t, v.Fixture)
	if !root.SameLength() {
		t.Logf("root length differs from generation root; budget-sensitive bytes may differ")
	}
	e, err := New(root.Path)
	if err != nil {
		t.Fatal(err)
	}
	input, err := ojson.Parse(v.Call.Input)
	if err != nil {
		t.Fatal(err)
	}
	before := testutil.Fingerprint(t, root.Path)
	text, runErr := runTool(t, e, v.Call.Tool, input.Value)
	if d := testutil.DiffFingerprints(before, testutil.Fingerprint(t, root.Path)); len(d) > 0 {
		t.Fatalf("read path modified the workspace: %v", d)
	}
	if runErr == errDeferred {
		return outDeferred, v.ID + ": " + v.Call.Tool + " is a stage C operation"
	}
	if dir := os.Getenv("SHIORI_VECTOR_DUMP"); dir != "" {
		name := strings.NewReplacer("/", "__", " ", "_").Replace(v.ID)
		body := root.Normalize(text)
		if runErr != nil {
			body = "ERROR: " + root.Normalize(runErr.Error())
		}
		os.WriteFile(filepath.Join(dir, name+".out"), []byte(body), 0o644)
	}
	// D.3.1 (contracts §14): a vector whose output changes with the D.3.1
	// additions is judged in two steps, like D.2/D.3. The D.3.1-off output
	// must pass every earlier check unchanged, and the D.3.1 output must
	// differ from it only by the approved change.
	if runErr == nil {
		off31 := *e
		off31.noD31 = true
		off31Text, off31Err := runTool(t, &off31, v.Call.Tool, input.Value)
		if off31Err != nil {
			t.Fatalf("D.3.1-off run failed: %v", off31Err)
		}
		if d31Candidate(t, v, text, off31Text) {
			_, note := judgeD3(t, v, root, &off31, input.Value, off31Text, nil)
			d31note := checkD31(t, v, root, e, text, off31Text)
			if note != "" {
				d31note += " (D.3.1-off: " + note + ")"
			}
			return outDivergence, d31note
		}
	}
	return judgeD3(t, v, root, e, input.Value, text, runErr)
}

// judgeD3 is the D.3 split followed by the D.2 and earlier checks, for an
// engine that may have the D.3.1 changes turned off.
func judgeD3(t *testing.T, v *vector, root testutil.Root, e *Engine, input ojson.Value, text string, runErr error) (outcome, string) {
	t.Helper()
	// D.3 (contracts §13): a vector whose output changes with the D.3
	// read-path additions is judged in two steps, like D.2. The D.3-off
	// output must pass every earlier check (oracle, D.1/D.2 pins,
	// divergences) unchanged, and the D.3 output must differ from it only
	// by the approved change.
	if runErr == nil {
		off3 := *e
		off3.noD3 = true
		off3Text, off3Err := runTool(t, &off3, v.Call.Tool, input)
		if off3Err != nil {
			t.Fatalf("D.3-off run failed: %v", off3Err)
		}
		if d3Candidate(t, v, text, off3Text) {
			_, note := judgeGraph(t, v, root, &off3, input, off3Text, nil)
			d3note := checkD3(t, v, root, text, off3Text)
			if note != "" {
				d3note += " (D.3-off: " + note + ")"
			}
			return outDivergence, d3note
		}
	}
	return judgeGraph(t, v, root, e, input, text, runErr)
}

// judgeGraph is the D.2 split followed by the oracle/D.1/divergence
// checks, for an engine that may have the D.3 additions turned off.
func judgeGraph(t *testing.T, v *vector, root testutil.Root, e *Engine, input ojson.Value, text string, runErr error) (outcome, string) {
	t.Helper()
	// D.2 (contracts §12): a vector whose output changes with the graph
	// additions is judged in two steps. The D.2-off output must pass the
	// earlier checks (oracle, D.1 pins, divergences) unchanged, and the
	// D.2 output must differ from it only by the approved change.
	if runErr == nil {
		off := *e
		off.noGraph = true
		offText, offErr := runTool(t, &off, v.Call.Tool, input)
		if offErr != nil {
			t.Fatalf("D.2-off run failed: %v", offErr)
		}
		if d2Candidate(t, v, text, offText) {
			_, note := judgeVector(t, v, root, offText, nil)
			d2note := checkD2(t, v, root, text, offText)
			if note != "" {
				d2note += " (D.2-off: " + note + ")"
			}
			return outDivergence, d2note
		}
	}
	return judgeVector(t, v, root, text, runErr)
}

// judgeVector compares one vector output with the oracle, the D.1 pins or
// a declared divergence.
func judgeVector(t *testing.T, v *vector, root testutil.Root, text string, runErr error) (outcome, string) {
	t.Helper()
	if runErr == nil && d1Candidate(t, v, root.Normalize(text)) {
		return checkD1(t, v, root, text)
	}
	if div, ok := divergences[v.ID]; ok {
		if runErr != nil {
			t.Fatalf("divergence %s: unexpected error %v", div.reason, runErr)
		}
		if err := div.check(v, root.Normalize(text)); err != nil {
			t.Fatalf("divergence %s comparator failed: %v", div.reason, err)
		}
		return outDivergence, v.ID + ": " + div.reason
	}
	if v.Expect.Kind == "error" {
		if runErr == nil {
			t.Fatalf("expected error %q, got output", v.Expect.Message)
		}
		got := root.Normalize(runErr.Error())
		if got == v.Expect.Message {
			return outPass, ""
		}
		if normEngineText(got) == normEngineText(v.Expect.Message) && jscDetail.MatchString(v.Expect.Message) {
			return outPass, ""
		}
		t.Fatalf("error\n got %q\nwant %q", got, v.Expect.Message)
	}
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	got := root.Normalize(text)
	var want string
	if v.Expect.OutputText != nil {
		want = *v.Expect.OutputText
	} else {
		var anyv any
		if err := json.Unmarshal(v.Expect.Output, &anyv); err != nil {
			t.Fatal(err)
		}
		p, _ := ojson.Parse(v.Expect.Output)
		want = string(ojson.Pretty(p.Value))
	}
	if sha(got) == v.Expect.OutputSha256 && got == want {
		if root.SameLength() && ojson.UTF16Len(text) != v.Expect.RawOutputLength {
			t.Fatalf("raw length %d want %d", ojson.UTF16Len(text), v.Expect.RawOutputLength)
		}
		return outPass, ""
	}
	if jscDetail.MatchString(want) && normEngineText(got) == normEngineText(want) {
		// Engine detail text differs by design; with a budgeted packet the
		// lengths differ too, so the budget invariant is checked instead.
		if v.Call.Tool == "workplan_resume" {
			checkResumeInvariants(t, v, text)
		}
		return outPass, ""
	}
	t.Fatalf("output mismatch\n%s", firstDiff(got, want))
	return outFail, ""
}

func checkResumeInvariants(t *testing.T, v *vector, text string) {
	var in struct {
		MaxChars *int `json:"maxChars"`
	}
	json.Unmarshal(v.Call.Input, &in)
	max := DefaultResumeMaxChars
	if in.MaxChars != nil {
		max = *in.MaxChars
	}
	if n := ojson.UTF16Len(text); n > max {
		t.Fatalf("resume output %d exceeds budget %d", n, max)
	}
}

func firstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	lo := i - 200
	if lo < 0 {
		lo = 0
	}
	hiG, hiW := i+200, i+200
	if hiG > len(got) {
		hiG = len(got)
	}
	if hiW > len(want) {
		hiW = len(want)
	}
	return fmt.Sprintf("at byte %d (got len %d, want len %d)\n got …%s…\nwant …%s…", i, len(got), len(want), got[lo:hiG], want[lo:hiW])
}

// divergence is an approved, intentional difference from the oracle.
type divergence struct {
	reason string
	check  func(v *vector, got string) error
}

func jsonEqual(a, b any) bool { return reflect.DeepEqual(a, b) }

func decodeAny(s string) (any, error) {
	var v any
	d := json.NewDecoder(strings.NewReader(normEngineText(s)))
	d.UseNumber()
	return v, d.Decode(&v)
}

func expectedAny(v *vector) any {
	var out any
	d := json.NewDecoder(strings.NewReader(normEngineText(string(v.Expect.Output))))
	d.UseNumber()
	d.Decode(&out)
	return out
}

// legacyNumbers (D4): Go preserves unknown number spellings exactly; the
// reference re-serialized them through IEEE doubles. Mapping the three
// exact spellings back to the reference spellings must yield the vector.
func legacyNumbers(v *vector, got string) error {
	for _, r := range [][2]string{{`"bigInt": 12345678901234567890`, `"bigInt": 12345678901234567000`}, {`"float": 1.0`, `"float": 1`}} {
		if !strings.Contains(got, r[0]) {
			return fmt.Errorf("expected exact spelling %s in Go output", r[0])
		}
		got = strings.Replace(got, r[0], r[1], 1)
	}
	if sha(got) != v.Expect.OutputSha256 {
		return fmt.Errorf("output differs beyond number spelling")
	}
	return nil
}

// unorderedList (D6): same members, deterministic UTF-16 order instead of
// ICU localeCompare / readdir order.
func unorderedList(v *vector, got string) error {
	g, err := decodeAny(got)
	if err != nil {
		return err
	}
	w := expectedAny(v)
	gm, wm := g.(map[string]any), w.(map[string]any)
	for _, key := range []string{"workplans", "sidecars"} {
		if key == "workplans" {
			if _, ok := gm[key]; !ok {
				key = "plans"
			}
		}
		gl, _ := gm[key].([]any)
		wl, _ := wm[key].([]any)
		if len(gl) != len(wl) {
			return fmt.Errorf("%s length %d want %d", key, len(gl), len(wl))
		}
		ser := func(l []any) []string {
			out := make([]string, len(l))
			for i, x := range l {
				b, _ := json.Marshal(x)
				out[i] = string(b)
			}
			return out
		}
		gs, ws := ser(gl), ser(wl)
		sort.Strings(gs)
		sort.Strings(ws)
		if !reflect.DeepEqual(gs, ws) {
			return fmt.Errorf("%s members differ\n got %v\nwant %v", key, gs, ws)
		}
		// Canonical ids keep the reference's relative order.
		delete(gm, key)
		delete(wm, key)
	}
	if !jsonEqual(gm, wm) {
		return fmt.Errorf("non-list fields differ")
	}
	return nil
}

// journalOnlyDoctor (D1): Go adds a plan entry for a journal-only plan
// carrying the read-only interrupted-state hash; everything else matches.
func journalOnlyDoctor(v *vector, got string) error {
	g, err := decodeAny(got)
	if err != nil {
		return err
	}
	gm := g.(map[string]any)
	plans := gm["plans"].([]any)
	if len(plans) != 1 {
		return fmt.Errorf("want one journal-only entry, got %d", len(plans))
	}
	p := plans[0].(map[string]any)
	if p["id"] != "tx-new" || p["recoveryRequired"] != true || p["valid"] != false {
		return fmt.Errorf("unexpected entry %v", p)
	}
	if sh, _ := p["stateHash"].(string); len(sh) != 64 {
		return fmt.Errorf("missing interrupted stateHash")
	}
	gm["plans"] = []any{}
	gm["planCount"] = json.Number("0")
	gm["returnedPlans"] = json.Number("0")
	wm := expectedAny(v).(map[string]any)
	sortSidecars(gm)
	sortSidecars(wm)
	if !jsonEqual(gm, wm) {
		return fmt.Errorf("fields other than the journal-only entry differ")
	}
	return nil
}

func sortSidecars(m map[string]any) {
	l, _ := m["sidecars"].([]any)
	sort.Slice(l, func(i, j int) bool {
		return l[i].(map[string]any)["name"].(string) < l[j].(map[string]any)["name"].(string)
	})
}

// limitedDoctor (D6): with UTF-16 order the first page of a limited
// doctor call holds the first plans/sidecars in that order. Counts and
// every other field must match the reference exactly.
func limitedDoctor(v *vector, got string) error {
	g, err := decodeAny(got)
	if err != nil {
		return err
	}
	gm := g.(map[string]any)
	wm := expectedAny(v).(map[string]any)
	ids := func(l []any, key string) []string {
		out := []string{}
		for _, x := range l {
			out = append(out, x.(map[string]any)[key].(string))
		}
		return out
	}
	gotPlans := ids(gm["plans"].([]any), "id")
	if want := []string{"Bad Name", "UPPER"}; !reflect.DeepEqual(gotPlans, want) {
		return fmt.Errorf("first page %v want %v (UTF-16 order)", gotPlans, want)
	}
	gotSide := ids(gm["sidecars"].([]any), "name")
	if want := []string{".a-plan.json.0000.0.stage", ".workspace-mutation.lock"}; !reflect.DeepEqual(gotSide, want) {
		return fmt.Errorf("first sidecars %v want %v", gotSide, want)
	}
	// Both first-page plans are invalid in the Go order, so the summary
	// issue appears; the lock issue is unchanged.
	for _, k := range []string{"plans", "sidecars", "issues"} {
		delete(gm, k)
		delete(wm, k)
	}
	if !jsonEqual(gm, wm) {
		return fmt.Errorf("counts or other fields differ")
	}
	return nil
}

// backslashValidate (D5): Go diagnoses the ambiguous backslash spec path.
func backslashValidate(v *vector, got string) error {
	g, err := decodeAny(got)
	if err != nil {
		return err
	}
	gm := g.(map[string]any)
	issues := gm["issues"].([]any)
	if len(issues) != 1 || !strings.Contains(issues[0].(string), `docs/quote"back\slash.md`) {
		return fmt.Errorf("expected the D5 issue, got %v", issues)
	}
	gm["issues"] = []any{}
	gm["issueCount"] = json.Number("0")
	gm["valid"] = true
	if !jsonEqual(gm, expectedAny(v)) {
		return fmt.Errorf("fields other than the D5 issue differ")
	}
	return nil
}

var divergences = map[string]divergence{
	"tools/legacy-fields/legacy-plan--read":               {"D4 exact unknown-number spelling", legacyNumbers},
	"tools/legacy-fields/legacy-plan--read-no-markdown":   {"D4 exact unknown-number spelling", legacyNumbers},
	"tools/list-mixed/list":                               {"D6 UTF-16 order (list is compared as a set)", unorderedList},
	"tools/list-mixed/doctor":                             {"D6 UTF-16 order (plans/sidecars compared as sets)", unorderedList},
	"tools/list-mixed/doctor-limit2":                      {"D6 UTF-16 order changes which plans fill a limited page", limitedDoctor},
	"tools/list-mixed/a-10--doctor":                       {"D6 sidecars sorted by name", unorderedList},
	"tools/list-mixed/a-9--doctor":                        {"D6 sidecars sorted by name", unorderedList},
	"tools/list-mixed/a-plan--doctor":                     {"D6 sidecars sorted by name", unorderedList},
	"tools/list-mixed/b-plan--doctor":                     {"D6 sidecars sorted by name", unorderedList},
	"tools/list-mixed/broken--doctor":                     {"D6 sidecars sorted by name", unorderedList},
	"tools/list-mixed/Bad Name--doctor":                   {"D6 sidecars sorted by name", unorderedList},
	"tools/list-mixed/UPPER--doctor":                      {"D6 sidecars sorted by name", unorderedList},
	"tools/pending-journal/list":                          {"D6 sidecars sorted by name", unorderedList},
	"tools/pending-journal/doctor":                        {"D6 sidecars sorted by name", unorderedList},
	"tools/pending-journal/tx-plan--doctor":               {"D6 sidecars sorted by name", unorderedList},
	"tools/pending-journal-precreate/list":                {"D6 sidecars sorted by name", unorderedList},
	"tools/pending-journal-precreate/doctor":              {"D1 journal-only interrupted stateHash (+D6 order)", journalOnlyDoctor},
	"tools/pending-journal-precreate/tx-new--doctor":      {"D1 journal-only interrupted stateHash (+D6 order)", journalOnlyDoctor},
	"tools/unicode/unicode-plan--validate":                {"D5 ambiguous backslash spec path diagnosed", backslashValidate},
	"tools/large-paging/big-plan--compact-preview":        {"D10 root-independent preview token and Shiori removals digest", comparePreview},
	"tools/checkpoint-stale-v2/cp-stale--compact-preview": {"D10 root-independent preview token and Shiori removals digest", comparePreview},
}
