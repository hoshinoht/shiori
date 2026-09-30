package engine

import (
	"strconv"
	"strings"

	"github.com/hoshinoht/shiori/internal/model"
	"github.com/hoshinoht/shiori/internal/ojson"
)

// Strict, schema-driven input parsing for the mutating tools
// (schema/v1/tools/*.input.schema.json). The parser reproduces the
// reference validator's accepted data (schema order, defaults inserted in
// place) and its issue text and paths (contracts §6.2 rule 2). Unknown
// keys are rejected at every level, including nested objects (D2).

type fkind int

const (
	kString fkind = iota
	kBool
	kEnum
	kHash
	kID
	kNonblank
	kStringList
	kIntList
	kObject
	kObjectList
)

type fspec struct {
	key      string
	kind     fkind
	required bool
	enum     []string
	obj      *ospec
	def      *ojson.Value
}

type ospec struct{ fields []fspec }

func str(key string) fspec     { return fspec{key: key, kind: kString} }
func reqStr(key string) fspec  { return fspec{key: key, kind: kString, required: true} }
func boolean(key string) fspec { return fspec{key: key, kind: kBool} }
func enum(key string, e []string) fspec {
	return fspec{key: key, kind: kEnum, enum: e}
}
func hash(key string) fspec              { return fspec{key: key, kind: kHash} }
func strList(key string) fspec           { return fspec{key: key, kind: kStringList} }
func intList(key string) fspec           { return fspec{key: key, kind: kIntList} }
func objList(key string, o *ospec) fspec { return fspec{key: key, kind: kObjectList, obj: o} }
func withDefault(f fspec, v ojson.Value) fspec {
	f.def = &v
	return f
}
func required(f fspec) fspec { f.required = true; return f }

var statusEnum = model.Statuses

var (
	stepSpec        = &ospec{fields: []fspec{str("id"), reqStr("title"), str("target"), str("action"), str("validation"), enum("status", statusEnum)}}
	phaseSpec       = &ospec{fields: []fspec{str("id"), reqStr("title"), enum("status", statusEnum), objList("steps", stepSpec)}}
	findingSpec     = &ospec{fields: []fspec{required(enum("severity", model.Severities)), reqStr("title"), str("detail"), str("source"), enum("status", model.FindingStatuses)}}
	refSpec         = &ospec{fields: []fspec{{key: "phaseId", kind: kNonblank, required: true}, {key: "stepId", kind: kNonblank, required: true}}}
	depSpec         = &ospec{fields: []fspec{{key: "phaseId", kind: kNonblank, required: true}, {key: "stepId", kind: kNonblank, required: true}, required(objList("dependsOn", refSpec))}}
	updatePhaseSpec = &ospec{fields: []fspec{reqStr("phaseId"), str("title"), enum("status", statusEnum)}}
	addPhaseSpec    = &ospec{fields: []fspec{str("afterPhaseId"), {key: "phase", kind: kObject, required: true, obj: phaseSpec}}}
	updateStepSpec  = &ospec{fields: []fspec{reqStr("phaseId"), reqStr("stepId"), str("title"), str("target"), str("action"), str("validation"), enum("status", statusEnum)}}
	addStepSpec     = &ospec{fields: []fspec{reqStr("phaseId"), str("afterStepId"), {key: "step", kind: kObject, required: true, obj: stepSpec}}}
)

var toolSpecs = map[string]*ospec{
	"create": {fields: []fspec{
		{key: "id", kind: kID, required: true},
		withDefault(str("kind"), ojson.StringValue("general")),
		str("title"), reqStr("goal"),
		strList("scope"), strList("nonGoals"), strList("constraints"), strList("relevantFiles"),
		str("planFile"), str("planMarkdown"), strList("specFiles"),
		objList("phases", phaseSpec), objList("reviewFindings", findingSpec), strList("notes"),
		withDefault(enum("status", statusEnum), ojson.StringValue("draft")),
		withDefault(boolean("overwrite"), ojson.BoolValue(false)),
		hash("expectedHash"), boolean("replaceMarkdown"),
	}},
	"update": {fields: []fspec{
		{key: "id", kind: kID, required: true},
		hash("expectedHash"), enum("recovery", []string{"resume", "rollback"}), boolean("replaceMarkdown"),
		str("title"), str("goal"), enum("status", statusEnum),
		strList("scope"), strList("nonGoals"), strList("constraints"),
		str("planFile"), str("planMarkdown"), strList("specFiles"),
		objList("reviewFindings", findingSpec), objList("phases", phaseSpec),
		objList("updatePhases", updatePhaseSpec), objList("addPhases", addPhaseSpec),
		objList("updateSteps", updateStepSpec), objList("addSteps", addStepSpec),
		strList("addRelevantFiles"), strList("addSpecFiles"), strList("removeSpecFiles"),
		objList("addReviewFindings", findingSpec), strList("appendNotes"),
		objList("dependencies", depSpec),
	}},
	"patch": {fields: []fspec{{key: "id", kind: kID, required: true}, reqStr("patchText"), boolean("validate"), hash("expectedHash")}},
	"reset": {fields: []fspec{
		{key: "id", kind: kID, required: true},
		withDefault(enum("mode", []string{"draft", "markdown-only", "wipe"}), ojson.StringValue("draft")),
		withDefault(boolean("preserveNotes"), ojson.BoolValue(false)),
		boolean("replaceMarkdown"), hash("expectedHash"),
		str("previewToken"), str("confirmation"), // D.3: mode=wipe apply
	}},
	"checkpoint": {fields: []fspec{
		{key: "id", kind: kID, required: true}, reqStr("summary"), reqStr("nextAction"),
		str("phaseId"), str("stepId"),
		strList("blockers"), strList("recentValidation"), strList("guardrails"), strList("references"),
		hash("expectedHash"),
	}},
	"compact": {fields: []fspec{
		{key: "id", kind: kID, required: true},
		withDefault(enum("mode", []string{"preview", "apply"}), ojson.StringValue("preview")),
		reqStr("archiveReason"), strList("completedPhaseIds"), intList("noteIndexes"), intList("resolvedFindingIndexes"),
		str("confirmation"), str("previewToken"), hash("expectedHash"),
	}},
	"compact_preview": {fields: []fspec{
		{key: "id", kind: kID, required: true},
		reqStr("archiveReason"), strList("completedPhaseIds"), intList("noteIndexes"), intList("resolvedFindingIndexes"),
		str("confirmation"), str("previewToken"), hash("expectedHash"),
	}},
}

type issueList struct{ issues []model.Issue }

func (l *issueList) add(path []string, msg string) {
	p := append([]string(nil), path...)
	l.issues = append(l.issues, model.Issue{Path: p, Message: msg})
}

func pathWith(path []string, seg string) []string {
	out := make([]string, len(path)+1)
	copy(out, path)
	out[len(path)] = seg
	return out
}

// parseObject validates v against spec and returns the accepted data.
func parseObject(spec *ospec, v ojson.Value, path []string, l *issueList, extraKnown map[string]bool, prefix []ojson.Member) (ojson.Value, bool) {
	if v.Kind() != ojson.Object {
		l.add(path, "Invalid input: expected object, received "+v.TypeName())
		return ojson.Value{}, false
	}
	members := map[string]ojson.Value{}
	var order []string
	for _, m := range v.UniqueMembers() {
		members[m.Key] = m.Value
		order = append(order, m.Key)
	}
	start := len(l.issues)
	out := append([]ojson.Member(nil), prefix...)
	known := map[string]bool{}
	for k := range extraKnown {
		known[k] = true
	}
	for _, f := range spec.fields {
		known[f.key] = true
		fv, present := members[f.key]
		fp := pathWith(path, f.key)
		if !present {
			if f.def != nil {
				out = append(out, ojson.Member{Key: f.key, Value: *f.def})
			} else if f.required {
				l.add(fp, "Invalid input: expected "+expectedType(f.kind)+", received undefined")
			}
			continue
		}
		if val, ok := parseField(f, fv, fp, l); ok {
			out = append(out, ojson.Member{Key: f.key, Value: val})
		}
	}
	var unknown []string
	for _, k := range order {
		if !known[k] {
			unknown = append(unknown, strconv.Quote(k))
		}
	}
	switch len(unknown) {
	case 0:
	case 1:
		l.add(path, "Unrecognized key: "+unknown[0])
	default:
		l.add(path, "Unrecognized keys: "+strings.Join(unknown, ", "))
	}
	return ojson.ObjectValue(out), len(l.issues) == start
}

func expectedType(k fkind) string {
	switch k {
	case kBool:
		return "boolean"
	case kStringList, kIntList, kObjectList:
		return "array"
	case kObject:
		return "object"
	default:
		return "string"
	}
}

func parseField(f fspec, v ojson.Value, path []string, l *issueList) (ojson.Value, bool) {
	typeErr := func(t string) (ojson.Value, bool) {
		l.add(path, "Invalid input: expected "+t+", received "+v.TypeName())
		return ojson.Value{}, false
	}
	switch f.kind {
	case kString:
		if v.Kind() != ojson.String {
			return typeErr("string")
		}
		return v, true
	case kBool:
		if v.Kind() != ojson.Bool {
			return typeErr("boolean")
		}
		return v, true
	case kEnum:
		if v.Kind() == ojson.String {
			for _, e := range f.enum {
				if e == v.Str() {
					return v, true
				}
			}
		}
		q := make([]string, len(f.enum))
		for i, e := range f.enum {
			q[i] = strconv.Quote(e)
		}
		l.add(path, "Invalid option: expected one of "+strings.Join(q, "|"))
		return ojson.Value{}, false
	case kHash:
		if v.Kind() != ojson.String {
			return typeErr("string")
		}
		if !hashRE.MatchString(v.Str()) {
			l.add(path, "Invalid string: must match pattern /^[a-f0-9]{64}$/")
			return ojson.Value{}, false
		}
		return v, true
	case kID:
		if v.Kind() != ojson.String {
			return typeErr("string")
		}
		s := model.TrimJS(v.Str())
		ok := true
		if s == "" {
			l.add(path, "Too small: expected string to have >=1 characters")
			ok = false
		}
		if !idPattern.MatchString(s) {
			l.add(path, "Invalid string: must match pattern /[a-z0-9]/i")
			ok = false
		}
		return ojson.StringValue(s), ok
	case kNonblank:
		if v.Kind() != ojson.String {
			return typeErr("string")
		}
		s := model.TrimJS(v.Str())
		if s == "" {
			l.add(path, "Too small: expected string to have >=1 characters")
			return ojson.Value{}, false
		}
		return ojson.StringValue(s), true
	case kStringList:
		if v.Kind() != ojson.Array {
			return typeErr("array")
		}
		ok := true
		for i, e := range v.Elems() {
			if e.Kind() != ojson.String {
				l.add(pathWith(path, strconv.Itoa(i)), "Invalid input: expected string, received "+e.TypeName())
				ok = false
			}
		}
		return v, ok
	case kIntList:
		if v.Kind() != ojson.Array {
			return typeErr("array")
		}
		ok := true
		out := make([]ojson.Value, 0, len(v.Elems()))
		for i, e := range v.Elems() {
			ep := pathWith(path, strconv.Itoa(i))
			if e.Kind() != ojson.Number {
				l.add(ep, "Invalid input: expected number, received "+e.TypeName())
				ok = false
				continue
			}
			fl, _ := e.Float()
			if fl != float64(int64(fl)) || fl > 9007199254740991 || fl < -9007199254740991 {
				l.add(ep, "Invalid input: expected int, received number")
				ok = false
				continue
			}
			if fl < 0 {
				l.add(ep, "Too small: expected number to be >=0")
				ok = false
				continue
			}
			out = append(out, ojson.IntValue(int64(fl)))
		}
		return ojson.ArrayValue(out), ok
	case kObject:
		return parseObject(f.obj, v, path, l, nil, nil)
	case kObjectList:
		if v.Kind() != ojson.Array {
			return typeErr("array")
		}
		ok := true
		out := make([]ojson.Value, 0, len(v.Elems()))
		for i, e := range v.Elems() {
			val, good := parseObject(f.obj, e, pathWith(path, strconv.Itoa(i)), l, nil, nil)
			if !good {
				ok = false
				continue
			}
			out = append(out, val)
		}
		return ojson.ArrayValue(out), ok
	}
	return ojson.Value{}, false
}

// ParseMutationInput validates the input of a mutating tool ("create",
// "update", "patch", "reset", "checkpoint", "compact", "compact_preview")
// on a surface and returns the accepted data object in schema order.
func ParseMutationInput(tool string, v ojson.Value, s Surface) (ojson.Value, error) {
	spec := toolSpecs[tool]
	if spec == nil {
		return ojson.Value{}, &InputError{Tool: tool, Issues: []model.Issue{{Message: "unknown tool"}}}
	}
	var l issueList
	var extra map[string]bool
	var prefix []ojson.Member
	if s == SurfaceCore {
		extra = map[string]bool{"workspaceRoot": true}
		if v.Kind() == ojson.Object {
			if wr, ok := v.Get("workspaceRoot"); ok {
				if wr.Kind() != ojson.String {
					l.add([]string{"workspaceRoot"}, "Invalid input: expected string, received "+wr.TypeName())
				} else {
					prefix = []ojson.Member{{Key: "workspaceRoot", Value: wr}}
				}
			}
		}
	}
	data, ok := parseObject(spec, v, nil, &l, extra, prefix)
	if ok && len(l.issues) == 0 {
		refine(tool, data, v, s, &l)
	}
	if len(l.issues) > 0 {
		return ojson.Value{}, &InputError{Tool: tool, Issues: l.issues}
	}
	return data, nil
}

const (
	msgNativeHash       = "Native existing-state writes require the current stateHash"
	msgCoreOverwrite    = "overwrite requires the current stateHash"
	msgRecoveryExcl     = "recovery is mutually exclusive with ordinary update fields"
	msgApplyConfirm     = "Apply requires confirmation=ARCHIVE_SELECTED_HISTORY"
	msgApplyTokenNative = "Apply requires the matching preview token"
	msgApplyTokenCore   = "apply requires the token from the matching preview"
	// ConfirmArchive is the compaction apply confirmation phrase.
	ConfirmArchive = "ARCHIVE_SELECTED_HISTORY"
)

// refine applies the cross-field rules (x-shiori-rules) after the shape
// checks pass.
func refine(tool string, data, raw ojson.Value, s Surface, l *issueList) {
	has := func(k string) bool { _, ok := data.Get(k); return ok }
	native := s == SurfaceNative
	switch tool {
	case "create":
		if ov, _ := data.Get("overwrite"); ov.Bool() && !has("expectedHash") {
			if native {
				l.add([]string{"expectedHash"}, msgNativeHash)
			} else {
				l.add([]string{"expectedHash"}, msgCoreOverwrite)
			}
		}
	case "update":
		if native && !has("expectedHash") {
			l.add([]string{"expectedHash"}, msgNativeHash)
		}
		if has("recovery") {
			for _, m := range data.Members() {
				switch m.Key {
				case "id", "expectedHash", "recovery":
					continue
				case "workspaceRoot":
					if !native {
						continue
					}
				case "replaceMarkdown":
					if !native && !m.Value.Bool() {
						continue
					}
				}
				l.add([]string{"recovery"}, msgRecoveryExcl)
				break
			}
		}
	case "patch", "reset", "checkpoint":
		if native && !has("expectedHash") {
			l.add([]string{"expectedHash"}, msgNativeHash)
		}
		if tool == "reset" {
			// D.3 (contracts §13 item 1): the wipe apply fields.
			mode, _ := data.Get("mode")
			for _, k := range []string{"previewToken", "confirmation"} {
				if has(k) && mode.Str() != "wipe" {
					l.add([]string{k}, msgWipeOnlyOptions)
				}
			}
			if mode.Str() == "wipe" && (has("previewToken") || has("confirmation")) {
				if c, _ := data.Get("confirmation"); c.Str() != ConfirmWipe {
					l.add([]string{"confirmation"}, msgWipeConfirm)
				}
				if !has("previewToken") {
					l.add([]string{"previewToken"}, msgWipeToken)
				}
			}
		}
	case "compact":
		if mode, _ := data.Get("mode"); mode.Str() == "apply" {
			if native {
				if !has("expectedHash") {
					l.add([]string{"expectedHash"}, msgNativeHash)
				}
				if c, _ := data.Get("confirmation"); c.Str() != ConfirmArchive {
					l.add([]string{"confirmation"}, msgApplyConfirm)
				}
				if !has("previewToken") {
					l.add([]string{"previewToken"}, msgApplyTokenNative)
				}
			} else if !has("previewToken") {
				l.add([]string{"previewToken"}, msgApplyTokenCore)
			}
		}
	}
}
