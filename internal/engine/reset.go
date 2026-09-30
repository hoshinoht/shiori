package engine

import (
	"errors"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/storage"
)

// PrepareReset prepares workplan_reset. mode "draft" clears phases,
// findings and (unless preserveNotes) notes and sets status draft;
// "markdown-only" regenerates the linked Markdown from the stored JSON.
// Handwritten Markdown is replaced only with replaceMarkdown=true. When
// nothing would change (already-generated Markdown), no intent is prepared
// and no authorization is requested (D12).
func (e *Engine) PrepareReset(data ojson.Value) (*Prepared, error) {
	rawID, _ := getStr(data, "id")
	s, err := e.loadForMutation(rawID, optHash(data))
	if err != nil {
		return nil, err
	}
	id := s.ID
	if err := d7Check(s.Plan); err != nil {
		return nil, err
	}
	mode, _ := getStr(data, "mode")
	preserveNotes, replaceMD := false, false
	if v, ok := data.Get("preserveNotes"); ok {
		preserveNotes = v.Bool()
	}
	if v, ok := data.Get("replaceMarkdown"); ok {
		replaceMD = v.Bool()
	}
	gen, err := e.generatedMarkdown(s)
	if err != nil {
		return nil, err
	}
	if s.Markdown.Exists && !gen && !replaceMD {
		return nil, errors.New(msgHandwrittenReset)
	}
	p := s.Plan.Clone()
	var specs []targetSpec
	if mode == "draft" {
		p.Phases = []model.Phase{}
		p.Findings = []model.Finding{}
		if !preserveNotes {
			p.Notes = []string{}
		}
		p.Status = "draft"
		p.UpdatedAt = nowISO()
		specs = append(specs, targetSpec{rel: s.JSON.Rel, kind: "plan", before: s.JSON.Bytes, beforeOK: true, after: p.EncodeStored(), afterOK: true})
	}
	md, err := model.RenderMarkdown(p)
	if err != nil {
		return nil, err
	}
	specs = append(specs, targetSpec{rel: p.PlanFile, kind: "markdown", before: s.Markdown.Bytes, beforeOK: s.Markdown.Exists, after: md, afterOK: true})
	in := e.buildIntent("reset:"+mode, id, storage.NewUUID(), specs, readsOf(s.StateManifest))
	if err := e.checkTargetPaths(in); err != nil {
		return nil, err
	}
	prep := &Prepared{Tool: "workplan_reset", Intent: in}
	prep.result = func(sync bool) (Output, error) {
		post, err := e.postSnapshot(in, id)
		if err != nil {
			return Output{}, err
		}
		return Output{Value: ojson.NewObject(8).
			Set("reset", ojson.BoolValue(true)).
			Set("mode", ojson.StringValue(mode)).
			Set("path", ojson.StringValue(e.absRel(s.JSON.Rel))).
			Set("planPath", ojson.StringValue(e.absRel(p.PlanFile))).
			Set("workplan", post.Plan.Summary()).
			Set("planHash", ojson.StringValue(post.PlanHash)).
			Set("stateHash", ojson.StringValue(post.StateHash)).
			Set("directorySync", dirSyncValue(sync)).Value()}, nil
	}
	return finalize(prep), nil
}
