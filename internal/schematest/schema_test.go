// Package schematest proves that the JSON Schemas in schema/ agree with
// the corpus. The validator is a test-only dependency and
// is never linked into the shipped binary.
package schematest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/hoshinoht/shiori/internal/input"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

const idBase = "https://shiori.invalid/schema/v1/"

var uEscape = regexp.MustCompile(`\\u([0-9A-Fa-f]{4})`)

// ecmaRegexp compiles the schemas' ECMA-262 patterns with Go's RE2 after
// translating \uXXXX escapes (the only ECMA-specific syntax they use).
func ecmaRegexp(s string) (jsonschema.Regexp, error) {
	return regexp.Compile(uEscape.ReplaceAllString(s, `\x{$1}`))
}

func compiler(t *testing.T) *jsonschema.Compiler {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseRegexpEngine(ecmaRegexp)
	root := filepath.Join(testutil.RepoRoot(), "schema", "v1")
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		doc, err := jsonschema.UnmarshalJSON(f)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		return c.AddResource(idBase+filepath.ToSlash(rel), doc)
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func load(t *testing.T, path string) any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	v, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		return nil // not JSON: rejected before schema validation
	}
	return v
}

// TestSchemasCompile compiles every schema (2020-12).
func TestSchemasCompile(t *testing.T) {
	c := compiler(t)
	root := filepath.Join(testutil.RepoRoot(), "schema", "v1")
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".json") {
			rel, _ := filepath.Rel(root, p)
			if _, err := c.Compile(idBase + filepath.ToSlash(rel)); err != nil {
				t.Errorf("%s: %v", rel, err)
			}
		}
		return nil
	})
}

// TestStorageSchemasAgreeWithFixtures: valid primary plans and sidecars
// validate; the invalid-schema fixtures do not.
func TestStorageSchemasAgreeWithFixtures(t *testing.T) {
	c := compiler(t)
	schemas := map[string]*jsonschema.Schema{}
	for k, rel := range map[string]string{
		"plan": "plan-v2.schema.json", "checkpoint": "checkpoint.schema.json",
		"dependencies": "dependencies-v1.schema.json", "transaction": "transaction-journal-v1.schema.json",
	} {
		s, err := c.Compile(idBase + rel)
		if err != nil {
			t.Fatal(err)
		}
		schemas[k] = s
	}
	invalid := map[string]bool{
		"invalid-schema/not-json.json": true, "invalid-schema/not-object.json": true,
		"invalid-schema/wrong-types.json": true, "invalid-schema/wrong-version.json": true,
		"list-mixed/broken.json": true, "list-mixed/UPPER.json": true, "list-mixed/Bad Name.json": true,
		"checkpoint-corrupt/cp-bad.checkpoint.json": true, "checkpoint-corrupt/cp-bad.dependencies.json": true,
		"list-mixed/a-plan.checkpoint.json": true,
	}
	fixtures := testutil.Testdata("fixtures")
	checked := 0
	filepath.WalkDir(fixtures, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") || !strings.Contains(p, "/.opencode/workplan/") || strings.Contains(p, "/archive/") {
			return err
		}
		name := filepath.Base(p)
		if strings.HasPrefix(name, ".") {
			return nil
		}
		kind := "plan"
		for _, k := range []string{"checkpoint", "dependencies", "transaction"} {
			if strings.HasSuffix(name, "."+k+".json") {
				kind = k
			}
		}
		rel, _ := filepath.Rel(fixtures, p)
		key := strings.SplitN(rel, string(filepath.Separator), 2)[0] + "/" + name
		v := load(t, p)
		var verr error = os.ErrInvalid
		if v != nil {
			verr = schemas[kind].Validate(v)
		}
		checked++
		if invalid[key] && verr == nil {
			t.Errorf("%s: expected schema rejection", key)
		}
		if !invalid[key] && verr != nil {
			t.Errorf("%s: %v", key, verr)
		}
		return nil
	})
	if checked < 40 {
		t.Fatalf("only %d artifacts checked", checked)
	}
}

// TestToolSchemasAgreeWithNativeParser: the native tool schema accepts
// exactly the inputs the reference native parser accepted (all 55).
func TestToolSchemasAgreeWithNativeParser(t *testing.T) {
	c := compiler(t)
	files, _ := filepath.Glob(testutil.Testdata("vectors", "validation", "input", "*.json"))
	for _, f := range files {
		var v struct {
			ID     string          `json:"id"`
			Tool   string          `json:"tool"`
			Input  json.RawMessage `json:"input"`
			Expect struct {
				Native struct {
					OK bool `json:"ok"`
				} `json:"native"`
			} `json:"expect"`
		}
		data, _ := os.ReadFile(f)
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		s, err := c.Compile(idBase + "tools/" + v.Tool + ".input.schema.json")
		if err != nil {
			t.Fatal(err)
		}
		inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(v.Input)))
		if err != nil {
			t.Fatal(err)
		}
		verr := s.Validate(inst)
		if (verr == nil) != v.Expect.Native.OK {
			t.Errorf("%s: schema ok=%v, native ok=%v (%v)", v.ID, verr == nil, v.Expect.Native.OK, verr)
		}
	}
}

// TestCheckpointMergeSchemaAgreesWithParser:
// the native checkpoint schema and the Go parser accept the same merge and
// appendValidation shapes (the withheld-placeholder refusal is an
// x-shiori-rules refinement, like the other cross-field rules).
func TestCheckpointMergeSchemaAgreesWithParser(t *testing.T) {
	c := compiler(t)
	s, err := c.Compile(idBase + "tools/workplan_checkpoint.input.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	h := `"expectedHash":"` + strings.Repeat("a", 64) + `"`
	cases := map[string]bool{
		`{"id":"p","merge":true,` + h + `}`:                                          true,
		`{"id":"p","merge":true,"appendValidation":"v",` + h + `}`:                   true,
		`{"id":"p","merge":true,"appendValidation":["v","w"],` + h + `}`:             true,
		`{"id":"p","summary":"s","nextAction":"n","appendValidation":"v",` + h + `}`: true,
		`{"id":"p","summary":"s","nextAction":"n","merge":false,` + h + `}`:          true,
		`{"id":"p",` + h + `}`:                                                                   false,
		`{"id":"p","merge":false,` + h + `}`:                                                     false,
		`{"id":"p","merge":true,"summary":"s",` + h + `}`:                                        true,
		`{"id":"p","merge":"yes","summary":"s","nextAction":"n",` + h + `}`:                      false,
		`{"id":"p","merge":true,"appendValidation":3,` + h + `}`:                                 false,
		`{"id":"p","merge":true,"appendValidation":["v",3],` + h + `}`:                           false,
		`{"id":"p","merge":true}`:                                                                false,
		`{"id":"p","summary":"s","nextAction":"n",` + h + `}`:                                    true,
		`{"id":"p","summary":"s","nextAction":"n","merge":true,"appendValidation":[],` + h + `}`: true,
	}
	for in, want := range cases {
		inst, err := jsonschema.UnmarshalJSON(strings.NewReader(in))
		if err != nil {
			t.Fatal(err)
		}
		if got := s.Validate(inst) == nil; got != want {
			t.Errorf("schema %s: ok=%v want %v", in, got, want)
		}
		v, err := ojson.Parse([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := input.ParseMutationInput("checkpoint", v.Value, input.SurfaceNative); (err == nil) != want {
			t.Errorf("parser %s: ok=%v want %v (%v)", in, err == nil, want, err)
		}
	}
}
