package engine

import (
	"fmt"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
)

// validationIssues is the ordered issue list for workplan_validate:
// structure rules, linked Markdown, linked specs, dependency metadata and
// pending recovery.
func (e *Engine) validationIssues(s *snapshot.Snapshot, requested string) ([]string, depView) {
	p := s.Plan
	issues := model.ValidateStructure(p, &requested)
	if !s.Markdown.Exists || model.Blank(string(s.Markdown.Bytes)) {
		issues = append(issues, "planFile: Linked Markdown is missing or empty: "+p.PlanFile)
	}
	for _, sp := range s.Specs {
		if !sp.Exists || model.Blank(string(sp.Bytes)) {
			issues = append(issues, "specFiles: Linked spec is missing or empty: "+sp.Rel)
		}
	}
	for _, bp := range s.BackslashPaths {
		// D5 (approved): the manifest rewrites '\' to '/' for hash parity,
		// which can alias a different file. The plan stays readable; the
		// ambiguity is diagnosed here and writers refuse new such links.
		field := "specFiles"
		if bp == p.PlanFile {
			field = "planFile"
		}
		issues = append(issues, field+": Linked path contains a backslash and has an ambiguous manifest identity: "+bp)
	}
	dv := e.dependencies(s, nil)
	for _, is := range dv.issues {
		issues = append(issues, "dependencies: "+is)
	}
	if s.Journal.Exists {
		issues = append(issues, "transaction: Recovery required at "+s.Journal.Rel)
	}
	if issues == nil {
		issues = []string{}
	}
	return issues, dv
}

// msgMarkdownDrift is the D.1 non-failing warning for linked Markdown that
// is not the generated rendering of the plan JSON (contracts §11 item C).
const msgMarkdownDrift = "planFile: Linked Markdown is not the generated rendering of the plan JSON (handwritten or edited): %s. " +
	"Later JSON changes (status, notes, findings, compaction) are not written into it. " +
	"To regenerate it from the JSON (discarding the edits) use workplan_reset with mode \"markdown-only\" and replaceMarkdown=true; otherwise keep it in sync by hand."

// validationWarnings are non-failing diagnostics (D.1); they never change
// "valid". A missing or empty Markdown is an issue, not a warning, and a
// plan whose ids cannot render is diagnosed by the structure rules.
func validationWarnings(s *snapshot.Snapshot) []string {
	if !s.Markdown.Exists || model.Blank(string(s.Markdown.Bytes)) {
		return nil
	}
	out, err := model.RenderMarkdown(s.Plan)
	if err != nil || string(out) == string(s.Markdown.Bytes) {
		return nil
	}
	return []string{fmt.Sprintf(msgMarkdownDrift, s.Markdown.Rel)}
}

// Validate implements workplan_validate. Load failures are reported as a
// single issue instead of an error.
func (e *Engine) Validate(in ValidateInput) (ojson.Value, error) {
	id, err := normalizeRequested(in.ID)
	if err != nil {
		return ojson.Value{}, err
	}
	s, err := e.load(id)
	if err != nil {
		if IsUnsupported(err) {
			return ojson.Value{}, err
		}
		return ojson.NewObject(3).
			Set("valid", ojson.BoolValue(false)).
			Set("issueCount", ojson.IntValue(1)).
			Set("issues", ojson.StringsValue([]string{err.Error()})).Value(), nil
	}
	issues, dv := e.validationIssues(s, in.ID)
	b := ojson.NewObject(11).
		Set("path", ojson.StringValue(s.JSON.Path)).
		Set("planPath", ojson.StringValue(s.Markdown.Path)).
		Set("valid", ojson.BoolValue(len(issues) == 0)).
		Set("issueCount", ojson.IntValue(int64(len(issues)))).
		Set("issues", ojson.StringsValue(issues))
	// D.1: additive, present only when there is something to report, so
	// outputs without warnings stay byte-identical to the reference.
	if w := validationWarnings(s); len(w) > 0 {
		b.Set("warnings", ojson.StringsValue(w))
	}
	return b.
		Set("workplan", s.Plan.Summary()).
		Set("planHash", ojson.StringValue(s.PlanHash)).
		Set("stateHash", ojson.StringValue(s.StateHash)).
		Set("planFresh", ojson.BoolValue(len(s.MissingPlanArtifacts) == 0)).
		Set("dependenciesRecorded", ojson.BoolValue(dv.recorded)).Value(), nil
}
