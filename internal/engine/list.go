package engine

import (
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
	"github.com/hoshinoht/shiori/internal/snapshot"
)

// Artifact classification kinds (spec 01 §4).
const (
	kindCheckpoint   = "checkpoint"
	kindDependencies = "dependencies"
	kindTransaction  = "transaction"
	kindLock         = "lock"
	kindTemporary    = "temporary"
	kindArchive      = "archive"
)

type dirEntry struct {
	name string
	kind string // sidecar kind, or "" for a primary plan candidate
}

// classifyName classifies one directory entry name. ok=false means the
// entry is unrelated (ignored).
func classifyName(name string, isDir bool) (dirEntry, bool) {
	if isDir {
		if name == "archive" {
			return dirEntry{name, kindArchive}, true
		}
		return dirEntry{}, false
	}
	switch {
	case strings.HasPrefix(name, "."):
		if strings.HasSuffix(name, ".lock") {
			return dirEntry{name, kindLock}, true
		}
		return dirEntry{name, kindTemporary}, true
	case strings.HasSuffix(name, ".checkpoint.json"):
		return dirEntry{name, kindCheckpoint}, true
	case strings.HasSuffix(name, ".dependencies.json"):
		return dirEntry{name, kindDependencies}, true
	case strings.HasSuffix(name, ".transaction.json"):
		return dirEntry{name, kindTransaction}, true
	case strings.HasSuffix(name, ".json"):
		return dirEntry{name, ""}, true
	}
	return dirEntry{}, false
}

type dirListing struct {
	exists   bool
	primary  []string // plan file names without ".json", UTF-16 order
	sidecars []dirEntry
}

// scanDir enumerates .opencode/workplan without following or modifying
// anything. Primary plans and sidecars are ordered by UTF-16 code units
// (D6: no dependence on locale or readdir order).
func (e *Engine) scanDir() (dirListing, error) {
	var l dirListing
	ents, err := os.ReadDir(e.dir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return l, nil
		}
		return l, err
	}
	l.exists = true
	for _, de := range ents {
		isDir := de.IsDir()
		if de.Type()&fs.ModeSymlink != 0 {
			isDir = false
		}
		ent, ok := classifyName(de.Name(), isDir)
		if !ok {
			continue
		}
		if ent.kind == "" {
			l.primary = append(l.primary, strings.TrimSuffix(ent.name, ".json"))
		} else {
			l.sidecars = append(l.sidecars, ent)
		}
	}
	sort.SliceStable(l.primary, func(i, j int) bool { return ojson.CompareUTF16(l.primary[i], l.primary[j]) < 0 })
	sort.SliceStable(l.sidecars, func(i, j int) bool { return ojson.CompareUTF16(l.sidecars[i].name, l.sidecars[j].name) < 0 })
	return l, nil
}

// artifactIssues are the per-plan linked-artifact problems used by list.
func artifactIssues(s *snapshot.Snapshot) []string {
	out := []string{}
	for _, m := range s.MissingPlanArtifacts {
		out = append(out, "Missing linked artifact: "+m)
	}
	if s.Markdown.Exists && model.Blank(string(s.Markdown.Bytes)) {
		out = append(out, "Linked Markdown is empty: "+s.Markdown.Rel)
	}
	for _, sp := range s.Specs {
		if sp.Exists && model.Blank(string(sp.Bytes)) {
			out = append(out, "Linked spec is empty: "+sp.Rel)
		}
	}
	return out
}

// List implements workplan_list.
func (e *Engine) List(ListInput) (ojson.Value, error) {
	l, err := e.scanDir()
	if err != nil {
		return ojson.Value{}, err
	}
	plans := []ojson.Value{}
	for _, name := range l.primary {
		canon, nerr := model.NormalizeID(name)
		if nerr != nil || canon != name {
			plans = append(plans, ojson.NewObject(3).
				Set("id", ojson.StringValue(name)).
				Set("valid", ojson.BoolValue(false)).
				Set("issue", ojson.StringValue("Primary plan filename is not a canonical workplan id: "+name+".json")).Value())
			continue
		}
		s, err := e.load(name)
		if err != nil {
			plans = append(plans, ojson.NewObject(3).
				Set("id", ojson.StringValue(name)).
				Set("valid", ojson.BoolValue(false)).
				Set("issue", ojson.StringValue(err.Error())).Value())
			continue
		}
		issues := artifactIssues(s)
		b := ojson.NewObject(16).
			Set("valid", ojson.BoolValue(len(issues) == 0)).
			Set("issues", ojson.StringsValue(issues))
		for _, m := range s.Plan.SummaryMembers() {
			b.Set(m.Key, m.Value)
		}
		b.Set("planHash", ojson.StringValue(s.PlanHash)).
			Set("stateHash", ojson.StringValue(s.StateHash)).
			Set("recoveryRequired", ojson.BoolValue(s.Journal.Exists))
		plans = append(plans, b.Value())
	}
	sidecars := make([]ojson.Value, 0, len(l.sidecars))
	for _, sc := range l.sidecars {
		sidecars = append(sidecars, sidecarValue(sc))
	}
	return ojson.NewObject(5).
		Set("workspaceRoot", ojson.StringValue(e.Root)).
		Set("directory", ojson.StringValue(e.dir())).
		Set("count", ojson.IntValue(int64(len(plans)))).
		Set("workplans", ojson.ArrayValue(plans)).
		Set("sidecars", ojson.ArrayValue(sidecars)).Value(), nil
}

func sidecarValue(sc dirEntry) ojson.Value {
	return ojson.NewObject(2).
		Set("name", ojson.StringValue(sc.name)).
		Set("kind", ojson.StringValue(sc.kind)).Value()
}
