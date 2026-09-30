package engine

import (
	"errors"
	"fmt"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
	"github.com/hoshinoht/shiori/internal/storage"
)

// PrepareCreate prepares workplan_create from accepted input data
// (ParseMutationInput("create", ...)).
func (e *Engine) PrepareCreate(data ojson.Value) (*Prepared, error) {
	rawID, _ := getStr(data, "id")
	id, err := model.NormalizeID(rawID)
	if err != nil {
		return nil, err
	}
	overwrite := false
	if v, ok := data.Get("overwrite"); ok {
		overwrite = v.Bool()
	}
	now := nowISO()
	p := &model.Plan{SchemaVersion: ojson.IntValue(2), ID: id, HasSpecFiles: true, CreatedAt: now, UpdatedAt: now}
	p.Kind = "general"
	if k, ok := getStr(data, "kind"); ok && !model.Blank(k) {
		p.Kind = model.TrimJS(k)
	}
	p.Title = optTrim(data, "title")
	g, _ := getStr(data, "goal")
	p.Goal = model.TrimJS(g)
	for _, f := range []struct {
		key string
		dst *[]string
	}{{"scope", &p.Scope}, {"nonGoals", &p.NonGoals}, {"constraints", &p.Constraints}, {"relevantFiles", &p.RelevantFiles}, {"notes", &p.Notes}} {
		l, _ := getList(data, f.key)
		*f.dst = trimDedupe(l)
	}
	specs, _ := getList(data, "specFiles")
	if p.SpecFiles, err = e.normalizeSpecList("specFiles", specs); err != nil {
		return nil, err
	}
	if p.Phases, err = phasesFromInput(getObjs(data, "phases")); err != nil {
		return nil, err
	}
	if p.Findings, err = findingsFromInput(getObjs(data, "reviewFindings"), 0); err != nil {
		return nil, err
	}
	p.Status = "draft"
	if s, ok := getStr(data, "status"); ok {
		p.Status = s
	}
	planFileSet := false
	if raw, ok := getStr(data, "planFile"); ok {
		if p.PlanFile, err = e.normalizePlanFileInput(raw); err != nil {
			return nil, err
		}
		planFileSet = true
	} else {
		p.PlanFile = snapshot.WorkplanDir + "/" + id + ".md"
	}
	if err := statusGate(p.Status, p); err != nil {
		return nil, err
	}
	explicitMD, hasMD := getStr(data, "planMarkdown")
	replaceMD := false
	if v, ok := data.Get("replaceMarkdown"); ok {
		replaceMD = v.Bool()
	}

	jsonRel := snapshot.PlanRel(id)
	r := &snapshot.Reader{Root: e.Root, Limits: e.Limits}
	var reads []storage.ReadEntry
	var jsonBefore, mdBefore snapshot.Artifact
	op := "create"
	if !overwrite {
		if jsonBefore, err = r.ReadFile(jsonRel); err != nil {
			return nil, err
		}
		if jsonBefore.Exists {
			return nil, fmt.Errorf("Workplan already exists: %s", jsonBefore.Path)
		}
		if mdBefore, err = r.ReadFile(p.PlanFile); err != nil {
			return nil, err
		}
		if mdBefore.Exists {
			return nil, fmt.Errorf("Plan file already exists: %s", mdBefore.Path)
		}
		reads = append(reads, storage.ReadEntry{Rel: jsonRel, Missing: true}, storage.ReadEntry{Rel: p.PlanFile, Missing: true})
		for _, suffix := range []string{".checkpoint.json", ".dependencies.json", ".transaction.json"} {
			a, err := r.ReadFile(snapshot.SidecarRel(id, suffix))
			if err != nil {
				return nil, err
			}
			if a.Exists {
				return nil, fmt.Errorf("Refusing to overwrite existing workplan artifact: %s", a.Path)
			}
			if suffix != ".transaction.json" {
				reads = append(reads, storage.ReadEntry{Rel: a.Rel, Missing: true})
			}
		}
	} else {
		op = "create:overwrite"
		if jsonBefore, err = r.ReadFile(jsonRel); err != nil {
			return nil, err
		}
		if !jsonBefore.Exists {
			return nil, fmt.Errorf("Cannot overwrite missing workplan: %s", jsonBefore.Path)
		}
		expected := optHash(data)
		s, err := e.load(id)
		if err != nil {
			return nil, err
		}
		if !planFileSet {
			p.PlanFile = s.Plan.PlanFile
		} else if p.PlanFile != s.Plan.PlanFile {
			return nil, fmt.Errorf("Overwrite target does not match the existing linked planFile %s; use workplan_update to move or update it", s.Plan.PlanFile)
		}
		if s, err = e.loadForMutation(id, expected); err != nil {
			return nil, err
		}
		if s.Markdown.Exists && !replaceMD {
			gen, err := e.generatedMarkdown(s)
			if err != nil {
				return nil, err
			}
			if !gen {
				return nil, errors.New(msgHandwrittenUpdate)
			}
		}
		jsonBefore, mdBefore = s.JSON, s.Markdown
		reads = readsOf(s.StateManifest)
	}
	var md []byte
	if hasMD {
		md = withNewline(explicitMD)
	} else if md, err = model.RenderMarkdown(p); err != nil {
		return nil, err
	}
	tx := storage.NewUUID()
	in := e.buildIntent(op, id, tx, []targetSpec{
		{rel: jsonRel, kind: "plan", before: jsonBefore.Bytes, beforeOK: jsonBefore.Exists, after: p.EncodeStored(), afterOK: true, forceWrite: true},
		{rel: p.PlanFile, kind: "markdown", before: mdBefore.Bytes, beforeOK: mdBefore.Exists, after: md, afterOK: true},
	}, reads)
	if err := e.checkTargetPaths(in); err != nil {
		return nil, err
	}
	all, mds := newTargets(in)
	if err := e.claimCheck(id, all, mds); err != nil {
		return nil, err
	}
	prep := &Prepared{Tool: "workplan_create", Intent: in}
	prep.recheck = func() error { return e.claimCheck(id, all, mds) }
	prep.result = func(sync bool) (Output, error) {
		post, err := e.postSnapshot(in, id)
		if err != nil {
			return Output{}, err
		}
		return Output{Value: ojson.NewObject(8).
			Set("created", ojson.BoolValue(!overwrite)).
			Set("overwritten", ojson.BoolValue(overwrite)).
			Set("path", ojson.StringValue(e.absRel(jsonRel))).
			Set("planPath", ojson.StringValue(e.absRel(p.PlanFile))).
			Set("workplan", post.Plan.Summary()).
			Set("planHash", ojson.StringValue(post.PlanHash)).
			Set("stateHash", ojson.StringValue(post.StateHash)).
			Set("directorySync", dirSyncValue(sync)).Value()}, nil
	}
	return finalize(prep), nil
}

const (
	msgHandwrittenUpdate = "Refusing to replace handwritten Markdown; pass replaceMarkdown=true for an explicit full replacement"
	msgHandwrittenReset  = "Refusing to replace handwritten Markdown; pass replaceMarkdown=true for an explicit replacement"
)

func optHash(data ojson.Value) *string {
	if v, ok := data.Get("expectedHash"); ok {
		s := v.Str()
		return &s
	}
	return nil
}

// generatedMarkdown reports whether the stored Markdown byte-equals the
// rendering of the stored plan. Ids that cannot render are reported with
// field paths (D7).
func (e *Engine) generatedMarkdown(s *snapshot.Snapshot) (bool, error) {
	if !s.Markdown.Exists {
		return false, nil
	}
	if err := d7Check(s.Plan); err != nil {
		return false, err
	}
	out, err := model.RenderMarkdown(s.Plan)
	if err != nil {
		return false, err
	}
	return string(out) == string(s.Markdown.Bytes), nil
}

// checkTargetPaths refuses targets that escape the root through symlinks
// or are themselves symlinks (spec 01 §4).
func (e *Engine) checkTargetPaths(in *storage.Intent) error {
	if in == nil {
		return nil
	}
	for _, t := range in.Targets {
		if err := snapshot.CheckWritable(e.Root, t.Rel); err != nil {
			return err
		}
	}
	return nil
}
