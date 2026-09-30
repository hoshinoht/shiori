package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

var readOnlyTools = map[string]bool{
	"workplan_read": true, "workplan_list": true, "workplan_inspect": true,
	"workplan_validate": true, "workplan_resume": true, "workplan_doctor": true,
}

// parseForSurface validates input and re-renders the accepted data in the
// reference's key order (schema order, workspaceRoot first on core).
func parseForSurface(tool string, v ojson.Value, s Surface) (map[string]any, error) {
	out := map[string]any{}
	put := func(k string, p any) {
		switch x := p.(type) {
		case *string:
			if x != nil {
				out[k] = *x
			}
		case *int:
			if x != nil {
				out[k] = float64(*x)
			}
		case *bool:
			if x != nil {
				out[k] = *x
			}
		case string:
			out[k] = x
		}
	}
	switch tool {
	case "workplan_read":
		in, err := ParseReadInput(v, s)
		if err != nil {
			return nil, err
		}
		put("workspaceRoot", in.WorkspaceRoot)
		put("id", in.ID)
		put("phaseId", in.PhaseID)
		put("stepId", in.StepID)
		put("includeMarkdown", in.IncludeMarkdown)
	case "workplan_list":
		in, err := ParseListInput(v, s)
		if err != nil {
			return nil, err
		}
		put("workspaceRoot", in.WorkspaceRoot)
	case "workplan_inspect":
		in, err := ParseInspectInput(v, s)
		if err != nil {
			return nil, err
		}
		put("workspaceRoot", in.WorkspaceRoot)
		put("id", in.ID)
		put("phaseId", in.PhaseID)
		put("limit", in.Limit)
		put("cursor", in.Cursor)
	case "workplan_validate":
		in, err := ParseValidateInput(v, s)
		if err != nil {
			return nil, err
		}
		put("workspaceRoot", in.WorkspaceRoot)
		put("id", in.ID)
	case "workplan_resume":
		in, err := ParseResumeInput(v, s)
		if err != nil {
			return nil, err
		}
		put("workspaceRoot", in.WorkspaceRoot)
		put("id", in.ID)
		put("maxChars", in.MaxChars)
		put("limit", in.Limit)
		put("cursor", in.Cursor)
		put("phaseId", in.PhaseID)
		put("stepId", in.StepID)
	case "workplan_doctor":
		in, err := ParseDoctorInput(v, s)
		if err != nil {
			return nil, err
		}
		put("workspaceRoot", in.WorkspaceRoot)
		put("id", in.ID)
		put("limit", in.Limit)
	}
	return out, nil
}

// TestInputVectors checks accept/reject and exact messages for the
// read-only tools on both surfaces. Mutating tools' input vectors belong
// to stage C (prepared intents) and are counted as deferred.
func TestInputVectors(t *testing.T) {
	files, _ := filepath.Glob(testutil.Testdata("vectors", "validation", "input", "*.json"))
	if len(files) != 55 {
		t.Fatalf("expected 55 input vectors, got %d", len(files))
	}
	ran, deferred := 0, 0
	for _, f := range files {
		var v struct {
			ID     string          `json:"id"`
			Tool   string          `json:"tool"`
			Input  json.RawMessage `json:"input"`
			Expect map[string]*struct {
				OK      bool           `json:"ok"`
				Data    map[string]any `json:"data"`
				Message string         `json:"message"`
			} `json:"expect"`
		}
		data, _ := os.ReadFile(f)
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		if !readOnlyTools[v.Tool] {
			deferred++
			continue
		}
		ran++
		t.Run(v.ID, func(t *testing.T) {
			parsed, err := ojson.Parse(v.Input)
			if err != nil {
				t.Fatal(err)
			}
			for _, sf := range []struct {
				name string
				s    Surface
			}{{"core", SurfaceCore}, {"native", SurfaceNative}} {
				exp := v.Expect[sf.name]
				got, err := parseForSurface(v.Tool, parsed.Value, sf.s)
				if !exp.OK {
					if err == nil || err.Error() != exp.Message {
						t.Fatalf("%s: error %v, want %q", sf.name, err, exp.Message)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s: unexpected error %v", sf.name, err)
				}
				if !reflect.DeepEqual(got, exp.Data) {
					t.Fatalf("%s: data %v want %v", sf.name, got, exp.Data)
				}
			}
		})
	}
	t.Logf("input vectors: %d read-only checked, %d mutating deferred to stage C", ran, deferred)
	if ran != 16 {
		t.Fatalf("expected 16 read-only input vectors, ran %d", ran)
	}
}

// TestGeneratedClassification checks markdown/generated-classification:
// stored Markdown is "generated" iff it byte-equals the rendering of the
// normalized stored JSON.
func TestGeneratedClassification(t *testing.T) {
	var doc struct {
		Rows []struct {
			Fixture   string `json:"fixture"`
			PlanID    string `json:"planId"`
			Present   *bool  `json:"markdownPresent"`
			Generated *bool  `json:"generated"`
			Error     string `json:"error"`
		} `json:"rows"`
	}
	testutil.ReadJSON(t, testutil.Testdata("vectors", "markdown", "generated-classification.json"), &doc)
	for _, row := range doc.Rows {
		t.Run(row.Fixture+"/"+row.PlanID, func(t *testing.T) {
			root := testutil.NewRoot(t, row.Fixture)
			e, err := New(root.Path)
			if err != nil {
				t.Fatal(err)
			}
			id, err := normalizeRequested(row.PlanID)
			if err != nil {
				t.Fatal(err)
			}
			gen, present, err := e.MarkdownGenerated(id)
			if row.Error != "" {
				if err == nil {
					t.Fatalf("expected error %q", row.Error)
				}
				got, want := normEngineText(root.Normalize(err.Error())), normEngineText(row.Error)
				if got != want {
					t.Fatalf("error %q want %q", got, want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if present != *row.Present || (row.Generated != nil && gen != *row.Generated) {
				t.Fatalf("present=%v generated=%v want %v/%v", present, gen, *row.Present, row.Generated)
			}
		})
	}
}

// TestMutationInputVectors checks the 39 mutating-tool input vectors on
// both surfaces: accept/reject, exact messages, and the accepted data in
// the reference's key order. The only divergence is D2 (unknown nested
// keys reject), checked by its own comparator.
func TestMutationInputVectors(t *testing.T) {
	files, _ := filepath.Glob(testutil.Testdata("vectors", "validation", "input", "*.json"))
	ran, diverged := 0, 0
	for _, f := range files {
		data, _ := os.ReadFile(f)
		parsed, err := ojson.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		vec := parsed.Value
		tool, _ := vec.Get("tool")
		if readOnlyTools[tool.Str()] {
			continue
		}
		name := strings.TrimPrefix(tool.Str(), "workplan_")
		id, _ := vec.Get("id")
		input, _ := vec.Get("input")
		expect, _ := vec.Get("expect")
		ran++
		t.Run(id.Str(), func(t *testing.T) {
			for _, sf := range []struct {
				name string
				s    Surface
			}{{"core", SurfaceCore}, {"native", SurfaceNative}} {
				exp, _ := expect.Get(sf.name)
				if exp.Kind() == ojson.Null || exp.IsUndefined() {
					continue
				}
				got, err := ParseMutationInput(name, input, sf.s)
				if id.Str() == "validation/input/create--unknown-nested-phase-key" {
					// D2 (approved): nested unknown keys reject with their path.
					want := `Invalid create input: phases.0.steps.0: Unrecognized key: "extra"; phases.0: Unrecognized key: "bogus"`
					if err == nil || err.Error() != want {
						t.Fatalf("%s: D2 error %v, want %q", sf.name, err, want)
					}
					continue
				}
				ok, _ := exp.Get("ok")
				if id.Str() == "validation/input/reset--bad-mode" {
					// D.3 (contracts §13 item 1): the enum lists the new
					// "wipe" mode; the message is otherwise the oracle's.
					msg, _ := exp.Get("message")
					want := strings.Replace(msg.Str(), `"markdown-only"`, `"markdown-only"|"wipe"`, 1)
					if want == msg.Str() || err == nil || err.Error() != want {
						t.Fatalf("%s: D.3 error %v, want %q", sf.name, err, want)
					}
					continue
				}
				if !ok.Bool() {
					msg, _ := exp.Get("message")
					if err == nil || err.Error() != msg.Str() {
						t.Fatalf("%s: error %v, want %q", sf.name, err, msg.Str())
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s: unexpected error %v", sf.name, err)
				}
				want, _ := exp.Get("data")
				if g, w := string(ojson.Compact(got)), string(ojson.Compact(want)); g != w {
					t.Fatalf("%s: data\n got %s\nwant %s", sf.name, g, w)
				}
			}
		})
		if id.Str() == "validation/input/create--unknown-nested-phase-key" || id.Str() == "validation/input/reset--bad-mode" {
			diverged++
		}
	}
	if ran != 39 || diverged != 2 {
		t.Fatalf("ran %d mutating input vectors (want 39), %d divergences (want 2: D2, D.3)", ran, diverged)
	}
}
