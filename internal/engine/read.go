package engine

import (
	"fmt"
	"strings"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
)

// Read implements workplan_read: the normalized document, an optional
// phase/step selection, the linked Markdown and the dependency view.
func (e *Engine) Read(in ReadInput) (ojson.Value, error) {
	id, err := normalizeRequested(in.ID)
	if err != nil {
		return ojson.Value{}, err
	}
	s, err := e.load(id)
	if err != nil {
		return ojson.Value{}, err
	}
	if s.Journal.Exists {
		return recoveryPacket(s, "read"), nil
	}
	p := s.Plan
	selection, err := selectPhases(p, in.PhaseID, in.StepID)
	if err != nil {
		return ojson.Value{}, err
	}
	plan := ojson.NewObject(3).
		Set("path", ojson.StringValue(s.Markdown.Path)).
		Set("exists", ojson.BoolValue(s.Markdown.Exists))
	if in.IncludeMarkdown == nil || *in.IncludeMarkdown {
		if s.Markdown.Exists {
			plan.Set("content", ojson.StringValue(bytesToString(s.Markdown.Bytes)))
		} else {
			plan.Set("content", ojson.NullValue())
		}
	}
	out := ojson.NewObject(7).
		Set("path", ojson.StringValue(s.JSON.Path)).
		Set("workplan", p.ToValue()).
		Set("selection", selection).
		Set("plan", plan.Value()).
		Set("dependencies", e.dependencies(s, nil).value()).
		Set("planHash", ojson.StringValue(s.PlanHash)).
		Set("stateHash", ojson.StringValue(s.StateHash)).
		Value()
	// D9: the reference returns unbounded output; Go refuses above the
	// response frame limit. Estimate cheaply before measuring exactly.
	est := 3*len(s.JSON.Bytes) + 2*len(s.Markdown.Bytes)
	if est > e.maxResponse()/2 {
		return e.checkResponse(out, "workplan_read", id)
	}
	return out, nil
}

// bytesToString decodes UTF-8 with U+FFFD replacement (TextDecoder).
func bytesToString(b []byte) string { return strings.ToValidUTF8(string(b), "�") }

func selectPhases(p *model.Plan, phaseID, stepID *string) (ojson.Value, error) {
	var phases []ojson.Value
	candidates := make([]int, 0, len(p.Phases))
	if phaseID != nil {
		for i := range p.Phases {
			if p.Phases[i].ID == *phaseID {
				candidates = append(candidates, i)
				break
			}
		}
		if len(candidates) == 0 {
			return ojson.Value{}, fmt.Errorf("Phase not found: %s", *phaseID)
		}
	} else {
		for i := range p.Phases {
			candidates = append(candidates, i)
		}
	}
	if stepID == nil {
		for _, i := range candidates {
			phases = append(phases, p.Phases[i].ToValue())
		}
	} else {
		matches := 0
		for _, i := range candidates {
			ph := p.Phases[i]
			var kept []model.Step
			for _, st := range ph.Steps {
				if st.ID == *stepID {
					kept = append(kept, st)
				}
			}
			if len(kept) > 0 {
				matches += len(kept)
				ph.Steps = kept
				phases = append(phases, ph.ToValue())
			}
		}
		if matches != 1 {
			return ojson.Value{}, fmt.Errorf("Step filter must identify exactly one step: %s", *stepID)
		}
	}
	return ojson.NewObject(3).
		Set("phaseId", ptrValue(phaseID)).
		Set("stepId", ptrValue(stepID)).
		Set("phases", ojson.ArrayValue(phases)).Value(), nil
}

// MarkdownGenerated reports whether the linked Markdown byte-equals the
// generated rendering of the normalized stored JSON (the reference's
// "generated" classification; handwritten Markdown is never regenerated).
func (e *Engine) MarkdownGenerated(id string) (generated, present bool, err error) {
	s, err := e.load(id)
	if err != nil {
		return false, false, err
	}
	if !s.Markdown.Exists {
		return false, false, nil
	}
	out, err := model.RenderMarkdown(s.Plan)
	if err != nil {
		return false, true, err
	}
	return string(out) == string(s.Markdown.Bytes), true, nil
}
