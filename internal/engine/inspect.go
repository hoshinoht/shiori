package engine

import (
	"fmt"
	"strconv"

	"github.com/hoshinoht/shiori/internal/index"
	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
)

// DefaultInspectLimit is the inspect page size default.
const DefaultInspectLimit = 100

// Inspect implements workplan_inspect: stable phase/step ids and Markdown
// markers, paged with a snapshot/options-bound checksummed cursor.
func (e *Engine) Inspect(in InspectInput) (ojson.Value, error) {
	id, err := normalizeRequested(in.ID)
	if err != nil {
		return ojson.Value{}, err
	}
	s, err := e.load(id)
	if err != nil {
		return ojson.Value{}, err
	}
	if s.Journal.Exists {
		return recoveryPacket(s, "inspect"), nil
	}
	p := s.Plan
	if err := model.UniqueIDError(p); err != nil {
		return ojson.Value{}, err
	}
	ix := index.Build(p)
	limit := DefaultInspectLimit
	if in.Limit != nil {
		limit = *in.Limit
	}
	phaseIdx := -1
	if in.PhaseID != nil {
		i, ok := ix.PhaseByID[*in.PhaseID]
		if !ok {
			return ojson.Value{}, fmt.Errorf("Phase not found: %s", *in.PhaseID)
		}
		phaseIdx = i
	}
	// Flatten in document order: each phase followed by its steps.
	type item struct {
		phase int
		step  int // -1 for the phase itself
	}
	var items []item
	for i := range p.Phases {
		if phaseIdx >= 0 && i != phaseIdx {
			continue
		}
		items = append(items, item{i, -1})
		for j := range p.Phases[i].Steps {
			items = append(items, item{i, j})
		}
	}
	offset := 0
	if in.Cursor != nil {
		c, err := parseInspectCursor(*in.Cursor)
		if err != nil {
			return ojson.Value{}, err
		}
		if c.stateHash != s.StateHash || c.limit != limit || !eqPtr(c.phaseID, in.PhaseID) || c.offset > len(items) {
			return ojson.Value{}, errInspectCursorStale
		}
		offset = c.offset
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	phases := []ojson.Value{}
	steps := []ojson.Value{}
	for _, it := range items[offset:end] {
		ph := &p.Phases[it.phase]
		if it.step < 0 {
			marker, err := model.PhaseMarker(ph.ID)
			if err != nil {
				return ojson.Value{}, err
			}
			phases = append(phases, ojson.NewObject(8).
				Set("type", ojson.StringValue("phase")).
				Set("id", ojson.StringValue(ph.ID)).
				Set("title", ojson.StringValue(ph.Title)).
				Set("status", ojson.StringValue(ph.Status)).
				Set("index", ojson.IntValue(int64(it.phase))).
				Set("indexLabel", ojson.StringValue(strconv.Itoa(it.phase+1))).
				Set("stepCount", ojson.IntValue(int64(len(ph.Steps)))).
				Set("markdownMarker", ojson.StringValue(marker)).Value())
			continue
		}
		st := &ph.Steps[it.step]
		marker, err := model.StepMarker(st.ID)
		if err != nil {
			return ojson.Value{}, err
		}
		steps = append(steps, ojson.NewObject(10).
			Set("type", ojson.StringValue("step")).
			Set("phaseId", ojson.StringValue(ph.ID)).
			Set("phaseTitle", ojson.StringValue(ph.Title)).
			Set("id", ojson.StringValue(st.ID)).
			Set("title", ojson.StringValue(st.Title)).
			Set("status", ojson.StringValue(st.Status)).
			Set("target", ptrValue(st.Target)).
			Set("index", ojson.IntValue(int64(it.step))).
			Set("indexPath", ojson.StringValue(strconv.Itoa(it.phase+1)+"."+strconv.Itoa(it.step+1))).
			Set("markdownMarker", ojson.StringValue(marker)).Value())
	}
	next := ojson.NullValue()
	if end < len(items) {
		next = ojson.StringValue(encodeCursor(inspectDomain, inspectCursor{
			stateHash: s.StateHash, phaseID: in.PhaseID, limit: limit, offset: end,
		}.fields()))
	}
	return ojson.NewObject(9).
		Set("path", ojson.StringValue(s.JSON.Path)).
		Set("workplan", p.Summary()).
		Set("plan", ojson.NewObject(2).
			Set("path", ojson.StringValue(s.Markdown.Path)).
			Set("exists", ojson.BoolValue(s.Markdown.Exists)).Value()).
		Set("phases", ojson.ArrayValue(phases)).
		Set("steps", ojson.ArrayValue(steps)).
		Set("dependencies", e.dependencies(s, ix).value()).
		Set("pagination", ojson.NewObject(5).
			Set("total", ojson.IntValue(int64(len(items)))).
			Set("offset", ojson.IntValue(int64(offset))).
			Set("returned", ojson.IntValue(int64(end-offset))).
			Set("omitted", ojson.IntValue(int64(len(items)-end))).
			Set("nextCursor", next).Value()).
		Set("planHash", ojson.StringValue(s.PlanHash)).
		Set("stateHash", ojson.StringValue(s.StateHash)).Value(), nil
}
