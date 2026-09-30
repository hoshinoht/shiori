package engine

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
)

// Input normalization shared by create/update (reference behaviour pinned
// by testdata/vectors/mutations and black-box probes): strings are trimmed
// with the ECMAScript whitespace set; string lists drop blanks and
// duplicates, keeping first occurrences; blank optional strings are
// omitted; a supplied id is normalized (an unnormalizable id is an error),
// an omitted id is generated.

func trimDedupe(list []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range list {
		t := model.TrimJS(s)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// appendDedupe appends trimmed nonblank items not already present.
func appendDedupe(base, add []string) []string {
	out := append([]string(nil), base...)
	seen := map[string]bool{}
	for _, s := range base {
		seen[s] = true
	}
	for _, s := range add {
		t := model.TrimJS(s)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func optTrim(v ojson.Value, key string) *string {
	x, ok := v.Get(key)
	if !ok {
		return nil
	}
	t := model.TrimJS(x.Str())
	if t == "" {
		return nil
	}
	return &t
}

func getStr(v ojson.Value, key string) (string, bool) {
	x, ok := v.Get(key)
	if !ok {
		return "", false
	}
	return x.Str(), true
}

func getList(v ojson.Value, key string) ([]string, bool) {
	x, ok := v.Get(key)
	if !ok {
		return nil, false
	}
	out := make([]string, len(x.Elems()))
	for i, e := range x.Elems() {
		out[i] = e.Str()
	}
	return out, true
}

func getObjs(v ojson.Value, key string) []ojson.Value {
	x, _ := v.Get(key)
	return x.Elems()
}

var idWords = []string{"amber", "birch", "cedar", "dawn", "ember", "fern", "grove", "harbor", "iris", "juniper", "kestrel", "lark", "maple", "north", "oak", "pine", "quartz", "reef", "river", "sage", "stone", "tide", "umber", "vale", "willow", "yarrow", "path", "field", "brook", "cliff"}

func randInt(n int64) int64 {
	v, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		panic(err)
	}
	return v.Int64()
}

// IDSource, when set (tests only), supplies generated ids.
var IDSource func(prefix string) string

// generateID returns "<prefix>-<word>-<word>-<6 digits>" unused in taken.
// Only the format is contractual (MANIFEST generation notes).
func generateID(prefix string, taken map[string]bool) string {
	if IDSource != nil {
		return IDSource(prefix)
	}
	for {
		id := fmt.Sprintf("%s-%s-%s-%06d", prefix, idWords[randInt(int64(len(idWords)))], idWords[randInt(int64(len(idWords)))], randInt(1000000))
		if !taken[id] {
			return id
		}
	}
}

// stepFromInput builds a step; position is the 1-based number used in the
// reference's missing-title message.
func stepFromInput(v ojson.Value, position int, taken map[string]bool) (model.Step, error) {
	var st model.Step
	title := ""
	if t, ok := getStr(v, "title"); ok {
		title = model.TrimJS(t)
	}
	if title == "" {
		return st, fmt.Errorf("Step %d is missing a title", position)
	}
	if raw, ok := getStr(v, "id"); ok {
		id, err := model.NormalizeID(raw)
		if err != nil {
			return st, err
		}
		st.ID = id
	} else {
		st.ID = generateID("step", taken)
	}
	if taken[st.ID] {
		return st, fmt.Errorf("Duplicate step id: %s", st.ID)
	}
	taken[st.ID] = true
	st.Title = title
	st.Target = optTrim(v, "target")
	st.Action = optTrim(v, "action")
	st.Validation = optTrim(v, "validation")
	st.Status = "draft"
	if s, ok := getStr(v, "status"); ok {
		st.Status = s
	}
	return st, nil
}

func phaseFromInput(v ojson.Value, position int, taken map[string]bool) (model.Phase, error) {
	var ph model.Phase
	title := ""
	if t, ok := getStr(v, "title"); ok {
		title = model.TrimJS(t)
	}
	if title == "" {
		return ph, fmt.Errorf("Phase %d is missing a title", position)
	}
	if raw, ok := getStr(v, "id"); ok {
		id, err := model.NormalizeID(raw)
		if err != nil {
			return ph, err
		}
		ph.ID = id
	} else {
		ph.ID = generateID("phase", taken)
	}
	if taken[ph.ID] {
		return ph, fmt.Errorf("Duplicate phase id: %s", ph.ID)
	}
	taken[ph.ID] = true
	ph.Title = title
	ph.Status = "draft"
	if s, ok := getStr(v, "status"); ok {
		ph.Status = s
	}
	stepTaken := map[string]bool{}
	ph.Steps = []model.Step{}
	for i, sv := range getObjs(v, "steps") {
		st, err := stepFromInput(sv, i+1, stepTaken)
		if err != nil {
			return ph, err
		}
		ph.Steps = append(ph.Steps, st)
	}
	return ph, nil
}

func phasesFromInput(list []ojson.Value) ([]model.Phase, error) {
	taken := map[string]bool{}
	out := []model.Phase{}
	for i, pv := range list {
		ph, err := phaseFromInput(pv, i+1, taken)
		if err != nil {
			return nil, err
		}
		out = append(out, ph)
	}
	return out, nil
}

func findingFromInput(v ojson.Value, position int) (model.Finding, error) {
	var f model.Finding
	t, _ := getStr(v, "title")
	f.Title = model.TrimJS(t)
	if f.Title == "" {
		return f, fmt.Errorf("Finding %d is missing a title", position)
	}
	f.Severity, _ = getStr(v, "severity")
	f.Detail = optTrim(v, "detail")
	f.Source = optTrim(v, "source")
	st := "open"
	if s, ok := getStr(v, "status"); ok {
		st = s
	}
	f.Status = &st
	return f, nil
}

func findingsFromInput(list []ojson.Value, offset int) ([]model.Finding, error) {
	out := []model.Finding{}
	for i, fv := range list {
		f, err := findingFromInput(fv, offset+i+1)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// backslashError is the D5 refusal for a new link containing '\'.
func backslashError(field, raw string) error {
	return fmt.Errorf("%s: Linked path contains a backslash and has an ambiguous manifest identity: %s", field, raw)
}

// normalizeSpecList trims, normalizes, dedupes and applies the D5 refusal.
func (e *Engine) normalizeSpecList(field string, list []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for i, raw := range list {
		t := model.TrimJS(raw)
		if t == "" {
			continue
		}
		if strings.Contains(t, `\`) {
			return nil, backslashError(field+"."+strconv.Itoa(i), raw)
		}
		rel, err := snapshot.NormalizeSpecFile(e.Root, t)
		if err != nil {
			return nil, err
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	return out, nil
}

// normalizePlanFileInput trims and applies the plan-file policy and D5.
func (e *Engine) normalizePlanFileInput(raw string) (string, error) {
	t := model.TrimJS(raw)
	if strings.Contains(t, `\`) {
		return "", backslashError("planFile", raw)
	}
	return snapshot.NormalizePlanFile(e.Root, t)
}

// withNewline is the reference's explicit-Markdown rule: content is
// written with exactly one appended newline when it lacks a final one.
func withNewline(s string) []byte {
	if strings.HasSuffix(s, "\n") {
		return []byte(s)
	}
	return []byte(s + "\n")
}

// d7Check reports ids that cannot produce a Markdown marker, with their
// field paths (D7: the reference fails without a path).
func d7Check(p *model.Plan) error {
	var issues []string
	check := func(path, id string) {
		if model.Blank(id) {
			issues = append(issues, path+": Must not be empty")
		} else if _, err := model.NormalizeID(id); err != nil {
			issues = append(issues, path+": "+err.Error())
		}
	}
	for i := range p.Phases {
		pp := "phases." + strconv.Itoa(i)
		check(pp+".id", p.Phases[i].ID)
		for j := range p.Phases[i].Steps {
			check(pp+".steps."+strconv.Itoa(j)+".id", p.Phases[i].Steps[j].ID)
		}
	}
	if len(issues) == 0 {
		return nil
	}
	return &D7Error{Issues: issues}
}

// D7Error lists unrenderable ids with field paths.
type D7Error struct{ Issues []string }

func (e *D7Error) Error() string {
	return strings.Join(e.Issues, "; ")
}
