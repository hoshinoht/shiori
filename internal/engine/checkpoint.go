package engine

import (
	"errors"
	"fmt"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/storage"
)

func terminal(status string) bool { return status == "completed" || status == "cancelled" }

// defaultStep picks the checkpoint position within a phase: the first
// non-draft, non-terminal step, else the first draft step.
func defaultStep(ph *model.Phase) int {
	for j := range ph.Steps {
		if st := ph.Steps[j].Status; !terminal(st) && st != "draft" {
			return j
		}
	}
	for j := range ph.Steps {
		if ph.Steps[j].Status == "draft" {
			return j
		}
	}
	return -1
}

func position(ph *model.Phase, j int) *model.Position {
	st := &ph.Steps[j]
	return &model.Position{PhaseID: ph.ID, PhaseTitle: ph.Title, PhaseStatus: ph.Status, StepID: st.ID, StepTitle: st.Title, StepStatus: st.Status}
}

// checkpointPosition resolves the current position (reference rules).
func checkpointPosition(p *model.Plan, phaseID, stepID *string) (*model.Position, error) {
	if phaseID != nil {
		pi := -1
		for i := range p.Phases {
			if p.Phases[i].ID == *phaseID {
				pi = i
				break
			}
		}
		if pi < 0 {
			return nil, fmt.Errorf("Phase not found: %s", *phaseID)
		}
		ph := &p.Phases[pi]
		if terminal(ph.Status) {
			return nil, fmt.Errorf("Current checkpoint phase is terminal: %s", ph.ID)
		}
		if stepID == nil {
			if j := defaultStep(ph); j >= 0 {
				return position(ph, j), nil
			}
			return nil, nil
		}
		for j := range ph.Steps {
			if ph.Steps[j].ID == *stepID {
				if terminal(ph.Steps[j].Status) {
					return nil, errors.New("Current checkpoint step must not be completed or cancelled")
				}
				return position(ph, j), nil
			}
		}
		return nil, fmt.Errorf("Step not found in phase %s: %s", ph.ID, *stepID)
	}
	if stepID != nil {
		var found []struct{ i, j int }
		for i := range p.Phases {
			for j := range p.Phases[i].Steps {
				if p.Phases[i].Steps[j].ID == *stepID {
					found = append(found, struct{ i, j int }{i, j})
				}
			}
		}
		if len(found) != 1 {
			return nil, fmt.Errorf("Step id must identify exactly one phase: %s", *stepID)
		}
		ph := &p.Phases[found[0].i]
		if terminal(ph.Status) || terminal(ph.Steps[found[0].j].Status) {
			return nil, errors.New("Current checkpoint step must not be completed or cancelled")
		}
		return position(ph, found[0].j), nil
	}
	for i := range p.Phases {
		ph := &p.Phases[i]
		if terminal(ph.Status) {
			continue
		}
		if j := defaultStep(ph); j >= 0 {
			return position(ph, j), nil
		}
	}
	return nil, nil
}

func manifestEntries(entries []snapshot.Entry) []model.ManifestEntry {
	out := make([]model.ManifestEntry, len(entries))
	for i, en := range entries {
		out[i] = model.ManifestEntry{Path: en.Path, SHA256: en.SHA256, Missing: en.Missing}
	}
	return out
}

// PrepareCheckpoint prepares workplan_checkpoint (checkpoint v2 binding
// the complete plan manifest; evidence stays unverified).
func (e *Engine) PrepareCheckpoint(data ojson.Value) (*Prepared, error) {
	rawID, _ := getStr(data, "id")
	s, err := e.loadForMutation(rawID, optHash(data))
	if err != nil {
		return nil, err
	}
	id := s.ID
	p := s.Plan
	if err := model.UniqueIDError(p); err != nil {
		return nil, err
	}
	summary, ok := nonblank(data, "summary")
	if !ok {
		return nil, errors.New("Checkpoint summary cannot be empty")
	}
	next, ok := nonblank(data, "nextAction")
	if !ok {
		return nil, errors.New("Checkpoint nextAction cannot be empty")
	}
	var phaseID, stepID *string
	if v, ok := getStr(data, "phaseId"); ok {
		phaseID = &v
	}
	if v, ok := getStr(data, "stepId"); ok {
		stepID = &v
	}
	cur, err := checkpointPosition(p, phaseID, stepID)
	if err != nil {
		return nil, err
	}
	now := nowISO()
	cp := &model.Checkpoint{SchemaVersion: 2, ID: id, SourceUpdatedAt: p.UpdatedAt, PlanHash: s.PlanHash,
		Manifest: manifestEntries(s.PlanManifest), CreatedAt: now, UpdatedAt: now, Status: p.Status,
		Summary: summary, Current: cur, NextAction: next}
	if old := classifyCheckpoint(s); old.cp != nil && old.cp.CreatedAt != "" {
		cp.CreatedAt = old.cp.CreatedAt
	}
	for _, f := range []struct {
		key string
		dst *[]string
	}{{"blockers", &cp.Blockers}, {"recentValidation", &cp.RecentValidation}, {"guardrails", &cp.Guardrails}, {"references", &cp.References}} {
		l, _ := getList(data, f.key)
		*f.dst = trimDedupe(l)
	}
	cpValue := model.CheckpointValue(cp, false)
	in := e.buildIntent("checkpoint", id, storage.NewUUID(), []targetSpec{{
		rel: s.Checkpoint.Rel, kind: "checkpoint", before: s.Checkpoint.Bytes, beforeOK: s.Checkpoint.Exists,
		after: append(ojson.Pretty(cpValue), '\n'), afterOK: true, forceWrite: true,
	}}, readsOf(s.StateManifest))
	if err := e.checkTargetPaths(in); err != nil {
		return nil, err
	}
	prep := &Prepared{Tool: "workplan_checkpoint", Intent: in}
	prep.result = func(sync bool) (Output, error) {
		post, err := e.postSnapshot(in, id)
		if err != nil {
			return Output{}, err
		}
		return Output{Value: ojson.NewObject(7).
			Set("checkpointPath", ojson.StringValue(e.absRel(s.Checkpoint.Rel))).
			Set("checkpoint", cpValue).
			Set("planFresh", ojson.BoolValue(len(s.MissingPlanArtifacts) == 0)).
			Set("evidenceStatus", ojson.StringValue("unverified")).
			Set("planHash", ojson.StringValue(post.PlanHash)).
			Set("stateHash", ojson.StringValue(post.StateHash)).
			Set("directorySync", dirSyncValue(sync)).Value()}, nil
	}
	return finalize(prep), nil
}
