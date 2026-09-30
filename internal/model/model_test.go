package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/testutil"
)

func parseRaw(t *testing.T, raw json.RawMessage) ojson.Parsed {
	t.Helper()
	p, err := ojson.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestMarkdownVectors renders every markdown vector byte-exactly.
func TestMarkdownVectors(t *testing.T) {
	files, _ := filepath.Glob(testutil.Testdata("vectors", "markdown", "*.json"))
	n := 0
	for _, f := range files {
		var v struct {
			ID       string          `json:"id"`
			Function string          `json:"function"`
			Input    json.RawMessage `json:"input"`
			Expect   struct {
				Kind    string `json:"kind"`
				File    string `json:"file"`
				SHA256  string `json:"sha256"`
				Message string `json:"message"`
			} `json:"expect"`
		}
		data, _ := os.ReadFile(f)
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		if v.Function != "renderWorkplanMarkdown" {
			continue
		}
		n++
		t.Run(v.ID, func(t *testing.T) {
			p, err := DecodePlan(parseRaw(t, v.Input))
			if err != nil {
				t.Fatal(err)
			}
			if v.ID == "markdown/error-missing-specFiles" {
				// The reference renderer throws a JavaScript TypeError on a raw
				// legacy document; Go renders only normalized documents, where
				// absent specFiles is [] (contracts §4). Declared divergence.
				if p.HasSpecFiles {
					t.Fatal("fixture should lack specFiles")
				}
				if _, err := RenderMarkdown(p); err != nil {
					t.Fatal(err)
				}
				return
			}
			// The reference rendering is RenderMarkdownLegacy; D.3.1
			// (contracts §14 item 5b) changes only the finding status
			// spacing, checked below against the same oracle file.
			out, err := RenderMarkdownLegacy(p)
			if v.Expect.Kind == "error" {
				if err == nil || err.Error() != v.Expect.Message {
					t.Fatalf("error %v want %q", err, v.Expect.Message)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(testutil.Testdata("vectors", v.Expect.File))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(out)
			if string(out) != string(want) || hex.EncodeToString(sum[:]) != v.Expect.SHA256 {
				t.Fatalf("markdown mismatch\n got %q\nwant %q", out, want)
			}
			checkD31Markdown(t, v.ID, p, want)
		})
	}
	d31WriteMarkdown(t)
	if n != 13 {
		t.Fatalf("expected 13 render vectors, ran %d", n)
	}
}

// TestStructureVectors checks compatible decoding and structure issues
// with exact field paths (validation/structure).
func TestStructureVectors(t *testing.T) {
	files, _ := filepath.Glob(testutil.Testdata("vectors", "validation", "structure", "*.json"))
	if len(files) != 37 {
		t.Fatalf("expected 37 structure vectors, got %d", len(files))
	}
	for _, f := range files {
		var v struct {
			ID    string `json:"id"`
			Input struct {
				Document   json.RawMessage `json:"document"`
				ExpectedID *string         `json:"expectedId"`
			} `json:"input"`
			Expect struct {
				DocumentDecoded bool     `json:"documentDecoded"`
				Issues          []string `json:"issues"`
			} `json:"expect"`
		}
		data, _ := os.ReadFile(f)
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		t.Run(v.ID, func(t *testing.T) {
			p, err := DecodePlan(parseRaw(t, v.Input.Document))
			var got []string
			if err != nil {
				if v.Expect.DocumentDecoded {
					t.Fatalf("unexpected decode failure: %v", err)
				}
				for _, is := range err.(*DecodeError).Issues {
					got = append(got, is.String())
				}
			} else {
				if !v.Expect.DocumentDecoded {
					t.Fatal("expected decode failure")
				}
				got = ValidateStructure(p, v.Input.ExpectedID)
			}
			if strings.Join(got, "\n") != strings.Join(v.Expect.Issues, "\n") {
				t.Fatalf("issues\n got %q\nwant %q", got, v.Expect.Issues)
			}
		})
	}
}

func TestNormalizeID(t *testing.T) {
	cases := map[string]string{
		"  FULL Plan ":           "full-plan",
		"phase-𝒜-x":              "phase-x",
		"step-été":               "step-t",
		"STEP--X__y":             "step-x-y",
		"  Phase A_1 !! ":        "phase-a-1",
		"İx":                     "i-x",
		strings.Repeat("x", 100): strings.Repeat("x", 80),
	}
	for in, want := range cases {
		got, err := NormalizeID(in)
		if err != nil || got != want {
			t.Errorf("NormalizeID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "!!!", "𝒜"} {
		if _, err := NormalizeID(bad); err == nil {
			t.Errorf("NormalizeID(%q) should fail", bad)
		}
	}
}

func TestValidDatetime(t *testing.T) {
	good := []string{"2026-01-02T03:04:05.000Z", "2026-01-02T03:04Z", "2024-02-29T00:00:00Z", "2026-01-02T03:04:05.123456Z"}
	bad := []string{"2026-01-02", "2026-01-02T03:04:05+08:00", "2025-02-29T00:00Z", "yesterday", "2026-13-01T00:00Z"}
	for _, s := range good {
		if !ValidDatetime(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range bad {
		if ValidDatetime(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

// ---- D.3.1 (contracts §14 item 5b): finding rendering ----

type d31Pin struct {
	Change          string `json:"change"`
	OutputSha256    string `json:"outputSha256"`
	RawOutputLength int    `json:"rawOutputLength"`
}

type d31PinFile struct {
	Note    string            `json:"note"`
	Vectors map[string]d31Pin `json:"vectors"`
}

var d31MarkdownPins = map[string]d31Pin{}

func d31Update() bool { return os.Getenv("SHIORI_D31_UPDATE") == "1" }

func readD31Pins(t *testing.T) d31PinFile {
	var f d31PinFile
	data, err := os.ReadFile(testutil.Testdata("d3_1", "expectations.json"))
	if err == nil {
		err = json.Unmarshal(data, &f)
	}
	if err != nil && !d31Update() {
		t.Fatalf("reading d3_1/expectations.json: %v", err)
	}
	if f.Vectors == nil {
		f.Vectors = map[string]d31Pin{}
	}
	return f
}

// checkD31Markdown: the current rendering equals the oracle (legacy)
// rendering with a space before every finding's "(status)", and nothing
// else changes. A render vector whose output changes is pinned.
func checkD31Markdown(t *testing.T, id string, p *Plan, oracle []byte) {
	t.Helper()
	cur, err := RenderMarkdown(p)
	if err != nil {
		t.Fatal(err)
	}
	want := string(oracle)
	for i := range p.Findings {
		f := &p.Findings[i]
		if f.Status == nil || *f.Status == "" {
			continue
		}
		old := "- [" + f.Severity + "] " + f.Title + "(" + *f.Status + ")"
		if !strings.Contains(want, old) {
			t.Fatalf("finding line %q not in the oracle rendering", old)
		}
		want = strings.Replace(want, old, "- ["+f.Severity+"] "+f.Title+" ("+*f.Status+")", 1)
	}
	if string(cur) != want {
		t.Fatalf("D.3.1 rendering differs beyond the finding status spacing\n got %q\nwant %q", cur, want)
	}
	// Markdown generated either way is recognized as generated.
	for _, md := range [][]byte{oracle, cur} {
		if gen, err := IsGeneratedMarkdown(p, md); err != nil || !gen {
			t.Fatalf("IsGeneratedMarkdown = %v, %v", gen, err)
		}
	}
	listed := readD31Pins(t).Vectors[id]
	if string(cur) == string(oracle) {
		if listed.OutputSha256 != "" {
			t.Fatalf("%s is pinned in d3_1 but renders like the oracle", id)
		}
		return
	}
	sum := sha256.Sum256(cur)
	pin := d31Pin{Change: "item 5b finding rendering", OutputSha256: hex.EncodeToString(sum[:]), RawOutputLength: len(cur)}
	if d31Update() {
		d31MarkdownPins[id] = pin
		return
	}
	if listed != pin {
		t.Fatalf("%s: D.3.1 pin %+v, got %+v (SHIORI_D31_UPDATE=1 to re-pin after review)", id, listed, pin)
	}
}

// d31WriteMarkdown rewrites the markdown/ entries of d3_1/expectations.json
// (internal/engine owns the other prefixes).
func d31WriteMarkdown(t *testing.T) {
	if !d31Update() {
		return
	}
	f := readD31Pins(t)
	for id := range f.Vectors {
		if strings.HasPrefix(id, "markdown/") {
			delete(f.Vectors, id)
		}
	}
	for id, p := range d31MarkdownPins {
		f.Vectors[id] = p
	}
	data, _ := json.MarshalIndent(f, "", "  ")
	if err := os.WriteFile(testutil.Testdata("d3_1", "expectations.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
