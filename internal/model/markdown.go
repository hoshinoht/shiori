package model

import (
	"strconv"
	"strings"
)

// PhaseMarker is the generated Markdown marker for a phase id.
func PhaseMarker(id string) (string, error) {
	n, err := NormalizeID(id)
	if err != nil {
		return "", err
	}
	return "<!-- workplan-phase-id: " + n + " -->", nil
}

// StepMarker is the generated Markdown marker for a step id.
func StepMarker(id string) (string, error) {
	n, err := NormalizeID(id)
	if err != nil {
		return "", err
	}
	return "<!-- workplan-step-id: " + n + " -->", nil
}

// RenderMarkdown renders the generated Markdown for a normalized plan
// (byte parity with testdata/vectors/markdown). Markers use the lossy
// normalized id (D8); a marker that normalizes to empty is an error.
func RenderMarkdown(p *Plan) ([]byte, error) {
	var b strings.Builder
	title := p.ID
	if p.Title != nil && !Blank(*p.Title) {
		title = TrimJS(*p.Title)
	}
	list := func(items []string) string {
		if len(items) == 0 {
			return "_None_"
		}
		lines := make([]string, len(items))
		for i, s := range items {
			lines[i] = "- " + s
		}
		return strings.Join(lines, "\n")
	}
	sections := []string{
		"# " + title,
		"## Goal\n" + p.Goal,
		"## Scope\n" + list(p.Scope),
		"## Non-goals\n" + list(p.NonGoals),
		"## Constraints\n" + list(p.Constraints),
		"## Relevant files\n" + list(p.RelevantFiles),
		"## Spec files\n" + list(p.SpecFiles),
	}
	phases := "_No phases defined yet._"
	if len(p.Phases) > 0 {
		blocks := make([]string, len(p.Phases))
		for i := range p.Phases {
			ph := &p.Phases[i]
			marker, err := PhaseMarker(ph.ID)
			if err != nil {
				return nil, err
			}
			label := strconv.Itoa(i + 1)
			lines := []string{
				"### " + label + ". " + ph.Title + " " + marker,
				"- Status: " + ph.Status,
				"- Id: " + ph.ID,
			}
			if len(ph.Steps) == 0 {
				lines = append(lines, "- Steps: _None yet_")
			}
			for j := range ph.Steps {
				st := &ph.Steps[j]
				sm, err := StepMarker(st.ID)
				if err != nil {
					return nil, err
				}
				lines = append(lines,
					"#### "+label+"."+strconv.Itoa(j+1)+" "+st.Title+" "+sm,
					"- Status: "+st.Status,
					"- Id: "+st.ID)
				if st.Target != nil && *st.Target != "" {
					lines = append(lines, "- Target: "+*st.Target)
				}
				if st.Action != nil && *st.Action != "" {
					lines = append(lines, "- Action: "+*st.Action)
				}
				if st.Validation != nil && *st.Validation != "" {
					lines = append(lines, "- Validation: "+*st.Validation)
				}
			}
			blocks[i] = strings.Join(lines, "\n")
		}
		phases = strings.Join(blocks, "\n\n")
	}
	sections = append(sections, "## Execution phases\n"+phases)
	findings := "_None_"
	if len(p.Findings) > 0 {
		lines := make([]string, len(p.Findings))
		for i := range p.Findings {
			f := &p.Findings[i]
			line := "- [" + f.Severity + "] " + f.Title
			if f.Status != nil && *f.Status != "" {
				line += "(" + *f.Status + ")"
			}
			if f.Detail != nil && *f.Detail != "" {
				line += "— " + *f.Detail
			}
			if f.Source != nil && *f.Source != "" {
				line += " [source: " + *f.Source + "]"
			}
			lines[i] = line
		}
		findings = strings.Join(lines, "\n")
	}
	sections = append(sections,
		"## Adversarial review findings\n"+findings,
		"## Notes\n"+list(p.Notes),
		"## Status\n- Overall status: "+p.Status+
			"\n- Metadata file: .opencode/workplan/"+p.ID+".json"+
			"\n- Detailed plan file: "+p.PlanFile)
	b.WriteString(strings.Join(sections, "\n\n"))
	b.WriteString("\n")
	return []byte(b.String()), nil
}
